package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"project_clear/internal/config"
	"project_clear/internal/logging"
	"project_clear/internal/mps"
	"project_clear/internal/service"
	"project_clear/internal/store"
	"project_clear/internal/view"

	wr "github.com/wailsapp/wails/v2/pkg/runtime"
)

// Version is shown in the About window.
const (
	AppName    = "CLEAR"
	AppFull    = "Consolidation & Loading of Enterprise Analytics for Replenishment"
	AppVersion = "1.6.0"
)

// App is the object whose exported methods are bound to the frontend.
type App struct {
	ctx context.Context
	// runCtx belongs to the application and is cancelled on shutdown. Wails
	// never cancels the context it hands to OnStartup, so a child context we
	// own is the only way to stop the log stream goroutine.
	runCtx context.Context
	cancel context.CancelFunc

	cfg  *config.Store
	log  *logging.Logger
	db   *store.Store
	svc  *service.Service
	data string

	bootOnce sync.Once
	// shutdownOnce keeps the clean-up idempotent: Quit and OnShutdown can both
	// run, in either order.
	shutdownOnce sync.Once

	// bootDone closes when startup has finished, so a webview that loads
	// faster than the backend can wait for it instead of failing outright.
	bootDone chan struct{}
	bootErr  error
}

// NewApp builds the application object.
func NewApp() *App { return &App{bootDone: make(chan struct{})} }

// domReady fires when the webview finished loading the document.
func (a *App) domReady(ctx context.Context) {
	// Startup may have failed before the logger existed, and Wails fires
	// OnDomReady regardless: dereferencing a nil log here would panic inside
	// the message loop and take the process down with no diagnostics.
	if a.log == nil {
		return
	}
	w, h := wr.WindowGetSize(ctx)
	a.log.Info("界面", "页面加载完成，窗口尺寸 %dx%d", w, h)
}

// startup runs once when Wails brings the backend up.
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.runCtx, a.cancel = context.WithCancel(ctx)
	// The non-macOS file pickers go through the Wails runtime, which needs the
	// context; on macOS this is a no-op.
	setDialogContext(ctx)
	a.bootOnce.Do(func() {
		a.bootErr = a.boot()
		close(a.bootDone)
	})
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
	// A database written by an older CLEAR may still sit in the bundle or in
	// ~/.clear; bring it over before anything opens an empty one.
	adopted := config.AdoptDataDir(dataDir)

	cfg, notes, err := config.NewStore()
	if err != nil {
		return err
	}
	a.cfg = cfg

	a.log = logging.New(filepath.Join(dataDir, "logs"))
	if adopted != "" {
		a.log.Success("启动", "已把旧位置的数据迁移到 %s（原目录 %s 保留未动）", dataDir, adopted)
	}
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

	// The window state is deliberately left alone here. Calling WindowShow or
	// WindowUnminimise after start-up can take the key-window status away at
	// exactly the moment a native panel is presented, which makes a file
	// dialog flash and close.
	go func() {
		time.Sleep(1200 * time.Millisecond)
		w, h := wr.WindowGetSize(a.ctx)
		a.log.Info("界面", "窗口尺寸 %dx%d", w, h)
	}()

	// Stream log entries to every open window so the log panel updates live.
	go a.streamLogs(a.runCtx)
	return nil
}

func (a *App) streamLogs(ctx context.Context) {
	if ctx == nil {
		ctx = context.Background()
	}
	ch, cancel := a.log.Subscribe()
	defer cancel()
	for {
		select {
		case <-ctx.Done():
			return
		case e := <-ch:
			wr.EventsEmit(a.ctx, "log:entry", e)
		}
	}
}

// shutdown stops the log stream and releases the database once any operation in
// progress has finished. Wails calls it from OnShutdown, and Quit calls it
// before asking the window to close, so it has to be safe to run twice and in
// either order. The context is the one Wails passes to OnShutdown; nothing here
// needs it.
func (a *App) shutdown(context.Context) {
	a.shutdownOnce.Do(func() {
		if a.cancel != nil {
			a.cancel()
		}
		if a.svc != nil {
			// An import or an export may still be running on its own goroutine.
			// Closing the database underneath it would fail the operation
			// halfway, so this waits for the current one to finish.
			a.svc.WaitIdle()
		}
		if a.db != nil {
			_ = a.db.Close()
		}
		// Nothing to flush: the logger writes and closes the daily file per
		// entry rather than buffering it.
	})
}

func (a *App) ready() error {
	if err := a.waitBoot(); err != nil {
		return err
	}
	if a.bootErr != nil {
		return a.bootErr
	}
	if a.svc == nil {
		return fmt.Errorf("应用尚未初始化")
	}
	return nil
}

// waitBoot blocks until startup has finished. Wails calls OnStartup and
// OnDomReady on separate goroutines, so the page can ask for data before the
// database is open; blocking briefly is what keeps the first render correct
// instead of leaving the window stuck on an empty state.
func (a *App) waitBoot() error {
	select {
	case <-a.bootDone:
		return nil
	case <-time.After(30 * time.Second):
		return fmt.Errorf("应用初始化超时，请重新启动 CLEAR")
	}
}

func (a *App) progress(stage string, done, total int) {
	wr.EventsEmit(a.ctx, "task:progress", map[string]any{
		"stage": stage, "done": done, "total": total,
	})
}

// recoverFault turns a panic inside a bound method into a returned error.
//
// Wails recovers a panicking bound method itself, but then calls the webview
// back with an empty string, which is not valid JSON. The promise never
// settles, so the window stays in its processing state with no message and the
// only way out is to kill the application. Returning an ordinary error instead
// lets the frontend report it and put its controls back.
//
// It must be deferred directly: recover only works from the deferred call
// itself.
func (a *App) recoverFault(where string, err *error) {
	r := recover()
	if r == nil {
		return
	}
	msg := fmt.Sprintf("%s 内部错误: %v", where, r)
	if a.log != nil {
		a.log.Error(where, "%s", msg)
		a.log.Error(where, "调用栈: %s", truncate(string(debug.Stack()), 2000))
	}
	if err != nil {
		*err = errors.New(msg)
	}
}

// ---------------------------------------------------------------- app info

// lastDir is where the chooser should open; the last folder used is the most
// useful default for a weekly batch job.
func (a *App) lastDir() string {
	c := a.cfg.Get()
	if c.ExportDir != "" {
		return c.ExportDir
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Documents")
	}
	return ""
}

func (a *App) rememberDir(dir string) {
	if dir == "" {
		return
	}
	c := a.cfg.Get()
	if c.ExportDir == dir {
		return
	}
	c.ExportDir = dir
	if _, err := a.cfg.Save(c); err != nil {
		a.log.Warn("参数", "保存目录失败: %v", err)
	}
}

// GetAppInfo returns the About-window payload.
func (a *App) GetAppInfo() view.AppInfo {
	defer a.recoverFault("GetAppInfo", nil)
	info := view.AppInfo{
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

// GetStatus returns the current state for the status bar.
func (a *App) GetStatus() (out *view.Status, err error) {
	defer a.recoverFault("GetStatus", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	st := &view.Status{
		ReadColumns: c.ReadColumns, LOCFilter: c.LOCFilter, PageSize: c.PageSize,
		HeaderDisplay: string(c.HeaderDisplay), ExportMode: string(c.ExportMode),
		Database: filepath.Join(a.data, "clear.db"),
	}
	sum, err := a.svc.StagingSummary()
	if err != nil {
		return nil, err
	}
	if sum.HasStaging {
		st.HasStaging = true
		st.WeekCode = sum.WeekCode
		st.WeekStart = sum.WeekStart
		st.StagedRows = sum.RowKept
	}
	list, err := a.svc.Archive()
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
func (a *App) ImportFolder() (out *service.ImportResult, err error) {
	defer a.recoverFault("ImportFolder", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	picked, err := selectPaths("选择包含 MPS 源文件的文件夹", a.lastDir(), false, false, nil)
	if err != nil {
		return nil, err
	}
	if len(picked) == 0 {
		return nil, nil // cancelled
	}
	dir := picked[0]
	a.rememberDir(dir)
	res, err := a.svc.ImportFolder(dir, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// AddFiles asks for one or more workbooks and stages them.
func (a *App) AddFiles() (out *service.ImportResult, err error) {
	defer a.recoverFault("AddFiles", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	paths, err := selectPaths("选择 MPS 源文件（可多选）", a.lastDir(), true, true,
		[]string{"xlsm", "xlsx"})
	if err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	if len(paths) == 1 {
		a.rememberDir(filepath.Dir(paths[0]))
	}
	res, err := a.svc.AddFiles(paths, a.progress, false)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// ConfirmAddFiles retries a 添加 after the user accepted that the listed files
// are already in the integration list. Their old rows are replaced by the
// freshly read ones; every other file keeps its data.
func (a *App) ConfirmAddFiles(paths []string) (out *service.ImportResult, err error) {
	defer a.recoverFault("ConfirmAddFiles", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	if len(paths) == 0 {
		return nil, nil
	}
	res, err := a.svc.AddFiles(paths, a.progress, true)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return res, nil
}

// ---------------------------------------------------------------- commit

// Commit promotes the staged data into its permanent weekly table.
func (a *App) Commit() (out *view.CommitResult, err error) {
	defer a.recoverFault("Commit", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	entry, overwrote, err := a.svc.Commit()
	if err != nil {
		return nil, err
	}
	return &view.CommitResult{
		WeekCode: entry.WeekCode, WeekStart: entry.WeekStart, TableName: entry.TableName,
		RowCount: entry.RowCount, FileCount: entry.FileCount, CommittedAt: entry.CommittedAt,
		Overwrote: overwrote,
	}, nil
}

// ------------------------------------------------------- staging file list

// GetStagingFiles lists the work set behind the toolbar's 已导入文件 button: the
// files that were imported (or added), and — after 整合 — the same list as the
// record of that import. It stays until 清空 or the next import.
func (a *App) GetStagingFiles() (out *view.StagingFilesView, err error) {
	defer a.recoverFault("GetStagingFiles", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	out = view.NewStagingFilesView()
	files, _, state, err := a.svc.BatchFiles()
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return out, nil
	}
	out.HasStaging = state == "staging"
	out.BatchState = state
	out.Files = files
	for _, f := range files {
		if f.Status == "ok" {
			out.FileCount++
			out.RowCount += f.RowsKept
		} else {
			out.Failed++
		}
	}
	return out, nil
}

// ---------------------------------------------------------------- clear

// ClearStaging drops every imported-but-unsaved row: the staging area and the
// export template that came with it. Weeks already integrated are untouched.
func (a *App) ClearStaging() (out *view.ClearStagingResult, err error) {
	defer a.recoverFault("ClearStaging", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	sum, err := a.svc.StagingSummary()
	if err != nil {
		return nil, err
	}
	// 清空 always runs: with no staging left it only ends the 已导入文件 list
	// (the record of the batch that was just integrated).
	rows, err := a.svc.ClearStaging()
	if err != nil {
		return nil, err
	}
	return &view.ClearStagingResult{WeekCode: sum.WeekCode, Rows: rows}, nil
}

// ---------------------------------------------------------------- export

// Export asks where to save the data and writes it. An empty mode uses the
// configured default (纯数据); "clean" and "template" force one engine.
func (a *App) Export(weekCode string, mode string) (out *view.ExportResult, err error) {
	defer a.recoverFault("Export", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	effective := config.ExportMode(mode)
	if effective == "" {
		effective = c.ExportMode
	}
	ext := ".xlsm"
	if effective == config.ExportClean {
		ext = ".xlsx"
	}
	// The save dialog needs a name before the export runs; with no week named
	// this is whatever the main grid is showing.
	target := a.svc.ExportWeek(weekCode)
	if target == "" {
		return nil, fmt.Errorf("没有可导出的数据，请先导入文件")
	}
	dir := c.ExportDir
	if dir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dir = filepath.Join(home, "Documents")
		}
	}
	dest, err := selectSavePath("导出整合数据", fmt.Sprintf("CLEAR_%s%s", target, ext), dir,
		[]string{strings.TrimPrefix(ext, ".")})
	if err != nil {
		return nil, err
	}
	if dest == "" {
		return nil, nil
	}
	a.rememberDir(filepath.Dir(dest))
	if !strings.EqualFold(filepath.Ext(dest), ext) {
		dest = strings.TrimSuffix(dest, filepath.Ext(dest)) + ext
	}
	res, err := a.svc.Export(service.ExportOptions{WeekCode: weekCode, DestPath: dest, Mode: effective}, a.progress)
	if err != nil {
		wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": false, "message": err.Error()})
		return nil, err
	}
	wr.EventsEmit(a.ctx, "task:done", map[string]any{"ok": true})
	return &view.ExportResult{
		DestPath: dest, Mode: res.Mode, Rows: res.Rows, Cols: res.Cols,
		Comments: res.Comments, CFRows: res.CFRows, StylesUsed: res.StylesUsed,
		Dropped: res.CFRulesDropped, SizeBytes: res.SizeBytes,
		PreservedVBA: res.PreservedVBA, DurationMS: res.DurationMS,
	}, nil
}

// RevealExport opens the folder containing an exported file.
func (a *App) RevealExport(path string) {
	defer a.recoverFault("RevealExport", nil)
	if path == "" {
		return
	}
	wr.BrowserOpenURL(a.ctx, "file://"+filepath.Dir(path))
}

// ---------------------------------------------------------------- data

// QueryData returns one page of the merged data.
func (a *App) QueryData(q view.GridQuery) (out *store.GridResult, err error) {
	defer a.recoverFault("QueryData", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	c := a.cfg.Get()
	if q.PageSize <= 0 {
		q.PageSize = c.PageSize
	}
	return a.svc.Query(store.Query{
		Source: q.Source, Page: q.Page, PageSize: q.PageSize,
		Search: q.Search, SortField: q.SortField, SortDesc: q.SortDesc, Filters: q.Filters,
	})
}

// GetGridHeader returns the column layout for a source.
func (a *App) GetGridHeader(source string) (out *view.GridHeader, err error) {
	defer a.recoverFault("GetGridHeader", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	out = view.NewGridHeader(source)
	// The week-code check stays here so a crafted request is rejected before it
	// reaches the store; resolving the source itself belongs to the service.
	if source != "" && !store.ValidWeekCode(source) {
		return nil, fmt.Errorf("非法周码 %q", source)
	}
	src, err := a.svc.GridSource(source)
	if err != nil {
		return nil, err
	}
	out.HasStaging = src.HasStaging
	out.WeekCode = src.WeekCode
	out.WeekStart = src.WeekStart
	if src.IndexNames != nil {
		out.IndexNames = src.IndexNames
	}
	for _, c := range src.WeekCodes {
		w, err := mps.ParseWeekCode(c)
		if err != nil {
			return nil, err
		}
		out.Weeks = append(out.Weeks, w)
	}
	return out, nil
}

// GetArchive lists committed weeks.
func (a *App) GetArchive() (out []store.ArchiveEntry, err error) {
	defer a.recoverFault("GetArchive", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Archive()
}

// GetYears lists the years that have committed weeks.
func (a *App) GetYears() (out []int, err error) {
	defer a.recoverFault("GetYears", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Years()
}

// GetWeeks lists committed week numbers for a year.
func (a *App) GetWeeks(year int) (out []int, err error) {
	defer a.recoverFault("GetWeeks", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return a.svc.Weeks(year)
}

// ---------------------------------------------------------------- config

// GetConfig returns the active parameters and where they live.
func (a *App) GetConfig() (out *view.ConfigView, err error) {
	defer a.recoverFault("GetConfig", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	return &view.ConfigView{Config: a.cfg.Get(), Path: a.cfg.Path()}, nil
}

// SaveConfig validates and persists parameters, reporting any corrections.
func (a *App) SaveConfig(c config.Config) (out []string, err error) {
	defer a.recoverFault("SaveConfig", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	notes, err := a.cfg.Save(c)
	if notes == nil {
		// Wails turns a nil slice into null; the dialog reads this as an array.
		notes = []string{}
	}
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
func (a *App) ResetConfig() (out *view.ConfigView, err error) {
	defer a.recoverFault("ResetConfig", &err)
	if err := a.ready(); err != nil {
		return nil, err
	}
	if err := a.cfg.ResetToDefault(); err != nil {
		return nil, err
	}
	a.log.Info("参数", "已恢复默认参数")
	return &view.ConfigView{Config: a.cfg.Get(), Path: a.cfg.Path()}, nil
}

// ---------------------------------------------------------------- logs

// ReportFrontendError records a webview-side exception in the same log the
// status bar shows, so a rendering failure is diagnosable instead of blank.
func (a *App) ReportFrontendError(message, stack, source string) {
	defer a.recoverFault("ReportFrontendError", nil)
	if a.log == nil {
		return
	}
	where := source
	if where == "" {
		where = "界面"
	}
	a.log.Error(where, "前端异常: %s", message)
	if stack != "" {
		a.log.Error(where, "调用栈: %s", truncate(stack, 2000))
	}
}

// GetLogs returns the most recent log entries.
func (a *App) GetLogs(limit int) []logging.Entry {
	defer a.recoverFault("GetLogs", nil)
	_ = a.waitBoot()
	if a.log == nil {
		return nil
	}
	return a.log.Recent(limit)
}

// ClearLogs empties the in-memory log buffer.
func (a *App) ClearLogs() {
	defer a.recoverFault("ClearLogs", nil)
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
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + " …"
}

func (a *App) Quit() {
	// The quit is registered first so it still runs if the clean-up panics,
	// which would otherwise leave the window open with no way out.
	defer wr.Quit(a.ctx)
	defer a.recoverFault("Quit", nil)
	a.shutdown(a.ctx)
}
