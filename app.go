package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"project_clear/internal/config"
	"project_clear/internal/logging"
	"project_clear/internal/mps"
	"project_clear/internal/service"
	"project_clear/internal/store"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Version is shown in the About window.
const (
	AppName    = "CLEAR"
	AppFull    = "Consolidation & Loading of Enterprise Analytics for Replenishment"
	AppVersion = "1.0.0"
)

// App is the object whose exported methods are bound to the frontend.
type App struct {
	ctx context.Context

	cfg  *config.Store
	log  *logging.Logger
	db   *store.Store
	svc  *service.Service
	data string

	bootOnce sync.Once
	bootErr  error
}

// NewApp builds the application object.
func NewApp() *App { return &App{} }

// startup runs once when Wails brings the backend up.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.bootOnce.Do(func() { a.bootErr = a.boot() })
}

func (a *App) boot() error {
	dataDir, err := config.DataDir()
	if err != nil {
		return err
	}
	a.data = dataDir
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("创建数据目录失败: %w", err)
	}

	cfg, notes, err := config.NewStore()
	if err != nil {
		return err
	}
	a.cfg = cfg

	a.log = logging.New(filepath.Join(dataDir, "logs"))
	for _, n := range notes {
		a.log.Info("启动", "%s", n)
	}

	db, err := store.Open(filepath.Join(dataDir, "clear.db"))
	if err != nil {
		return err
	}
	a.db = db
	a.svc = service.New(cfg, a.log, db, dataDir)

	a.log.Success("启动", "%s v%s 已就绪，数据库 %s", AppName, AppVersion, filepath.Join(dataDir, "clear.db"))

	// Stream log entries to every open window so the log panel updates live.
	go a.streamLogs()
	return nil
}

func (a *App) streamLogs() {
	ch, cancel := a.log.Subscribe()
	defer cancel()
	for {
		select {
		case <-a.ctx.Done():
			return
		case e := <-ch:
			wr.EventsEmit(a.ctx, "log:entry", e)
		}
	}
}

func (a *App) ready() error {
	if a.bootErr != nil {
		return a.bootErr
	}
	if a.svc == nil {
		return fmt.Errorf("应用尚未初始化")
	}
	return nil
}

func (a *App) progress(stage string, done, total int) {
	wr.EventsEmit(a.ctx, "task:progress", map[string]any{
		"stage": stage, "done": done, "total": total,
	})
}

// ---------------------------------------------------------------- app info

// AppInfo describes the application for the About window.
type AppInfo struct {
	Name       string `json:"name"`
	FullName   string `json:"fullName"`
	Version    string `json:"version"`
	DataDir    string `json:"dataDir"`
	ConfigPath string `json:"configPath"`
	Database   string `json:"database"`
	GoVersion  string `json:"goVersion"`
	Platform   string `json:"platform"`
}

// GetAppInfo returns the About-window payload.
func (a *App) GetAppInfo() AppInfo {
	info := AppInfo{
		Name: AppName, FullName: AppFull, Version: AppVersion,
		GoVersion: runtime.Version(), Platform: runtime.GOOS + "/" + runtime.GOARCH,
	}
	if a.cfg != nil {
		info.ConfigPath = a.cfg.Path()
	}
	if a.data != "" {
		info.DataDir = a.data
		info.Database = filepath.Join(a.data, "clear.db")
	}
	return info
}

// ---------------------------------------------------------------- status

// Status backs the status bar.
type Status struct {
	HasStaging    bool   `json:"hasStaging"`
	WeekCode      string `json:"weekCode"`
	WeekStart     string `json:"weekStart"`
	StagedRows    int    `json:"stagedRows"`
	ArchivedWeeks int    `json:"archivedWeeks"`
	ArchivedRows  int    `json:"archivedRows"`
	ReadColumns   int    `json:"readColumns"`
	LOCFilter     string `json:"locFilter"`
	PageSize      int    `json:"pageSize"`
	HeaderDisplay string `json:"headerDisplay"`
	ExportMode    string `json:"exportMode"`
	Database      string `json:"database"`
	LastAction    string `json:"lastAction"`
}

// GetStatus returns the current state for the status bar.
func (a *App) GetStatus() (*Status, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	st := &Status{
		ReadColumns: c.ReadColumns, LOCFilter: c.LOCFilter, PageSize: c.PageSize,
		HeaderDisplay: string(c.HeaderDisplay), ExportMode: string(c.ExportMode),
		Database: filepath.Join(a.data, "clear.db"),
	}
	sum, err := a.db.LoadStaging()
	if err != nil {
		return nil, err
	}
	if sum.HasStaging {
		st.HasStaging = true
		st.WeekCode = sum.WeekCode
		st.WeekStart = sum.WeekStart
		st.StagedRows = sum.RowKept
	}
	list, err := a.db.ListArchive()
	if err != nil {
		return nil, err
	}
	st.ArchivedWeeks = len(list)
	for _, e := range list {
		st.ArchivedRows += e.RowCount
	}
	if len(a.log.Recent(1)) > 0 {
		last := a.log.Recent(1)[0]
		st.LastAction = fmt.Sprintf("%s %s", last.Time, last.Message)
	}
	return st, nil
}

// ---------------------------------------------------------------- import

// ImportFolder asks for a folder and imports every workbook inside it.
func (a *App) ImportFolder() (*service.ImportResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	dir, err := wr.OpenDirectoryDialog(a.ctx, wr.OpenDialogOptions{
		Title: "选择包含 MPS 源文件的文件夹",
	})
	if err != nil {
		return nil, err
	}
	if dir == "" {
		return nil, nil // cancelled
	}
	res, err := a.svc.ImportFolder(dir, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// AddFiles asks for one or more workbooks and stages them.
func (a *App) AddFiles() (*service.ImportResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	paths, err := wr.OpenMultipleFilesDialog(a.ctx, wr.OpenDialogOptions{
		Title: "选择 MPS 源文件（可多选）",
		Filters: []wr.FileFilter{
			{DisplayName: "Excel 工作簿 (*.xlsm;*.xlsx)", Pattern: "*.xlsm;*.xlsx"},
		},
	})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	res, err := a.svc.AddFiles(paths, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// ---------------------------------------------------------------- commit

// CommitResult reports the outcome of 整合.
type CommitResult struct {
	WeekCode    string `json:"weekCode"`
	WeekStart   string `json:"weekStart"`
	TableName   string `json:"tableName"`
	RowCount    int    `json:"rowCount"`
	FileCount   int    `json:"fileCount"`
	CommittedAt string `json:"committedAt"`
	Overwrote   bool   `json:"overwrote"`
}

// Commit promotes the staged data into its permanent weekly table.
func (a *App) Commit() (*CommitResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	previous, _ := a.db.ArchiveEntryFor(mustStagingWeek(a))
	entry, err := a.svc.Commit()
	if err != nil {
		return nil, err
	}
	return &CommitResult{
		WeekCode: entry.WeekCode, WeekStart: entry.WeekStart, TableName: entry.TableName,
		RowCount: entry.RowCount, FileCount: entry.FileCount, CommittedAt: entry.CommittedAt,
		Overwrote: previous != nil,
	}, nil
}

func mustStagingWeek(a *App) string {
	sum, err := a.db.LoadStaging()
	if err != nil || !sum.HasStaging {
		return ""
	}
	return sum.WeekCode
}

// ---------------------------------------------------------------- export

// ExportResult reports the outcome of 导出.
type ExportResult struct {
	DestPath     string `json:"destPath"`
	Mode         string `json:"mode"`
	Rows         int    `json:"rows"`
	Cols         int    `json:"cols"`
	Comments     int    `json:"comments"`
	CFRows       int    `json:"cfRows"`
	StylesUsed   int    `json:"stylesUsed"`
	Dropped      int    `json:"dropped"`
	SizeBytes    int64  `json:"sizeBytes"`
	PreservedVBA bool   `json:"preservedVba"`
	DurationMS   int64  `json:"durationMs"`
}

// Export asks where to save the committed data and writes it.
func (a *App) Export(weekCode string, clean bool) (*ExportResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if weekCode == "" {
		weekCode = mustStagingWeek(a)
	}
	if weekCode == "" {
		return nil, fmt.Errorf("没有可导出的数据，请先导入并整合")
	}
	c := a.cfg.Get()
	ext := ".xlsm"
	if clean || c.ExportMode == config.ExportClean {
		ext = ".xlsx"
	}
	dir := c.ExportDir
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, "Documents")
		}
	}
	dest, err := wr.SaveFileDialog(a.ctx, wr.SaveDialogOptions{
		Title:            "导出整合数据",
		DefaultDirectory: dir,
		DefaultFilename:  fmt.Sprintf("CLEAR_%s%s", weekCode, ext),
		Filters: []wr.FileFilter{
			{DisplayName: "Excel 工作簿", Pattern: "*" + ext},
		},
	})
	if err != nil {
		return nil, err
	}
	if dest == "" {
		return nil, nil
	}
	if !strings.EqualFold(filepath.Ext(dest), ext) {
		dest = strings.TrimSuffix(dest, filepath.Ext(dest)) + ext
	}
	res, err := a.svc.Export(service.ExportOptions{WeekCode: weekCode, DestPath: dest, Clean: clean}, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	// Remember the folder for next time.
	c.ExportDir = filepath.Dir(dest)
	if _, err := a.cfg.Save(c); err != nil {
		a.log.Warn("参数", "保存导出目录失败: %v", err)
	}
	return &ExportResult{
		DestPath: dest, Mode: res.Mode, Rows: res.Rows, Cols: res.Cols,
		Comments: res.Comments, CFRows: res.CFRows, StylesUsed: res.StylesUsed,
		Dropped: res.CFRulesDropped, SizeBytes: res.SizeBytes,
		PreservedVBA: res.PreservedVBA, DurationMS: res.DurationMS,
	}, nil
}

// RevealExport opens the folder containing an exported file.
func (a *App) RevealExport(path string) {
	if path == "" {
		return
	}
	wr.BrowserOpenURL(a.ctx, "file://"+filepath.Dir(path))
}

// ---------------------------------------------------------------- data

// GridQuery is the data-grid request.
type GridQuery struct {
	// Source is "" for the staging area or a week code such as "2639".
	Source    string            `json:"source"`
	Page      int               `json:"page"`
	PageSize  int               `json:"pageSize"`
	Search    string            `json:"search"`
	SortField string            `json:"sortField"`
	SortDesc  bool              `json:"sortDesc"`
	Filters   map[string]string `json:"filters"`
}

// QueryData returns one page of the merged data.
func (a *App) QueryData(q GridQuery) (*store.GridResult, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	if q.PageSize <= 0 {
		q.PageSize = c.PageSize
	}
	return a.db.QueryRows(store.Query{
		Source: q.Source, Page: q.Page, PageSize: q.PageSize,
		Search: q.Search, SortField: q.SortField, SortDesc: q.SortDesc, Filters: q.Filters,
	})
}

// GridHeader describes the columns the grid must render.
type GridHeader struct {
	Source     string     `json:"source"`
	WeekCode   string     `json:"weekCode"`
	WeekStart  string     `json:"weekStart"`
	IndexNames []string   `json:"indexNames"`
	Weeks      []mps.Week `json:"weeks"`
	Total      int        `json:"total"`
	HasStaging bool       `json:"hasStaging"`
}

// GetGridHeader returns the column layout for a source.
func (a *App) GetGridHeader(source string) (*GridHeader, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	out := &GridHeader{Source: source}
	var codes, names []string
	if source == "" {
		sum, err := a.db.LoadStaging()
		if err != nil {
			return nil, err
		}
		if !sum.HasStaging {
			return out, nil
		}
		out.HasStaging = true
		out.WeekCode = sum.WeekCode
		out.WeekStart = sum.WeekStart
		codes, names = sum.WeekCodes, sum.IndexNames
	} else {
		if !store.ValidWeekCode(source) {
			return nil, fmt.Errorf("非法周码 %q", source)
		}
		entry, err := a.db.ArchiveEntryFor(source)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return out, nil
		}
		out.WeekCode = entry.WeekCode
		out.WeekStart = entry.WeekStart
		codes, names = entry.WeekCodes, entry.IndexNames
	}
	out.IndexNames = names
	for _, c := range codes {
		w, err := mps.ParseWeekCode(c)
		if err != nil {
			return nil, err
		}
		out.Weeks = append(out.Weeks, w)
	}
	return out, nil
}

// GetArchive lists committed weeks.
func (a *App) GetArchive() ([]store.ArchiveEntry, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.db.ListArchive()
}

// GetYears lists the years that have committed weeks.
func (a *App) GetYears() ([]int, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.db.AvailableYears()
}

// GetWeeks lists committed week numbers for a year.
func (a *App) GetWeeks(year int) ([]int, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.db.WeeksOfYear(year)
}

// ---------------------------------------------------------------- config

// ConfigView is the settings payload.
type ConfigView struct {
	Config config.Config `json:"config"`
	Path   string        `json:"path"`
}

// GetConfig returns the active parameters and where they live.
func (a *App) GetConfig() (*ConfigView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	return &ConfigView{Config: a.cfg.Get(), Path: a.cfg.Path()}, nil
}

// SaveConfig validates and persists parameters, reporting any corrections.
func (a *App) SaveConfig(c config.Config) ([]string, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	notes, err := a.cfg.Save(c)
	if err != nil {
		a.log.Error("参数", "保存失败: %v", err)
		return notes, err
	}
	if len(notes) > 0 {
		for _, n := range notes {
			a.log.Warn("参数", "%s", n)
		}
	} else {
		a.log.Success("参数", "已保存：读取列数 %d，LOC 筛选 %s，每页 %d 行，导出模式 %s",
			c.ReadColumns, c.LOCFilter, c.PageSize, c.ExportMode)
	}
	return notes, nil
}

// ResetConfig restores the shipped defaults.
func (a *App) ResetConfig() (*ConfigView, error) {
	if err := a.ready(); err != nil {
		return nil, err
	}
	if err := a.cfg.ResetToDefault(); err != nil {
		return nil, err
	}
	a.log.Info("参数", "已恢复默认参数")
	return &ConfigView{Config: a.cfg.Get(), Path: a.cfg.Path()}, nil
}

// ---------------------------------------------------------------- logs

// GetLogs returns the most recent log entries.
func (a *App) GetLogs(limit int) []logging.Entry {
	if a.log == nil {
		return nil
	}
	return a.log.Recent(limit)
}

// ClearLogs empties the in-memory log buffer.
func (a *App) ClearLogs() {
	if a.log != nil {
		a.log.Clear()
	}
}

// ---------------------------------------------------------------- windows
//
// Wails v2 is a single-window framework: there is no WindowCreate, and the
// runtime window calls only ever act on the calling window. The secondary
// views (history, settings, about) are therefore rendered as full-height
// overlays inside the main window rather than as separate OS windows.

// Quit shuts the application down.
func (a *App) Quit() {
	if a.db != nil {
		_ = a.db.Close()
	}
	wr.Quit(a.ctx)
}
