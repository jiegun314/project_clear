// Package service orchestrates the application's use cases: importing the
// Global workbooks into a draft (or adding to it) and finishing that import,
// importing the Local workbook as a complete dataset, integrating the two, and
// exporting the datasets again.
package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"project_clear/internal/config"
	"project_clear/internal/logging"
	"project_clear/internal/mps"
	"project_clear/internal/store"
)

// FileResult is the per-file report shown after an import or add.
type FileResult struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Status    string `json:"status"`
	RowsTotal int    `json:"rowsTotal"`
	RowsKept  int    `json:"rowsKept"`
	WeekCode  string `json:"weekCode"`
	Err       string `json:"err,omitempty"`
}

// ImportResult summarises one import or add action. A Global import produces a
// draft: it is not usable for 整合 until FinishGlobal is called.
type ImportResult struct {
	Action     string       `json:"action"`
	WeekCode   string       `json:"weekCode"`
	WeekStart  string       `json:"weekStart"`
	WeekCodes  []mps.Week   `json:"weekCodes"`
	IndexNames []string     `json:"indexNames"`
	Total      int          `json:"total"`
	OK         int          `json:"ok"`
	Failed     int          `json:"failed"`
	RowsKept   int          `json:"rowsKept"`
	Files      []FileResult `json:"files"`
	Warnings   []string     `json:"warnings"`
	DurationMS int64        `json:"durationMs"`
	// Replaced names the draft workbooks this add overwrote (matched by name).
	Replaced []string `json:"replaced,omitempty"`
	// Draft marks a Global import that is still being assembled.
	Draft     bool  `json:"draft"`
	DatasetID int64 `json:"datasetId"`
	// LocalRows is how many rows the 本地 MPS import brought in.
	LocalRows int `json:"localRows"`
	// Version is set once a dataset is complete (a Local import, a finish).
	Version int `json:"version"`
	// Expired says the week's existing integration no longer matches the
	// completed imports.
	Expired bool `json:"expired"`
	// RowCount/FileCount are the dataset's totals after the action.
	RowCount  int `json:"rowCount"`
	FileCount int `json:"fileCount"`
}

// FinishResult reports a Global draft that has just been completed.
type FinishResult struct {
	DatasetID     int64      `json:"datasetId"`
	WeekCode      string     `json:"weekCode"`
	WeekStart     string     `json:"weekStart"`
	Version       int        `json:"version"`
	RowCount      int        `json:"rowCount"`
	FileCount     int        `json:"fileCount"`
	FailedCount   int        `json:"failedCount"`
	WeekCodes     []mps.Week `json:"weekCodes"`
	IndexNames    []string   `json:"indexNames"`
	ImportedAt    string     `json:"importedAt"`
	Expired       bool       `json:"expired"`
	IntegrationAt string     `json:"integrationAt,omitempty"`
}

// Progress reports how far a long operation has got.
type Progress func(stage string, done, total int)

// Workspace is the current week's set of datasets and integration.
type Workspace struct {
	Week        string
	Draft       *store.Dataset
	Global      *store.Dataset
	Local       *store.Dataset
	Integration *store.Integration
	// Expired means the integration points at versions older than the current
	// Global or Local imports.
	Expired bool
	// Unintegrated means there is a draft, or a completed import the integration
	// does not include yet. Moving to another week is blocked while this is true.
	Unintegrated bool
}

// ArchiveEntry is one dataset of an integrated week, for the history panel.
type ArchiveEntry struct {
	WeekCode   string `json:"weekCode"`
	WeekStart  string `json:"weekStart"`
	Year       int    `json:"year"`
	WeekNo     int    `json:"weekNo"`
	Dataset    string `json:"dataset"`
	DatasetID  int64  `json:"datasetId"`
	Version    int    `json:"version"`
	RowCount   int    `json:"rowCount"`
	FileCount  int    `json:"fileCount"`
	ImportedAt string `json:"committedAt"`
	Expired    bool   `json:"expired"`
}

// Service is the application layer.
type Service struct {
	cfg *config.Store
	log *logging.Logger
	db  *store.Store

	dataDir     string
	templateDir string

	// activeWeek/sessionActive are the in-memory workbench. They start empty on
	// every launch, so a new session shows no previous import; the finished
	// datasets remain in the database and are reachable through history.
	sessionActive bool
	activeWeek    string

	mu sync.Mutex
}

// New wires the service to its database, log and configuration.
func New(cfg *config.Store, log *logging.Logger, db *store.Store, dataDir string) *Service {
	if log != nil {
		db.SetWarn(func(format string, args ...any) {
			log.Warn("数据库", format, args...)
		})
	}
	return &Service{
		cfg:         cfg,
		log:         log,
		db:          db,
		dataDir:     dataDir,
		templateDir: filepath.Join(dataDir, "templates"),
	}
}

// ---------------------------------------------------------------- imports

// ImportFolder reads every workbook in a folder into a fresh Global draft.
func (s *Service) ImportFolder(dir string, progress Progress) (*ImportResult, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("无法读取文件夹: %w", err)
	}
	var paths []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if isWorkbook(e.Name()) {
			paths = append(paths, filepath.Join(dir, e.Name()))
		}
	}
	if len(paths) == 0 {
		return nil, fmt.Errorf("文件夹内没有找到 .xlsm 或 .xlsx 文件")
	}
	sort.Strings(paths)
	s.log.Info("导入", "扫描文件夹 %s，发现 %d 个源文件", dir, len(paths))
	return s.ingest("import", paths, progress, ingestOptions{})
}

// AddFiles adds workbooks to the open Global draft. With no draft it starts one:
// from the current completed Global dataset (copy-on-write) when the week has
// one, otherwise from the files themselves.
func (s *Service) AddFiles(paths []string, progress Progress) (*ImportResult, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("未选择任何文件")
	}
	paths = uniquePaths(paths)
	ws, err := s.workspaceLocked()
	if err != nil {
		return nil, err
	}
	existing := ws.Draft
	if existing == nil {
		existing = ws.Global
	}
	stagedNames := map[string]bool{}
	if existing != nil {
		files, err := s.db.DatasetFiles(existing.ID)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			stagedNames[f.Name] = true
		}
	}
	var replaced []string
	for _, p := range paths {
		if name := filepath.Base(p); stagedNames[name] {
			replaced = append(replaced, name)
		}
	}
	if len(replaced) > 0 {
		s.log.Info("添加", "%d 个文件与当前导入同名，将覆盖原有数据: %s",
			len(replaced), strings.Join(replaced, "、"))
	}
	s.log.Info("添加", "添加 %d 个源文件", len(paths))
	res, err := s.ingest("add", paths, progress, ingestOptions{add: true, replace: replaced})
	if err != nil {
		return nil, err
	}
	res.Replaced = replaced
	return res, nil
}

func uniquePaths(paths []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(paths))
	for _, p := range paths {
		c := filepath.Clean(p)
		if seen[c] {
			continue
		}
		seen[c] = true
		out = append(out, c)
	}
	return out
}

func isWorkbook(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	return ext == ".xlsm" || ext == ".xlsx"
}

type ingestOptions struct {
	// add is true for 添加: append to the draft instead of starting a fresh one.
	add     bool
	replace []string
}

// ingest reads the workbooks and writes them into the Global draft.
//
// The first workbook that reads cleanly defines the reference header. Every
// other workbook must match it exactly.
func (s *Service) ingest(action string, paths []string, progress Progress, opts ingestOptions) (*ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	cfg := s.cfg.Get()
	res := &ImportResult{
		Action:     action,
		Total:      len(paths),
		Files:      []FileResult{},
		Warnings:   []string{},
		WeekCodes:  []mps.Week{},
		IndexNames: []string{},
		Draft:      true,
	}
	dict := mps.NewStyleDict()
	dxf := mps.NewDxfDict()

	ws, err := s.workspaceLocked()
	if err != nil {
		return nil, err
	}
	var canonical *mps.Header
	templateReady := false
	if opts.add {
		// A draft this week is the base. Otherwise only a completed Global of the
		// *active* week is copied: when the active week is a Local-only week, the
		// Global of an older, already integrated week must not leak into it.
		base := ws.Draft
		if base == nil && ws.Global != nil && ws.Global.WeekCode == ws.Week {
			base = ws.Global
		}
		if base != nil {
			canonical = &mps.Header{IndexNames: base.IndexNames, Weeks: parseWeekCodes(base.WeekCodes)}
			res.WeekCode = base.WeekCode
			res.WeekStart = base.WeekStart
			res.WeekCodes = canonical.Weeks
			res.IndexNames = base.IndexNames
			templateReady = true
		}
	}

	var rows []store.StagedRow
	var staged []store.StagedFile
	var templateSrc string

	for i, p := range paths {
		if progress != nil {
			progress("读取文件", i, len(paths))
		}
		name := filepath.Base(p)
		fr, err := mps.ReadFile(p, mps.ReadOptions{
			LOCFilter:   cfg.GlobalLOCFilter,
			ReadColumns: cfg.ReadColumns,
			Dict:        dict,
			Dxf:         dxf,
		})
		if err != nil {
			s.log.Error("读取", "%s 打开失败: %v", name, err)
			res.Files = append(res.Files, FileResult{Name: name, Path: p, Status: "failed", Err: err.Error()})
			res.Failed++
			staged = append(staged, store.StagedFile{Path: p, Name: name, Status: "failed", Err: err.Error()})
			continue
		}
		if fr.Err != "" {
			s.log.Error("读取", "%s %s", name, fr.Err)
			res.Files = append(res.Files, FileResult{
				Name: name, Path: p, Size: fr.Size, Status: "failed",
				RowsTotal: fr.RowsTotal, Err: fr.Err,
			})
			res.Failed++
			staged = append(staged, store.StagedFile{
				Path: p, Name: name, Size: fr.Size, Status: "failed",
				RowsTotal: fr.RowsTotal, Err: fr.Err,
			})
			continue
		}

		if canonical == nil {
			canonical = &fr.Header
			res.WeekCode = fr.Header.FirstWeekCode()
			res.WeekStart = fr.Header.Weeks[0].StartText()
			res.WeekCodes = fr.Header.Weeks
			res.IndexNames = fr.Header.IndexNames
		} else if diff := headerDiff(*canonical, fr.Header); diff != "" {
			msg := fmt.Sprintf("表头与首个文件不一致（%s），已跳过", diff)
			s.log.Warn("合并", "%s %s", name, msg)
			res.Warnings = append(res.Warnings, name+": "+msg)
			res.Files = append(res.Files, FileResult{
				Name: name, Path: p, Size: fr.Size, Status: "failed",
				RowsTotal: fr.RowsTotal, Err: msg,
			})
			res.Failed++
			staged = append(staged, store.StagedFile{
				Path: p, Name: name, Size: fr.Size, Status: "failed",
				RowsTotal: fr.RowsTotal, WeekCode: fr.Header.FirstWeekCode(), Err: msg,
			})
			continue
		}

		for _, r := range fr.Rows {
			rows = append(rows, store.StagedRow{
				FileName:  name,
				FilePath:  p,
				SourceRow: r.SourceRow,
				Index:     r.Index,
				Weeks:     r.Weeks,
				StyleIDs:  r.StyleIDs,
				CF:        r.CF,
				Comments:  r.Comments,
				CFColors:  r.CFColors,
			})
		}
		res.OK++
		res.RowsKept += fr.RowsKept
		s.log.Info("读取", "%s: 数据行 %d，LOC=%s 命中 %d 行", name, fr.RowsTotal, cfg.GlobalLOCFilter, fr.RowsKept)

		res.Files = append(res.Files, FileResult{
			Name: name, Path: p, Size: fr.Size, Status: "ok",
			RowsTotal: fr.RowsTotal, RowsKept: fr.RowsKept, WeekCode: fr.Header.FirstWeekCode(),
		})
		staged = append(staged, store.StagedFile{
			Path: p, Name: name, Size: fr.Size, Status: "ok",
			RowsTotal: fr.RowsTotal, RowsKept: fr.RowsKept, WeekCode: fr.Header.FirstWeekCode(),
		})

		for _, w := range fr.Warnings {
			s.log.Warn("读取", "%s: %s", name, w)
			res.Warnings = append(res.Warnings, name+": "+w)
		}
		if templateSrc == "" && !templateReady {
			templateSrc = p
			templateReady = true
		}
	}

	if canonical == nil || res.OK == 0 {
		return nil, fmt.Errorf("没有可用的源文件：%d 个文件全部读取失败", res.Failed)
	}
	targetWeek := canonical.FirstWeekCode()
	if ws.Unintegrated && ws.Week != "" && targetWeek != ws.Week {
		return nil, fmt.Errorf("周码 %s 的导入尚未整合；请先整合或丢弃本周导入，再导入周码 %s",
			ws.Week, targetWeek)
	}
	if opts.add && ws.Draft != nil && ws.Draft.WeekCode != targetWeek {
		return nil, fmt.Errorf("当前 Global 导入是周码 %s，与文件 %s 不一致", ws.Draft.WeekCode, targetWeek)
	}
	if progress != nil {
		progress("写入临时数据", len(paths), len(paths))
	}

	var draft *store.Dataset
	if opts.add && ws.Draft != nil {
		draft = ws.Draft
	} else {
		var baseID int64
		if opts.add && ws.Global != nil && ws.Global.WeekCode == targetWeek {
			baseID = ws.Global.ID
		}
		snap, _ := json.Marshal(cfg)
		draft, err = s.db.BeginGlobalImport(targetWeek, res.WeekStart, weekCodes(canonical.Weeks),
			canonical.IndexNames, string(snap), baseID)
		if err != nil {
			return nil, err
		}
	}
	if err := s.db.AddDraftData(draft.ID, staged, rows, opts.replace, dict, dxf); err != nil {
		s.log.Error("临时数据", "写入失败: %v", err)
		return nil, err
	}
	if templateSrc != "" {
		if err := s.storeTemplate(targetWeek, templateSrc); err != nil {
			s.log.Warn("模板", "保存导出模板失败: %v", err)
			res.Warnings = append(res.Warnings, "保存导出模板失败: "+err.Error())
		}
	}
	s.activateLocked(targetWeek)
	res.DatasetID = draft.ID
	res.RowCount = len(rows)
	res.FileCount = res.OK
	res.DurationMS = time.Since(start).Milliseconds()
	src := logSource(action)
	label := "Global 导入草稿"
	if baseRows := countBase(ws, opts.add); baseRows > 0 {
		label += fmt.Sprintf("（基于已完成版本 %d 行）", baseRows)
	}
	s.log.Success(src, "已写入%s：%d 个文件，%d 行（周码 %s）；点「完成 Global 导入」后生效",
		label, res.OK, res.RowsKept, targetWeek)
	if len(res.Warnings) > 0 {
		s.log.Warn(src, "共 %d 条提示，详见日志", len(res.Warnings))
	}
	return res, nil
}

func countBase(ws *Workspace, add bool) int {
	if !add || ws.Global == nil {
		return 0
	}
	return ws.Global.RowCount
}

// FinishGlobal completes the open Global draft.
func (s *Service) FinishGlobal(confirmFailures bool) (*FinishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	draft, err := s.db.Draft()
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return nil, fmt.Errorf("没有正在进行的 Global 导入")
	}
	d, err := s.db.FinishDraft(draft.ID, confirmFailures)
	if err != nil {
		s.log.Error("完成导入", "%v", err)
		return nil, err
	}
	s.activateLocked(d.WeekCode)
	expired, integrationAt, err := s.expiredFor(d.WeekCode)
	if err != nil {
		return nil, err
	}
	res := &FinishResult{
		DatasetID: d.ID, WeekCode: d.WeekCode, WeekStart: d.WeekStart, Version: d.Version,
		RowCount: d.RowCount, FileCount: d.FileCount, FailedCount: d.FailedCount,
		WeekCodes: parseWeekCodes(d.WeekCodes), IndexNames: d.IndexNames,
		ImportedAt: d.ImportedAt, Expired: expired, IntegrationAt: integrationAt,
	}
	msg := fmt.Sprintf("周码 %s 的 Global 导入已完成：v%d，%d 个文件，%d 行（周码 %s）",
		d.WeekCode, d.Version, d.FileCount, d.RowCount, d.WeekCode)
	if d.FailedCount > 0 {
		msg += fmt.Sprintf("，%d 个文件读取失败", d.FailedCount)
	}
	s.log.Success("完成导入", "%s", msg)
	if expired {
		s.log.Warn("完成导入", "周码 %s 已有整合记录（%s），新的 Global 版本使其过期；请重新整合",
			d.WeekCode, integrationAt)
	}
	return res, nil
}

// DiscardDraft removes the open Global draft.
func (s *Service) DiscardDraft() (*ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	draft, err := s.db.Draft()
	if err != nil {
		return nil, err
	}
	if draft == nil {
		return &ImportResult{Action: "discard", Files: []FileResult{}}, nil
	}
	if err := s.db.DiscardDraft(draft.ID); err != nil {
		return nil, err
	}
	s.refreshSessionLocked()
	s.log.Success("清空", "已丢弃未完成的 Global 导入（周码 %s，%d 个文件，%d 行）",
		draft.WeekCode, draft.FileCount, draft.RowCount)
	return &ImportResult{
		Action: "discard", WeekCode: draft.WeekCode, DatasetID: draft.ID,
		FileCount: draft.FileCount, RowCount: draft.RowCount,
		Files: []FileResult{},
	}, nil
}

// DiscardWeekImports removes the week's draft and every ready dataset its
// integration does not reference. History is untouched.
func (s *Service) DiscardWeekImports(week string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n, err := s.db.DiscardWeekImports(week)
	if err != nil {
		return 0, err
	}
	if s.activeWeek == week {
		s.refreshSessionLocked()
	}
	s.log.Success("丢弃", "已丢弃周码 %s 未整合的导入（%d 份数据集），历史整合记录不受影响", week, n)
	return n, nil
}

// ImportLocal reads a 本地 MPS workbook and stores it as a complete Local
// dataset. It has no draft/finish step: one successful import is the dataset.
func (s *Service) ImportLocal(path string, progress Progress) (*ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	cfg := s.cfg.Get()
	name := filepath.Base(path)

	if progress != nil {
		progress("读取本地文件", 0, 2)
	}
	dict := mps.NewStyleDict()
	local, err := mps.ReadLocalFile(path, mps.ReadOptions{LOCFilter: cfg.LocalLOCFilter, Dict: dict})
	if err != nil {
		s.log.Error("本地", "%s 格式检查未通过: %v", name, err)
		return nil, err
	}
	if len(local.Header.Weeks) == 0 {
		return nil, fmt.Errorf("本地文件没有周列")
	}
	week := local.Header.FirstWeekCode()
	ws, err := s.workspaceLocked()
	if err != nil {
		return nil, err
	}
	if ws.Unintegrated && ws.Week != "" && week != ws.Week {
		return nil, fmt.Errorf("周码 %s 的导入尚未整合；请先整合或丢弃本周导入，再导入周码 %s", ws.Week, week)
	}
	if ws.Draft != nil && ws.Draft.WeekCode != week {
		return nil, fmt.Errorf("当前 Global 导入是周码 %s，与本地文件 %s 不一致", ws.Draft.WeekCode, week)
	}

	rows := make([]store.StagedRow, 0, len(local.Rows))
	for _, r := range local.Rows {
		rows = append(rows, store.StagedRow{
			FileName: name, FilePath: path, SourceRow: r.SourceRow, Index: r.Index, Weeks: r.Weeks,
			StyleIDs: r.StyleIDs, Comments: r.Comments, CFColors: r.CFColors,
		})
	}
	var size int64
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	files := []store.StagedFile{{
		Path: path, Name: name, Size: size, Status: "ok",
		RowsTotal: local.RowsTotal, RowsKept: local.RowsKept, WeekCode: week,
	}}

	if progress != nil {
		progress("写入 Local 数据", 1, 2)
	}
	snap, _ := json.Marshal(cfg)
	d, err := s.db.CreateLocalDataset(week, local.Header.Weeks[0].StartText(),
		weekCodes(local.Header.Weeks), local.Header.IndexNames, string(snap), files, rows, dict)
	if err != nil {
		s.log.Error("本地", "写入 Local 数据失败: %v", err)
		return nil, err
	}
	s.activateLocked(week)
	expired, integrationAt, err := s.expiredFor(week)
	if err != nil {
		return nil, err
	}
	res := &ImportResult{
		Action: "local", WeekCode: week, WeekStart: local.Header.Weeks[0].StartText(),
		WeekCodes: local.Header.Weeks, IndexNames: local.Header.IndexNames,
		Total: 1, OK: 1, RowsKept: len(rows), LocalRows: len(rows),
		DatasetID: d.ID, Version: d.Version, RowCount: d.RowCount, FileCount: d.FileCount,
		Expired: expired,
		Files: []FileResult{{
			Name: name, Path: path, Size: size, Status: "ok",
			RowsTotal: local.RowsTotal, RowsKept: local.RowsKept, WeekCode: week,
		}},
		Warnings: []string{}, DurationMS: time.Since(start).Milliseconds(),
	}
	s.log.Success("本地", "已导入 %s：Local v%d，%d 行（LOC=%s 命中 %d/%d 行），周码 %s",
		name, d.Version, d.RowCount, cfg.LocalLOCFilter, local.RowsKept, local.RowsTotal, week)
	if expired {
		s.log.Warn("本地", "周码 %s 的 Local 版本使已有整合（%s）过期；请重新整合", week, integrationAt)
	}
	return res, nil
}

// ---------------------------------------------------------------- integrate

// Commit combines the current Global and Local datasets into the week's
// integration. 整合 is the only place the two inputs are joined.
func (s *Service) Commit(weekLimit int) (*store.IntegrationResult, bool, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	ws, err := s.workspaceLocked()
	if err != nil {
		return nil, false, false, err
	}
	if ws.Draft != nil {
		return nil, false, false, fmt.Errorf("请先完成或丢弃当前 Global 导入，再整合")
	}
	if ws.Global == nil {
		return nil, false, false, fmt.Errorf("周码 %s 还没有完成导入的 Global 数据", ws.Week)
	}
	if ws.Local == nil {
		return nil, false, false, fmt.Errorf("周码 %s 还没有导入 Local 数据", ws.Week)
	}
	if ws.Global.WeekCode != ws.Local.WeekCode {
		return nil, false, false, fmt.Errorf("Global 是周码 %s，Local 是周码 %s，不能整合",
			ws.Global.WeekCode, ws.Local.WeekCode)
	}
	had := ws.Integration != nil
	wasExpired := ws.Expired
	res, err := s.db.Integrate(ws.Week, weekLimit)
	if err != nil {
		s.log.Error("整合", "失败: %v", err)
		return nil, false, false, err
	}
	s.activateLocked(ws.Week)
	for _, warning := range res.Warnings {
		s.log.Warn("整合", "%s", warning)
	}
	s.log.Success("整合", "周码 %s 已整合：截止 %d 周，实际 %d 周，合并 %d 行，Global v%d（%d 行），Local v%d（%d 行）%s",
		ws.Week, res.RequestedWeeks, res.MergedWeeks, res.Merged.RowCount,
		res.Global.Version, res.Global.RowCount, res.Local.Version, res.Local.RowCount,
		map[bool]string{true: "（覆盖原有整合）", false: ""}[had])
	return res, had, wasExpired, nil
}

// ---------------------------------------------------------------- status

// StagingSummary keeps the old name used by the status binding.
func (s *Service) StagingSummary() (*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workspaceLocked()
}

// ActivateWeek makes a week with persisted data the current workbench again,
// without re-reading any source workbook. It is the "继续本周" path from history:
// completed Global/Local datasets and an existing integration are already in the
// database, so the user can carry on editing, import the missing side, or press
// 整合 immediately.
//
// An unfinished Global draft is the one thing that cannot be left behind: drafts
// are not in history, so switching away would strand them invisibly. Completed
// inputs are safe to leave for another week; they remain in the database.
func (s *Service) ActivateWeek(week string) (*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !store.ValidWeekCode(week) {
		return nil, fmt.Errorf("非法周码 %q", week)
	}
	draft, err := s.db.Draft()
	if err != nil {
		return nil, err
	}
	if draft != nil && draft.WeekCode != week {
		return nil, fmt.Errorf("当前还有未完成的 Global 导入（周码 %s），请先完成或清空后再切换", draft.WeekCode)
	}
	global, err := s.db.Current(week, store.DatasetGlobal)
	if err != nil {
		return nil, err
	}
	local, err := s.db.Current(week, store.DatasetLocal)
	if err != nil {
		return nil, err
	}
	integration, err := s.db.Integration(week)
	if err != nil {
		return nil, err
	}
	if draft == nil && global == nil && local == nil && integration == nil {
		return nil, fmt.Errorf("周码 %s 没有可继续的数据", week)
	}
	s.activateLocked(week)
	ws, err := s.workspaceLocked()
	if err != nil {
		return nil, err
	}
	s.log.Info("工作台", "已切换到周码 %s（Global %s / Local %s）", week,
		datasetLabel(global), datasetLabel(local))
	return ws, nil
}

func datasetLabel(d *store.Dataset) string {
	if d == nil {
		return "无"
	}
	return fmt.Sprintf("v%d %d 行", d.Version, d.RowCount)
}

// Workspace returns the current week's datasets and integration. On a fresh
// launch it is empty: the workbench never reopens last session's imports.
func (s *Service) Workspace() (*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workspaceLocked()
}

func (s *Service) workspaceLocked() (*Workspace, error) {
	if !s.sessionActive || s.activeWeek == "" {
		return &Workspace{}, nil
	}
	week := s.activeWeek
	draft, err := s.db.Draft()
	if err != nil {
		return nil, err
	}
	if draft != nil {
		week = draft.WeekCode
	}
	global, err := s.db.Current(week, store.DatasetGlobal)
	if err != nil {
		return nil, err
	}
	local, err := s.db.Current(week, store.DatasetLocal)
	if err != nil {
		return nil, err
	}
	integration, err := s.db.Integration(week)
	if err != nil {
		return nil, err
	}
	if draft == nil && global == nil && local == nil && integration == nil {
		// The session's data has gone (for example the draft was discarded and
		// there was nothing else this week): the workbench is empty again.
		s.sessionActive = false
		s.activeWeek = ""
		return &Workspace{}, nil
	}
	s.activeWeek = week
	expired := integrationExpired(integration, global, local)
	unintegrated := draft != nil
	if integration == nil {
		unintegrated = unintegrated || global != nil || local != nil
	} else {
		if global != nil && (!integration.GlobalID.Valid || integration.GlobalID.Int64 != global.ID) {
			unintegrated = true
		}
		if local != nil && (!integration.LocalID.Valid || integration.LocalID.Int64 != local.ID) {
			unintegrated = true
		}
	}
	return &Workspace{
		Week: week, Draft: draft, Global: global, Local: local,
		Integration: integration, Expired: expired, Unintegrated: unintegrated,
	}, nil
}

// activateLocked makes a week the current workbench. Callers hold s.mu.
func (s *Service) activateLocked(week string) {
	if week == "" {
		return
	}
	s.sessionActive = true
	s.activeWeek = week
}

// refreshSessionLocked clears the workbench when its week has no data left.
// Callers hold s.mu.
func (s *Service) refreshSessionLocked() {
	if !s.sessionActive || s.activeWeek == "" {
		return
	}
	if draft, err := s.db.Draft(); err == nil && draft != nil {
		s.activeWeek = draft.WeekCode
		return
	}
	global, err := s.db.Current(s.activeWeek, store.DatasetGlobal)
	if err != nil {
		return
	}
	local, err := s.db.Current(s.activeWeek, store.DatasetLocal)
	if err != nil {
		return
	}
	integration, err := s.db.Integration(s.activeWeek)
	if err != nil {
		return
	}
	if global == nil && local == nil && integration == nil {
		s.sessionActive = false
		s.activeWeek = ""
	}
}

func integrationExpired(in *store.Integration, global, local *store.Dataset) bool {
	if in == nil {
		return false
	}
	if global != nil && (!in.GlobalID.Valid || in.GlobalID.Int64 != global.ID) {
		return true
	}
	if local != nil && (!in.LocalID.Valid || in.LocalID.Int64 != local.ID) {
		return true
	}
	return false
}

func (s *Service) expiredFor(week string) (bool, string, error) {
	in, err := s.db.Integration(week)
	if err != nil || in == nil {
		return false, "", err
	}
	global, err := s.db.Current(week, store.DatasetGlobal)
	if err != nil {
		return false, "", err
	}
	local, err := s.db.Current(week, store.DatasetLocal)
	if err != nil {
		return false, "", err
	}
	return integrationExpired(in, global, local), in.IntegratedAt, nil
}

// Files returns the workbooks behind the current draft/Global and Local imports.
func (s *Service) Files() ([]FileWithDataset, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ws, err := s.workspaceLocked()
	if err != nil {
		return nil, err
	}
	out := []FileWithDataset{}
	appendFiles := func(d *store.Dataset, label string) error {
		if d == nil {
			return nil
		}
		files, err := s.db.DatasetFiles(d.ID)
		if err != nil {
			return err
		}
		for _, f := range files {
			out = append(out, FileWithDataset{
				Name: f.Name, Path: f.Path, Size: f.Size, Status: f.Status,
				RowsTotal: f.RowsTotal, RowsKept: f.RowsKept, WeekCode: f.WeekCode,
				Err: f.Err, Dataset: label, Version: d.Version,
			})
		}
		return nil
	}
	if ws.Draft != nil {
		// The draft state belongs to the import, not to the dataset column: the
		// file table shows Global on the left and 导入中 in its status column.
		if err := appendFiles(ws.Draft, "Global"); err != nil {
			return nil, err
		}
	} else if err := appendFiles(ws.Global, "Global"); err != nil {
		return nil, err
	}
	// Local deliberately has no entry here: it is one workbook, and its import is
	// complete the moment it is opened. The status strip shows it instead.
	return out, nil
}

// FileWithDataset is one workbook record with the dataset it belongs to.
type FileWithDataset struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Status    string `json:"status"`
	RowsTotal int    `json:"rowsTotal"`
	RowsKept  int    `json:"rowsKept"`
	WeekCode  string `json:"weekCode"`
	Err       string `json:"err,omitempty"`
	Dataset   string `json:"dataset"`
	Version   int    `json:"version"`
}

// Archive returns the integrated weeks as three dataset entries each.
func (s *Service) Archive() ([]ArchiveEntry, error) {
	// History is every week with a completed dataset, not only integrated weeks:
	// 完成 Global 导入 and a Local import are already final datasets.
	weeks, err := s.db.WeeksWithReadyData()
	if err != nil {
		return nil, err
	}
	out := []ArchiveEntry{}
	for _, week := range weeks {
		global, err := s.db.Current(week, store.DatasetGlobal)
		if err != nil {
			return nil, err
		}
		local, err := s.db.Current(week, store.DatasetLocal)
		if err != nil {
			return nil, err
		}
		integration, err := s.db.Integration(week)
		if err != nil {
			return nil, err
		}
		expired := integrationExpired(integration, global, local)

		// The merged row exists only once the week is integrated, and is the only
		// row that can be outdated. Global and Local show their current versions;
		// they are there as soon as they are completed.
		type item struct {
			kind    string
			id      int64
			expired bool
		}
		var items []item
		if integration != nil {
			items = append(items, item{store.DatasetMerged, integration.MergedID, expired})
		}
		if global != nil {
			items = append(items, item{store.DatasetGlobal, global.ID, false})
		}
		if local != nil {
			items = append(items, item{store.DatasetLocal, local.ID, false})
		}
		for _, it := range items {
			d, err := s.db.Dataset(it.id)
			if err != nil || d == nil {
				continue
			}
			w, _ := mps.ParseWeekCode(d.WeekCode)
			out = append(out, ArchiveEntry{
				WeekCode: d.WeekCode, WeekStart: d.WeekStart,
				Year: w.FullYear(), WeekNo: w.WeekNo,
				Dataset: it.kind, DatasetID: d.ID, Version: d.Version,
				RowCount: d.RowCount, FileCount: d.FileCount,
				ImportedAt: d.ImportedAt, Expired: it.expired,
			})
		}
	}
	return out, nil
}

// Years lists the years with an integrated week.
func (s *Service) Years() ([]int, error) {
	list, err := s.Archive()
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	out := []int{}
	for _, e := range list {
		if e.Year > 0 && !seen[e.Year] {
			seen[e.Year] = true
			out = append(out, e.Year)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out, nil
}

// Weeks lists the ISO weeks of a year that have an integration.
func (s *Service) Weeks(year int) ([]int, error) {
	list, err := s.Archive()
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	out := []int{}
	for _, e := range list {
		if e.Year == year && !seen[e.WeekNo] {
			seen[e.WeekNo] = true
			out = append(out, e.WeekNo)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out, nil
}

// GridSource describes one dataset for the grid header.
type GridSource struct {
	Dataset   *store.Dataset
	WeekCodes []mps.Week
	Expired   bool
}

// GridSource resolves a dataset id for the grid.
func (s *Service) GridSource(datasetID int64) (*GridSource, error) {
	if datasetID <= 0 {
		return &GridSource{}, nil
	}
	d, err := s.db.Dataset(datasetID)
	if err != nil || d == nil {
		return &GridSource{}, err
	}
	expired, _, err := s.expiredFor(d.WeekCode)
	if err != nil {
		return nil, err
	}
	// Only the merged dataset is the integration; a newer Global/Local dataset is
	// current and must not be labelled expired just because the week's merged
	// result is.
	current := expired && d.Kind == store.DatasetMerged
	return &GridSource{Dataset: d, WeekCodes: parseWeekCodes(d.WeekCodes), Expired: current}, nil
}

// Query runs a grid query for one dataset.
func (s *Service) Query(q store.DatasetQuery) (*store.GridResult, error) {
	return s.db.QueryDataset(q)
}

// ---------------------------------------------------------------- export

// ExportOptions selects the single dataset to export and how.
type ExportOptions struct {
	DatasetID int64
	DestPath  string
	Mode      config.ExportMode
}

// Export writes one dataset as the single-sheet report.
func (s *Service) Export(opt ExportOptions, progress Progress) (*mps.ExportStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.cfg.Get()
	mode := opt.Mode
	if mode == "" {
		mode = cfg.ExportMode
	}
	clean := mode == config.ExportClean

	if progress != nil {
		progress("读取数据", 0, 1)
	}
	entry, rows, err := s.db.ExportRows(opt.DatasetID)
	if err != nil {
		s.log.Error("导出", "读取数据失败: %v", err)
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("该数据集没有可导出的数据")
	}
	styles, err := s.styleMap()
	if err != nil {
		return nil, err
	}
	dxfStyles, dxfIndex, err := s.dxfMap()
	if err != nil {
		return nil, err
	}
	templatePath, err := s.templateFor(entry.WeekCode)
	if err != nil {
		return nil, err
	}
	fallbackTemplate := false
	if templatePath == "" {
		// A Local-only week has no Global source workbook to use as the template.
		// A blank workbook is enough for a clean rebuild, but it cannot satisfy
		// the template engine's header copy, so this path exports clean.
		fallbackTemplate = true
		tmp, err := os.CreateTemp("", "clear-template-*.xlsx")
		if err != nil {
			return nil, fmt.Errorf("创建导出模板失败: %w", err)
		}
		tmpPath := tmp.Name()
		tmp.Close()
		_ = os.Remove(tmpPath)
		if err := mps.WriteBlankTemplate(tmpPath); err != nil {
			return nil, fmt.Errorf("生成空白导出模板失败: %w", err)
		}
		defer os.Remove(tmpPath)
		templatePath = tmpPath
		s.log.Warn("导出", "周码 %s 没有 Global 源模板（只有 Local 数据），按纯数据格式导出", entry.WeekCode)
	}
	exportRows := make([]mps.ExportRow, 0, len(rows))
	for _, r := range rows {
		exportRows = append(exportRows, mps.ExportRow{
			Index: r.Index, Weeks: r.Weeks, StyleIDs: r.StyleIDs,
			CF: r.CF, Comments: r.Comments, CFColors: r.CFColors,
			SourceRow: r.SourceRow, FileName: r.FileName,
		})
	}
	if progress != nil {
		progress("写入文件", 1, 2)
	}
	effectiveClean := clean || fallbackTemplate
	exportStart := time.Now()
	stats, err := mps.Export(mps.ExportInput{
		TemplatePath: templatePath,
		DestPath:     opt.DestPath,
		Weeks:        parseWeekCodes(entry.WeekCodes),
		IndexNames:   entry.IndexNames,
		Rows:         exportRows,
		StyleByID:    styles,
		DxfStyles:    dxfStyles,
		DxfIndex:     dxfIndex,
		Clean:        effectiveClean,
	})
	if err != nil {
		s.log.Error("导出", "失败: %v", err)
		return nil, err
	}
	stats.DurationMS = time.Since(exportStart).Milliseconds()
	if progress != nil {
		progress("完成", 2, 2)
	}
	label := "原文件格式（仅 MPS 页）"
	if effectiveClean {
		label = "纯数据（仅 MPS 页）"
	}
	s.log.Success("导出", "%s v%d（%s）→ %s（%s，%d 行 × %d 列，条件格式 %d 行，注释 %d 条，宏保留 %v）",
		entry.WeekCode, entry.Version, entry.Kind, opt.DestPath, label,
		stats.Rows, stats.Cols, stats.CFRows, stats.Comments, stats.PreservedVBA)
	for _, w := range stats.Warnings {
		s.log.Warn("导出", "%s", w)
	}
	return stats, nil
}

// BundleExportOptions selects the week whose integration is written as a
// three-table workbook.
type BundleExportOptions struct {
	WeekCode string
	DestPath string
}

// ExportBundle writes the integrated week's MPS / Local / Global workbook. It
// exports the integration even when a newer import has made it expired; the
// caller is told so in the result.
func (s *Service) ExportBundle(opt BundleExportOptions, progress Progress) (*mps.ExportStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	week := opt.WeekCode
	if week == "" {
		ws, err := s.workspaceLocked()
		if err != nil {
			return nil, err
		}
		week = ws.Week
	}
	if !store.ValidWeekCode(week) {
		return nil, fmt.Errorf("非法周码 %q", week)
	}
	in, err := s.db.Integration(week)
	if err != nil {
		return nil, err
	}
	if in == nil {
		return nil, fmt.Errorf("周码 %s 还没有整合", week)
	}
	parts := []struct {
		sheet string
		id    int64
	}{
		{"MPS", in.MergedID},
		{"Local", in.LocalID.Int64},
		{"Global", in.GlobalID.Int64},
	}
	if !in.GlobalID.Valid || !in.LocalID.Valid {
		return nil, fmt.Errorf("周码 %s 是旧版整合（只有合并结果），无法导出三表；请在历史数据里单独导出", week)
	}
	if progress != nil {
		progress("读取数据", 0, 2)
	}
	styles, err := s.styleMap()
	if err != nil {
		return nil, err
	}
	merged, err := s.db.Dataset(in.MergedID)
	if err != nil {
		return nil, err
	}
	if merged == nil {
		return nil, fmt.Errorf("周码 %s 的整合数据不存在", week)
	}
	targetWeeks := parseWeekCodes(merged.WeekCodes)
	sheets := make([]mps.BundleSheet, 0, 3)
	for _, p := range parts {
		d, rows, err := s.db.ExportRows(p.id)
		if err != nil {
			return nil, err
		}
		var bundleRows []mps.ExportRow
		if p.id == in.MergedID {
			bundleRows = toMPSRows(rows)
		} else {
			bundleRows = trimExportRows(rows, parseWeekCodes(d.WeekCodes), targetWeeks)
		}
		sheets = append(sheets, mps.BundleSheet{
			Name: p.sheet, Weeks: targetWeeks,
			IndexNames: d.IndexNames, Rows: bundleRows, StyleByID: styles,
		})
	}
	if progress != nil {
		progress("写入文件", 1, 2)
	}
	start := time.Now()
	stats, err := mps.ExportBundle(mps.BundleInput{DestPath: opt.DestPath, Sheets: sheets})
	if err != nil {
		s.log.Error("导出", "三表文件写入失败: %v", err)
		return nil, err
	}
	stats.DurationMS = time.Since(start).Milliseconds()
	expired, _, err := s.expiredFor(week)
	if err != nil {
		return nil, err
	}
	stats.Expired = expired
	if progress != nil {
		progress("完成", 2, 2)
	}
	s.log.Success("导出", "三表文件 %s → %s（MPS %d 行 / Local %d 行 / Global %d 行）",
		week, opt.DestPath, len(sheets[0].Rows), len(sheets[1].Rows), len(sheets[2].Rows))
	if expired {
		s.log.Warn("导出", "该周已有更新的导入，整合已过期；导出的是上次整合结果")
	}
	return stats, nil
}

// ExportWeek names the week the toolbar export would write.
func (s *Service) ExportWeek(weekCode string) string {
	if weekCode != "" && store.ValidWeekCode(weekCode) {
		return weekCode
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ws, err := s.workspaceLocked()
	if err == nil {
		if ws.Week != "" {
			return ws.Week
		}
	}
	return ""
}

func (s *Service) templateFor(week string) (string, error) {
	tpl, err := s.db.TemplateFor(week)
	if err != nil {
		return "", err
	}
	if tpl != nil {
		if _, statErr := os.Stat(tpl.StoredPath); statErr == nil {
			return tpl.StoredPath, nil
		}
		s.log.Warn("导出", "导出模板缺失，改用原目录中的源文件")
	}
	dir := filepath.Join(s.templateDir, week)
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		path := filepath.Join(dir, entries[0].Name())
		s.log.Info("导出", "使用模板 %s", filepath.Base(path))
		return path, nil
	}
	return "", nil
}

func (s *Service) styleMap() (func(int) (json.RawMessage, bool), error) {
	list, err := s.db.LoadStyles()
	if err != nil {
		return nil, err
	}
	m := make(map[int]json.RawMessage, len(list))
	for _, e := range list {
		m[e.ID] = e.Sig
	}
	s.log.Info("导出", "已载入 %d 条单元格样式", len(m))
	return func(id int) (json.RawMessage, bool) {
		v, ok := m[id]
		return v, ok
	}, nil
}

func (s *Service) dxfMap() ([]string, func(int) (int, bool), error) {
	stored, err := s.db.LoadDxfStyles()
	if err != nil {
		return nil, nil, err
	}
	list := make([]string, len(stored))
	at := make(map[int]int, len(stored))
	for i, d := range stored {
		list[i] = d.XML
		at[d.ID] = i
	}
	if len(list) > 0 {
		s.log.Info("导出", "已载入 %d 条条件格式配色", len(list))
	}
	return list, func(id int) (int, bool) {
		i, ok := at[id]
		return i, ok
	}, nil
}

// WaitIdle blocks until the operation in progress has finished.
func (s *Service) WaitIdle() { s.mu.Lock(); s.mu.Unlock() }

// ---------------------------------------------------------------- helpers

func logSource(action string) string {
	switch action {
	case "import":
		return "导入"
	case "add":
		return "添加"
	}
	return action
}

func weekCodes(weeks []mps.Week) []string {
	out := make([]string, len(weeks))
	for i, w := range weeks {
		out[i] = w.Code
	}
	return out
}

func parseWeekCodes(codes []string) []mps.Week {
	out := make([]mps.Week, 0, len(codes))
	for _, c := range codes {
		if w, err := mps.ParseWeekCode(c); err == nil {
			out = append(out, w)
		}
	}
	return out
}

func headerDiff(a, b mps.Header) string {
	if a.Signature() != b.Signature() {
		for i := range a.IndexNames {
			if i >= len(b.IndexNames) || a.IndexNames[i] != b.IndexNames[i] {
				got := "(缺失)"
				if i < len(b.IndexNames) {
					got = b.IndexNames[i]
				}
				return fmt.Sprintf("第 %d 列表头 %q != %q", i+1, got, a.IndexNames[i])
			}
		}
		return "索引列数量不同"
	}
	if a.WeekSignature() != b.WeekSignature() {
		return fmt.Sprintf("周列 %s != %s", b.WeekSignature(), a.WeekSignature())
	}
	return ""
}

func (s *Service) storeTemplate(weekCode, src string) error {
	dir := filepath.Join(s.templateDir, weekCode)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, filepath.Base(src))
	if err := copyFile(src, dst); err != nil {
		return err
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == filepath.Base(src) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
	return nil
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}

// trimExportRows keeps only the weeks selected by the integration, so the Local
// and Global sheets have the same week columns as the merged MPS sheet. Values,
// styles, conditional colours and comments follow the weeks they belong to.
func trimExportRows(rows []store.ExportRow, from, to []mps.Week) []mps.ExportRow {
	at := map[string]int{}
	for i, w := range from {
		at[w.Code] = i
	}
	out := make([]mps.ExportRow, 0, len(rows))
	for _, r := range rows {
		nr := mps.ExportRow{
			Index: r.Index, Weeks: make([]any, len(to)), CF: r.CF,
			SourceRow: r.SourceRow, FileName: r.FileName,
		}
		if len(r.StyleIDs) > 0 {
			nr.StyleIDs = make([]int, mps.IndexCols+len(to))
			copy(nr.StyleIDs, r.StyleIDs[:minInt(len(r.StyleIDs), mps.IndexCols)])
		}
		if len(r.CFColors) > 0 {
			nr.CFColors = make([]string, len(to))
		}
		if len(r.Comments) > 0 {
			nr.Comments = mps.CommentMap{}
			for col, c := range r.Comments {
				if col <= mps.IndexCols {
					nr.Comments[col] = c
				}
			}
		}
		for i, w := range to {
			j, ok := at[w.Code]
			if !ok {
				continue
			}
			if j < len(r.Weeks) {
				nr.Weeks[i] = r.Weeks[j]
			}
			if nr.StyleIDs != nil {
				if k := mps.IndexCols + j; k < len(r.StyleIDs) {
					nr.StyleIDs[mps.IndexCols+i] = r.StyleIDs[k]
				}
			}
			if nr.CFColors != nil && j < len(r.CFColors) {
				nr.CFColors[i] = r.CFColors[j]
			}
			if len(r.Comments) > 0 {
				if c, ok := r.Comments[mps.IndexCols+1+j]; ok {
					nr.Comments[mps.IndexCols+1+i] = c
				}
			}
		}
		out = append(out, nr)
	}
	return out
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func toMPSRows(rows []store.ExportRow) []mps.ExportRow {
	out := make([]mps.ExportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, mps.ExportRow{
			Index: r.Index, Weeks: r.Weeks, StyleIDs: r.StyleIDs,
			CF: r.CF, Comments: r.Comments, CFColors: r.CFColors,
			SourceRow: r.SourceRow, FileName: r.FileName,
		})
	}
	return out
}
