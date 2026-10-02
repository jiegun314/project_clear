// Package service orchestrates the application's use cases: importing a folder
// of source workbooks, adding individual files, committing the merged result to
// a permanent weekly table, and exporting it again.
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

// ImportResult summarises one import or add action.
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
	// NeedsConfirm is set by 添加 when one of the picked workbooks is already in
	// the staging list: the caller asks the user before its rows are replaced.
	NeedsConfirm   bool     `json:"needsConfirm,omitempty"`
	DuplicateFiles []string `json:"duplicateFiles,omitempty"`
	PendingPaths   []string `json:"pendingPaths,omitempty"`
}

// Progress reports how far a long operation has got.
type Progress func(stage string, done, total int)

// Service is the application layer.
type Service struct {
	cfg *config.Store
	log *logging.Logger
	db  *store.Store

	dataDir     string
	templateDir string

	mu sync.Mutex
}

// New wires the service to its database, log and configuration.
func New(cfg *config.Store, log *logging.Logger, db *store.Store, dataDir string) *Service {
	return &Service{
		cfg:         cfg,
		log:         log,
		db:          db,
		dataDir:     dataDir,
		templateDir: filepath.Join(dataDir, "templates"),
	}
}

// DataDir is the folder holding the database, templates and logs.
func (s *Service) DataDir() string { return s.dataDir }

// ImportFolder reads every workbook in a folder.
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

// AddFiles reads individually chosen workbooks and merges them into the list
// that is already staged: adding a file grows the current 整合清单, it does not
// start a new one.
//
// A workbook that is already in the list is never added twice. Unless
// overwrite is set, the call stops as soon as one is found and reports
// NeedsConfirm with the offending files; the caller asks the user, then calls
// again with overwrite=true, which replaces exactly those files' rows.
func (s *Service) AddFiles(paths []string, progress Progress, overwrite bool) (*ImportResult, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("未选择任何文件")
	}
	paths = uniquePaths(paths)
	staged, err := s.db.StagingFiles()
	if err != nil {
		return nil, err
	}
	stagedPaths := map[string]bool{}
	for _, f := range staged {
		stagedPaths[filepath.Clean(f.Path)] = true
	}
	var duplicates []string
	for _, p := range paths {
		if stagedPaths[filepath.Clean(p)] {
			duplicates = append(duplicates, p)
		}
	}
	if len(duplicates) > 0 && !overwrite {
		names := make([]string, len(duplicates))
		for i, p := range duplicates {
			names[i] = filepath.Base(p)
		}
		s.log.Warn("添加", "%d 个文件已在整合清单中，等待确认是否覆盖", len(duplicates))
		return &ImportResult{
			Action:         "add",
			Total:          len(paths),
			Files:          []FileResult{},
			Warnings:       []string{},
			WeekCodes:      []mps.Week{},
			IndexNames:     []string{},
			NeedsConfirm:   true,
			DuplicateFiles: names,
			PendingPaths:   duplicates,
		}, nil
	}
	s.log.Info("添加", "添加 %d 个源文件", len(paths))
	return s.ingest("add", paths, progress, ingestOptions{merge: true, replace: duplicates})
}

// uniquePaths drops repeated picks of the same workbook (the file dialog can
// return the same file twice) and normalises the spelling of the path.
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

// ingestOptions tunes how the result is written: 导入文件夹 starts a fresh
// batch, 添加 merges into the staged one and replaces the listed files.
type ingestOptions struct {
	merge   bool
	replace []string
}

// ingest reads each workbook and stages the merged result.
//
// The first workbook that reads cleanly defines the reference header. Every
// other workbook must match it exactly: a different index block or a different
// set of week columns is reported as a failure and skipped rather than being
// silently realigned.
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
	}
	dict := mps.NewStyleDict()
	dxf := mps.NewDxfDict()

	var canonical *mps.Header
	templateReady := false
	// 添加 merges into the batch that is already staged, so the staged header
	// is the reference every added workbook has to match.
	if opts.merge {
		sum, err := s.db.LoadStaging()
		if err != nil {
			return nil, err
		}
		if sum.HasStaging {
			canonical = &mps.Header{IndexNames: sum.IndexNames, Weeks: parseWeekCodes(sum.WeekCodes)}
			res.WeekCode = sum.WeekCode
			res.WeekStart = sum.WeekStart
			res.WeekCodes = canonical.Weeks
			res.IndexNames = sum.IndexNames
			templateReady = sum.TemplateID.Valid
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
			LOCFilter:   cfg.LOCFilter,
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
			})
		}
		res.OK++
		res.RowsKept += fr.RowsKept
		s.log.Info("读取", "%s: 数据行 %d，LOC=%s 命中 %d 行", name, fr.RowsTotal, cfg.LOCFilter, fr.RowsKept)

		res.Files = append(res.Files, FileResult{
			Name: name, Path: p, Size: fr.Size, Status: "ok",
			RowsTotal: fr.RowsTotal, RowsKept: fr.RowsKept, WeekCode: fr.Header.FirstWeekCode(),
		})
		staged = append(staged, store.StagedFile{
			Path: p, Name: name, Size: fr.Size, Status: "ok",
			RowsTotal: fr.RowsTotal, RowsKept: fr.RowsKept, WeekCode: fr.Header.FirstWeekCode(),
		})

		// The first workbook that reads cleanly also becomes the export
		// template, so the exported file looks like a real MPS report.
		if templateSrc == "" && !templateReady {
			if err := s.storeTemplate(fr.Header.FirstWeekCode(), p); err != nil {
				s.log.Warn("模板", "保存导出模板失败: %v", err)
				res.Warnings = append(res.Warnings, "保存导出模板失败: "+err.Error())
			} else {
				templateSrc = p
				templateReady = true
			}
		}
	}

	if canonical == nil {
		return nil, fmt.Errorf("没有可用的源文件：%d 个文件全部读取失败", res.Failed)
	}
	if res.OK == 0 {
		return nil, fmt.Errorf("没有可用的源文件：%d 个文件全部读取失败", res.Failed)
	}
	if progress != nil {
		progress("写入临时数据", len(paths), len(paths))
	}

	snap, _ := json.Marshal(cfg)
	input := store.StagingInput{
		Action:      action,
		WeekCode:    res.WeekCode,
		WeekStart:   res.WeekStart,
		WeekCodes:   weekCodes(canonical.Weeks),
		IndexNames:  canonical.IndexNames,
		Files:       staged,
		Rows:        rows,
		Dict:        dict,
		Dxf:         dxf,
		TemplateSrc: templateSrc,
		TemplateNam: filepath.Base(templateSrc),
		ParamSnap:   string(snap),
	}
	var writeErr error
	if opts.merge {
		_, writeErr = s.db.MergeStaging(input, opts.replace)
	} else {
		_, writeErr = s.db.SaveStaging(input)
	}
	if writeErr != nil {
		s.log.Error("临时数据", "写入失败: %v", writeErr)
		return nil, writeErr
	}

	res.DurationMS = time.Since(start).Milliseconds()
	s.log.Success(action, "完成：成功 %d 个文件，失败 %d 个，合并 %d 行（周码 %s），耗时 %dms",
		res.OK, res.Failed, res.RowsKept, res.WeekCode, res.DurationMS)
	if len(res.Warnings) > 0 {
		s.log.Warn(action, "共 %d 条提示，详见日志", len(res.Warnings))
	}
	return res, nil
}

func weekCodes(weeks []mps.Week) []string {
	out := make([]string, len(weeks))
	for i, w := range weeks {
		out[i] = w.Code
	}
	return out
}

// parseWeekCodes turns the stored week codes of a staged batch back into the
// header shape the merger compares each added workbook against.
func parseWeekCodes(codes []string) []mps.Week {
	out := make([]mps.Week, 0, len(codes))
	for _, c := range codes {
		if w, err := mps.ParseWeekCode(c); err == nil {
			out = append(out, w)
		}
	}
	return out
}

// headerDiff describes how two headers differ, or returns "" when they match.
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

// storeTemplate keeps a copy of a source workbook to export from later.
func (s *Service) storeTemplate(weekCode, src string) error {
	dir := filepath.Join(s.templateDir, weekCode)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dir, filepath.Base(src))
	if err := copyFile(src, dst); err != nil {
		return err
	}
	// Older copies of the same week are no longer needed.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if e.Name() == filepath.Base(src) {
			continue
		}
		_ = os.Remove(filepath.Join(dir, e.Name()))
	}
	return nil
}

// Commit promotes the staged rows into the permanent weekly table.
func (s *Service) Commit() (*store.ArchiveEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sum, err := s.db.LoadStaging()
	if err != nil {
		return nil, err
	}
	if !sum.HasStaging {
		return nil, fmt.Errorf("没有待整合的临时数据，请先导入或添加文件")
	}
	entry, err := s.db.Commit(sum.WeekCode)
	if err != nil {
		s.log.Error("整合", "失败: %v", err)
		return nil, err
	}
	s.log.Success("整合", "周码 %s（%s）已入库，共 %d 行，覆盖原有数据",
		entry.WeekCode, entry.WeekStart, entry.RowCount)
	return entry, nil
}

// ClearStaging removes everything an import staged but the user never
// integrated — the merged rows, the per-file records, and the template copy
// kept for a week that was not committed. It reports how many rows went away,
// and does nothing when there is no staging batch.
func (s *Service) ClearStaging() (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sum, err := s.db.LoadStaging()
	if err != nil {
		return 0, err
	}
	if !sum.HasStaging {
		return 0, nil
	}

	orphaned, err := s.db.ClearStaging()
	if err != nil {
		s.log.Error("清空", "清空临时数据失败: %v", err)
		return 0, err
	}
	for _, wc := range orphaned {
		if err := os.RemoveAll(filepath.Join(s.templateDir, wc)); err != nil {
			s.log.Warn("清空", "删除临时导出模板 %s 失败: %v", wc, err)
		}
	}

	s.log.Success("清空", "已清空临时数据：周码 %s，共 %d 行（整合入库的数据不受影响）", sum.WeekCode, sum.RowKept)
	return sum.RowKept, nil
}

// ExportOptions selects what to export and how.
type ExportOptions struct {
	// WeekCode is the committed week to export; empty exports the staging area.
	WeekCode string
	// DestPath is the destination file.
	DestPath string
	// Mode forces an export engine. Empty uses the configured default.
	Mode config.ExportMode
}

// ExportWeek reports which week an export request would write, so the save
// dialog can offer a sensible file name before the work starts. It returns ""
// when there is nothing to export.
func (s *Service) ExportWeek(weekCode string) string {
	if weekCode != "" {
		if store.ValidWeekCode(weekCode) {
			return weekCode
		}
		return ""
	}
	if sum, err := s.db.LoadStaging(); err == nil && sum.HasStaging {
		return sum.WeekCode
	}
	if list, err := s.db.ListArchive(); err == nil && len(list) > 0 {
		return list[0].WeekCode
	}
	return ""
}

// Export writes a committed week back out as a workbook.
func (s *Service) Export(opt ExportOptions, progress Progress) (*mps.ExportStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := s.cfg.Get()
	mode := opt.Mode
	if mode == "" {
		mode = cfg.ExportMode
	}
	clean := mode == config.ExportClean

	weekCode := opt.WeekCode
	if progress != nil {
		progress("读取数据", 0, 1)
	}

	entry, rows, err := s.resolveExportRows(weekCode)
	if err != nil {
		s.log.Error("导出", "读取数据失败: %v", err)
		return nil, err
	}
	weekCode = entry.WeekCode
	if len(rows) == 0 {
		return nil, fmt.Errorf("周码 %s 没有可导出的数据", weekCode)
	}

	styles, err := s.styleMap()
	if err != nil {
		return nil, err
	}
	dxfStyles, dxfIndex, err := s.dxfMap()
	if err != nil {
		return nil, err
	}

	tpl, err := s.db.TemplateFor(weekCode)
	if err != nil {
		return nil, err
	}
	templatePath := ""
	if tpl != nil {
		if _, statErr := os.Stat(tpl.StoredPath); statErr == nil {
			templatePath = tpl.StoredPath
		} else {
			s.log.Warn("导出", "导出模板缺失，改用原目录中的源文件")
		}
	}
	if templatePath == "" {
		// Fall back to any stored copy for this week.
		dir := filepath.Join(s.templateDir, weekCode)
		if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
			templatePath = filepath.Join(dir, entries[0].Name())
			s.log.Info("导出", "使用模板 %s", filepath.Base(templatePath))
		}
	}

	weeks := make([]mps.Week, 0, len(entry.WeekCodes))
	for _, code := range entry.WeekCodes {
		w, err := mps.ParseWeekCode(code)
		if err != nil {
			return nil, err
		}
		weeks = append(weeks, w)
	}

	exportRows := make([]mps.ExportRow, 0, len(rows))
	for _, r := range rows {
		styleIDs := r.StyleIDs
		exportRows = append(exportRows, mps.ExportRow{
			Index:     r.Index,
			Weeks:     r.Weeks,
			StyleIDs:  styleIDs,
			CF:        r.CF,
			Comments:  r.Comments,
			SourceRow: r.SourceRow,
			FileName:  r.FileName,
		})
	}

	if progress != nil {
		progress("写入文件", 1, 2)
	}
	exportStart := time.Now()
	stats, err := mps.Export(mps.ExportInput{
		TemplatePath: templatePath,
		DestPath:     opt.DestPath,
		Weeks:        weeks,
		IndexNames:   entry.IndexNames,
		Rows:         exportRows,
		StyleByID:    styles,
		DxfStyles:    dxfStyles,
		DxfIndex:     dxfIndex,
		Clean:        clean,
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
	if clean {
		label = "纯数据（仅 MPS 页）"
	}
	s.log.Success("导出", "%s → %s（%s，%d 行 × %d 列，条件格式 %d 行，注释 %d 条，宏保留 %v）",
		weekCode, opt.DestPath, label, stats.Rows, stats.Cols, stats.CFRows, stats.Comments, stats.PreservedVBA)
	if stats.CFRulesDropped > 0 {
		s.log.Warn("导出", "有 %d 处样式或条件格式因源数据缺失未能还原", stats.CFRulesDropped)
	}
	return stats, nil
}

// resolveExportRows decides what 导出 writes.
//
// A named week (the history panel, or an explicit choice) always wins. With no
// week named, the export follows the main grid: the staging area while there
// is one, otherwise the newest integrated week. Falling back like this is what
// keeps the toolbar button working after 整合, when the staging batch has been
// promoted and no longer counts as staging.
func (s *Service) resolveExportRows(weekCode string) (*store.ArchiveEntry, []store.ExportRow, error) {
	if weekCode != "" {
		if !store.ValidWeekCode(weekCode) {
			return nil, nil, fmt.Errorf("非法周码 %q", weekCode)
		}
		return s.db.FetchExportRows(weekCode)
	}

	sum, err := s.db.LoadStaging()
	if err != nil {
		return nil, nil, err
	}
	if sum.HasStaging {
		staged, rows, err := s.db.FetchStagingExportRows()
		if err != nil {
			return nil, nil, err
		}
		s.log.Info("导出", "导出尚未整合的临时数据（周码 %s）", staged.WeekCode)
		return &store.ArchiveEntry{
			WeekCode:   staged.WeekCode,
			WeekStart:  staged.WeekStart,
			WeekCodes:  staged.WeekCodes,
			IndexNames: staged.IndexNames,
			RowCount:   len(rows),
			FileCount:  staged.FileOK,
			TableName:  "stg_row",
		}, rows, nil
	}

	list, err := s.db.ListArchive()
	if err != nil {
		return nil, nil, err
	}
	if len(list) == 0 {
		return nil, nil, fmt.Errorf("没有可导出的数据，请先导入文件")
	}
	latest := list[0]
	s.log.Info("导出", "导出已整合的最新一周：%s", latest.WeekCode)
	return s.db.FetchExportRows(latest.WeekCode)
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

// dxfMap loads the stored differential formats: the list the export appends,
// and the mapping from the id a rule was stored with to its place in that list.
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

// Query is the data-grid read path, re-exported so callers outside the service
// package (and the end-to-end audit) do not need to import the store.
func (s *Service) Query(q store.Query) (*store.GridResult, error) { return s.db.QueryRows(q) }
