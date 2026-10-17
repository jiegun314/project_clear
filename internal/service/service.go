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
	// Replaced names the staged workbooks this action overwrote. Re-importing a
	// file replaces the entry with the same name, so the user is told afterwards
	// which ones were affected rather than being asked beforehand.
	Replaced []string `json:"replaced,omitempty"`
	// LocalRows is how many rows the 本地 MPS import brought in. The merged
	// result does not exist until 整合 runs, so this is the only count a local
	// import reports.
	LocalRows int `json:"localRows"`
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
	// A stored payload the store cannot decode leaves its field empty rather than
	// failing the query, and says so here. Without it a missing colour or note is
	// reported nowhere at all.
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
// A workbook that is already in the list is never added twice: the entry with
// the same file name is overwritten, whichever folder this copy came from. The
// caller is not asked first — the requirement is that re-importing replaces the
// previous import outright — so the names that were overwritten come back in
// Replaced for the summary the user sees.
func (s *Service) AddFiles(paths []string, progress Progress) (*ImportResult, error) {
	if len(paths) == 0 {
		return nil, fmt.Errorf("未选择任何文件")
	}
	paths = uniquePaths(paths)
	staged, err := s.db.StagingFiles()
	if err != nil {
		return nil, err
	}
	stagedNames := map[string]bool{}
	for _, f := range staged {
		stagedNames[f.Name] = true
	}
	var replaced []string
	for _, p := range paths {
		if name := filepath.Base(p); stagedNames[name] {
			replaced = append(replaced, name)
		}
	}
	if len(replaced) > 0 {
		s.log.Info("添加", "%d 个文件与整合清单同名，将覆盖原有数据: %s",
			len(replaced), strings.Join(replaced, "、"))
	}
	s.log.Info("添加", "添加 %d 个源文件", len(paths))
	res, err := s.ingest("add", paths, progress, ingestOptions{merge: true, replace: replaced})
	if err != nil {
		return nil, err
	}
	res.Replaced = replaced
	return res, nil
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

	// The workbench, if one is open. Its global rows and local rows are
	// independent: either import may have come first.
	sum, err := s.db.LoadStaging()
	if err != nil {
		return nil, err
	}
	hasGlobal := sum.HasStaging && sum.Has(store.DatasetGlobal)

	var canonical *mps.Header
	templateReady := false
	// 添加 merges into the global rows already staged, so the staged header is
	// the reference every added workbook has to match.
	if opts.merge && hasGlobal {
		canonical = &mps.Header{IndexNames: sum.IndexNames, Weeks: parseWeekCodes(sum.WeekCodes)}
		res.WeekCode = sum.WeekCode
		res.WeekStart = sum.WeekStart
		res.WeekCodes = canonical.Weeks
		res.IndexNames = sum.IndexNames
		templateReady = sum.TemplateID.Valid
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
				CFColors:  r.CFColors,
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

		// What the reader could not reproduce (a missing colour table, an
		// unreadable annotation) is a property of this file, so it is named here
		// rather than degrading the whole batch silently.
		for _, w := range fr.Warnings {
			s.log.Warn("读取", "%s: %s", name, w)
			res.Warnings = append(res.Warnings, name+": "+w)
		}

		// The first workbook that reads cleanly also becomes the export
		// template, so the exported file looks like a real MPS report. The copy
		// itself is taken after the staging write succeeds (below): doing it here
		// meant a failed write left a folder on disk with no record of it.
		if templateSrc == "" && !templateReady {
			templateSrc = p
			templateReady = true
		}
	}

	if canonical == nil {
		return nil, fmt.Errorf("没有可用的源文件：%d 个文件全部读取失败", res.Failed)
	}
	if res.OK == 0 {
		return nil, fmt.Errorf("没有可用的源文件：%d 个文件全部读取失败", res.Failed)
	}
	if sum.HasStaging {
		// Two imports belong to one week; mixing weeks would produce a file that
		// belongs to neither. Keep the open workbench intact and say so.
		if canonical.FirstWeekCode() != sum.WeekCode {
			return nil, fmt.Errorf("当前工作台是周码 %s，导入的文件是 %s；请先完成或清空当前工作台",
				sum.WeekCode, canonical.FirstWeekCode())
		}
		if sum.Has(store.DatasetLocal) && canonical.Signature() != strings.Join(sum.IndexNames, "\x1f") {
			return nil, fmt.Errorf("Global 文件的索引列与已导入的本地数据不一致（%s）",
				headerDiff(mps.Header{IndexNames: sum.IndexNames}, *canonical))
		}
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
	var result *store.StagingResult
	var writeErr error
	switch {
	case !sum.HasStaging:
		result, writeErr = s.db.SaveStaging(input)
	case opts.merge && hasGlobal:
		result, writeErr = s.db.MergeStaging(input, opts.replace)
	default:
		// A global import into a workbench that has no global rows yet (local
		// imported first), or a fresh 导入文件夹 over an existing one: replace only
		// the global side, adopt the new file's columns, and re-index the local
		// rows that are already staged onto them.
		var localRows []store.StagedRow
		if sum.Has(store.DatasetLocal) {
			oldRows, err := s.db.LoadStagingDataset(sum.BatchID, store.DatasetLocal)
			if err != nil {
				return nil, err
			}
			localRows = realignStagedRows(oldRows, parseWeekCodes(sum.WeekCodes), canonical.Weeks)
		}
		writeErr = s.db.ReplaceGlobalInBatch(sum.BatchID, store.GlobalBatchInput{
			Header: store.StagingHeader{
				WeekCode:   res.WeekCode,
				WeekStart:  res.WeekStart,
				WeekCodes:  weekCodes(canonical.Weeks),
				IndexNames: canonical.IndexNames,
			},
			Files:       staged,
			Rows:        rows,
			Dict:        dict,
			Dxf:         dxf,
			LocalRows:   localRows,
			TemplateSrc: input.TemplateSrc,
			TemplateNam: input.TemplateNam,
		})
		if writeErr == nil {
			result = &store.StagingResult{BatchID: sum.BatchID, TemplateID: sum.TemplateID}
		}
	}
	if writeErr != nil {
		s.log.Error("临时数据", "写入失败: %v", writeErr)
		return nil, writeErr
	}

	// Only now that the batch is recorded is the template copy worth taking.
	if templateSrc != "" {
		if err := s.storeTemplate(res.WeekCode, templateSrc); err != nil {
			s.log.Warn("模板", "保存导出模板失败: %v", err)
			res.Warnings = append(res.Warnings, "保存导出模板失败: "+err.Error())
		}
	}
	// A superseded week's template row is gone; its file copy has to follow,
	// otherwise the folder stays on disk with nothing pointing at it.
	s.dropOrphanedTemplates(result)

	res.DurationMS = time.Since(start).Milliseconds()
	src := logSource(action)
	s.log.Success(src, "完成：成功 %d 个文件，失败 %d 个，合并 %d 行（周码 %s），耗时 %d 毫秒",
		res.OK, res.Failed, res.RowsKept, res.WeekCode, res.DurationMS)
	if len(res.Warnings) > 0 {
		s.log.Warn(src, "共 %d 条提示，详见日志", len(res.Warnings))
	}
	return res, nil
}

// logSource turns the machine action name into the Chinese label the log window
// shows. The result struct keeps the raw value ("import"/"add") because the
// frontend keys its wording off it.
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

// dropOrphanedTemplates deletes the stored template folders of the weeks whose
// template row the write removed. A database transaction cannot touch files, so
// it happens here, once the write has succeeded.
func (s *Service) dropOrphanedTemplates(res *store.StagingResult) {
	if res == nil {
		return
	}
	for _, week := range res.OrphanedTemplates {
		if err := os.RemoveAll(filepath.Join(s.templateDir, week)); err != nil {
			s.log.Warn("模板", "删除过期导出模板 %s 失败: %v", week, err)
		}
	}
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

// ImportLocal reads a 本地 MPS workbook, checks its layout and stages its rows.
//
// It does not merge: 整合 is the only place where the global and local rows are
// joined. The two imports are independent, so a local file may be imported
// before or after the global one; whichever comes second is aligned onto the
// workbench's week columns.
func (s *Service) ImportLocal(path string, progress Progress) (*ImportResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	start := time.Now()
	cfg := s.cfg.Get()
	name := filepath.Base(path)

	if progress != nil {
		progress("读取本地文件", 0, 2)
	}
	local, err := mps.ReadLocalFile(path, mps.ReadOptions{LOCFilter: cfg.LOCFilter})
	if err != nil {
		// The reader's error already names the sheet, row and column that do not
		// look like the local layout.
		s.log.Error("本地", "%s 格式检查未通过: %v", name, err)
		return nil, err
	}
	if len(local.Header.Weeks) == 0 {
		return nil, fmt.Errorf("本地文件没有周列")
	}

	sum, err := s.db.LoadStaging()
	if err != nil {
		return nil, err
	}
	weekCode := local.Header.FirstWeekCode()
	if sum.HasStaging && sum.WeekCode != weekCode {
		// Mixing two weeks would silently produce a file that belongs to neither.
		return nil, fmt.Errorf("当前工作台是周码 %s，本地文件是 %s；请先完成或清空当前工作台", sum.WeekCode, weekCode)
	}

	present := localStagedRows(local, name, path, local.Header.Weeks)
	var size int64
	if info, err := os.Stat(path); err == nil {
		size = info.Size()
	}
	localFile := store.StagedFile{
		Path: path, Name: name, Size: size, Status: "ok",
		RowsTotal: local.RowsTotal, RowsKept: local.RowsKept,
		WeekCode: weekCode,
	}

	if progress != nil {
		progress("写入临时数据", 1, 2)
	}
	switch {
	case !sum.HasStaging:
		// Local first: its own columns define the workbench until the global
		// import arrives and takes over the week list.
		snap, _ := json.Marshal(cfg)
		if _, err := s.db.SaveStaging(store.StagingInput{
			Action:     "local",
			Dataset:    store.DatasetLocal,
			WeekCode:   weekCode,
			WeekStart:  local.Header.Weeks[0].StartText(),
			WeekCodes:  weekCodes(local.Header.Weeks),
			IndexNames: local.Header.IndexNames,
			Files:      []store.StagedFile{localFile},
			Rows:       present,
			ParamSnap:  string(snap),
		}); err != nil {
			s.log.Error("本地", "写入临时数据失败: %v", err)
			return nil, err
		}
	case !sum.Has(store.DatasetGlobal):
		// A local-only workbench being re-imported: the new file's columns become
		// the frame, so the batch is rebuilt rather than patched.
		snap, _ := json.Marshal(cfg)
		if _, err := s.db.SaveStaging(store.StagingInput{
			Action:     "local",
			Dataset:    store.DatasetLocal,
			WeekCode:   weekCode,
			WeekStart:  local.Header.Weeks[0].StartText(),
			WeekCodes:  weekCodes(local.Header.Weeks),
			IndexNames: local.Header.IndexNames,
			Files:      []store.StagedFile{localFile},
			Rows:       present,
			ParamSnap:  string(snap),
		}); err != nil {
			s.log.Error("本地", "写入临时数据失败: %v", err)
			return nil, err
		}
	default:
		// Global is already staged: its columns stay, and the local values are
		// re-indexed onto them by week code.
		if local.Header.Signature() != strings.Join(sum.IndexNames, "\x1f") {
			err := fmt.Errorf("本地文件的索引列与已导入的 Global 数据不一致（%s）",
				headerDiff(mps.Header{IndexNames: sum.IndexNames}, local.Header))
			s.log.Error("本地", "%s 格式检查未通过: %v", name, err)
			return nil, err
		}
		present = realignStagedRows(present, local.Header.Weeks, parseWeekCodes(sum.WeekCodes))
		if err := s.db.ReplaceStagingDatasets(sum.BatchID, store.DatasetInput{
			Dataset: store.DatasetLocal, Files: []store.StagedFile{localFile}, Rows: present,
		}); err != nil {
			s.log.Error("本地", "写入临时数据失败: %v", err)
			return nil, err
		}
	}

	res := &ImportResult{
		Action:     "local",
		WeekCode:   weekCode,
		WeekStart:  local.Header.Weeks[0].StartText(),
		WeekCodes:  local.Header.Weeks,
		IndexNames: local.Header.IndexNames,
		Total:      1,
		OK:         1,
		RowsKept:   len(present),
		LocalRows:  len(present),
		Files: []FileResult{{
			Name: name, Path: path, Size: size, Status: "ok",
			RowsTotal: local.RowsTotal, RowsKept: local.RowsKept,
			WeekCode: weekCode,
		}},
		Warnings:   []string{},
		DurationMS: time.Since(start).Milliseconds(),
	}

	hasGlobal := sum.HasStaging && sum.Has(store.DatasetGlobal)
	next := "下一步：导入 Global 数据后即可整合"
	if hasGlobal {
		next = "Global 已在工作台，下一步：点整合生成最终结果"
	}
	s.log.Success("本地", "已导入 %s：本地 %d 行（LOC=%s 命中 %d/%d 行），周码 %s；%s",
		name, local.RowsKept, cfg.LOCFilter, local.RowsKept, local.RowsTotal, weekCode, next)
	return res, nil
}

// localStagedRows converts the local reader's rows into staged rows. When to is
// the file's own week list the values pass through unchanged.
func localStagedRows(local *mps.LocalSheet, name, path string, to []mps.Week) []store.StagedRow {
	out := make([]store.StagedRow, 0, len(local.Rows))
	for _, r := range local.Rows {
		out = append(out, store.StagedRow{
			FileName:  name,
			FilePath:  path,
			SourceRow: r.SourceRow,
			Index:     r.Index,
			Weeks:     alignWeeks(r.Weeks, local.Header.Weeks, to),
		})
	}
	return out
}

// realignStagedRows re-indexes staged rows onto another week list by code. The
// rows passed here are the local ones, which carry no styles or annotations.
func realignStagedRows(rows []store.StagedRow, from, to []mps.Week) []store.StagedRow {
	at := make(map[string]int, len(from))
	for i, w := range from {
		at[w.Code] = i
	}
	out := make([]store.StagedRow, len(rows))
	for i, r := range rows {
		cp := r
		cp.Weeks = make([]string, len(to))
		for j, w := range to {
			if k, ok := at[w.Code]; ok && k < len(r.Weeks) {
				cp.Weeks[j] = r.Weeks[k]
			}
		}
		if len(from) != len(to) {
			cp.CF = mps.CFRow{}
			cp.Comments = nil
			cp.CFColors = nil
		}
		out[i] = cp
	}
	return out
}

// alignWeeks re-indexes per-week values from one week list onto another by code.
// Values for weeks the target does not have are dropped.
func alignWeeks(vals []string, from, to []mps.Week) []string {
	at := make(map[string]int, len(from))
	for i, w := range from {
		at[w.Code] = i
	}
	out := make([]string, len(to))
	for i, w := range to {
		if j, ok := at[w.Code]; ok && j < len(vals) {
			out[i] = vals[j]
		}
	}
	return out
}

// Commit promotes the staged data into the permanent weekly tables and returns
// the merged result's entry. The second result reports whether a week with that
// code was already integrated and has just been replaced.
//
// The previous state is read inside the lock: reading it beforehand left a
// window in which a concurrent import could change what was staged, and the
// answer would describe a different batch.
func (s *Service) Commit() (*store.ArchiveEntry, bool, error) {
	entries, overwrote, err := s.CommitEntries()
	if err != nil {
		return nil, false, err
	}
	for _, e := range entries {
		if e.Dataset == store.DatasetMerged {
			return e, overwrote, nil
		}
	}
	return nil, false, fmt.Errorf("整合没有生成整合结果")
}

// CommitEntries is Commit with all three of the week's datasets: the caller
// reports their row counts in the summary.
//
// 整合 now needs the local file: the result the business hands on is the join of
// the global data and the local sheet, so integrating before that join would
// store an incomplete week.
func (s *Service) CommitEntries() ([]*store.ArchiveEntry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sum, err := s.db.LoadStaging()
	if err != nil {
		return nil, false, err
	}
	if !sum.HasStaging {
		return nil, false, fmt.Errorf("没有待整合的临时数据，请先导入或添加文件")
	}
	// 整合 is the only place the two inputs are joined, so it needs both of them.
	// Their import order does not matter.
	if !sum.Has(store.DatasetGlobal) {
		return nil, false, fmt.Errorf("还需要导入 Global 数据，才能整合")
	}
	if !sum.Has(store.DatasetLocal) {
		return nil, false, fmt.Errorf("还需要导入本地 MPS 文件，才能整合")
	}
	previous, err := s.db.ArchiveEntryFor(sum.WeekCode, store.DatasetMerged)
	if err != nil {
		return nil, false, err
	}
	entries, err := s.db.CommitAll(sum.WeekCode)
	if err != nil {
		s.log.Error("整合", "失败: %v", err)
		return nil, false, err
	}
	counts := map[string]int{}
	for _, e := range entries {
		counts[e.Dataset] = e.RowCount
	}
	s.log.Success("整合", "周码 %s（%s）已入库：整合 %d 行、Global %d 行、Local %d 行%s",
		sum.WeekCode, sum.WeekStart, counts[store.DatasetMerged], counts[store.DatasetGlobal],
		counts[store.DatasetLocal], map[bool]string{true: "（覆盖原有数据）", false: ""}[previous != nil])
	return entries, previous != nil, nil
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
		// Nothing to delete, but 清空 also ends the 已导入文件 list, which keeps
		// showing the batch that was integrated until then.
		if _, err := s.db.ClearStaging(); err != nil {
			s.log.Error("清空", "清空导入文件列表失败: %v", err)
			return 0, err
		}
		s.log.Success("清空", "已清空导入文件列表（没有未整合的临时数据）")
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

	// The workbench holds the two imported sides; there is no merged result to
	// count before 整合, so the summary reports the raw rows that went away.
	rows := sum.Rows(store.DatasetGlobal) + sum.Rows(store.DatasetLocal)
	if rows == 0 {
		rows = sum.RowKept
	}
	s.log.Success("清空", "已清空未整合的数据：周码 %s，Global %d 行 + Local %d 行（整合入库的数据不受影响）",
		sum.WeekCode, sum.Rows(store.DatasetGlobal), sum.Rows(store.DatasetLocal))
	return rows, nil
}

// ExportOptions selects what to export and how.
type ExportOptions struct {
	// WeekCode is the committed week to export; empty exports the staging area.
	WeekCode string
	// Dataset picks which of a week's three datasets the single-sheet report is
	// built from. Empty means the merged result.
	Dataset string
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

	entry, rows, err := s.resolveExportRows(weekCode, opt.Dataset)
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
	for _, w := range stats.Warnings {
		s.log.Warn("导出", "%s", w)
	}
	if stats.CFRulesDropped > 0 {
		s.log.Warn("导出", "有 %d 处样式或条件格式因源数据缺失未能还原", stats.CFRulesDropped)
	}
	return stats, nil
}

// toMPSRows converts stored export rows into the package's write shape.
func toMPSRows(rows []store.ExportRow) []mps.ExportRow {
	out := make([]mps.ExportRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, mps.ExportRow{
			Index: r.Index, Weeks: r.Weeks, StyleIDs: r.StyleIDs,
			CF: r.CF, Comments: r.Comments, SourceRow: r.SourceRow, FileName: r.FileName,
		})
	}
	return out
}

// BundleExportOptions selects what the three-table export writes.
type BundleExportOptions struct {
	// WeekCode is the integrated week; empty means the staging area, or the
	// newest integrated week when there is none.
	WeekCode string
	DestPath string
}

// ExportBundle writes the business's three-table workbook: MPS (the merged
// result), Local and Global, one sheet each. It reads integrated weeks only —
// the merged result does not exist before 整合, and exporting a half-finished
// workbench would produce a file that belongs to no week.
func (s *Service) ExportBundle(opt BundleExportOptions, progress Progress) (*mps.ExportStats, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	parts := []struct{ sheet, dataset string }{
		{"MPS", store.DatasetMerged},
		{"Local", store.DatasetLocal},
		{"Global", store.DatasetGlobal},
	}

	sum, err := s.db.LoadStaging()
	if err != nil {
		return nil, err
	}
	if sum.HasStaging && (opt.WeekCode == "" || opt.WeekCode == sum.WeekCode) {
		return nil, fmt.Errorf("请先整合后再导出三表文件")
	}

	if progress != nil {
		progress("读取数据", 0, 2)
	}

	var sheets []mps.BundleSheet
	weekCode := opt.WeekCode
	if weekCode == "" {
		list, err := s.db.ListArchive()
		if err != nil {
			return nil, err
		}
		if len(list) == 0 {
			return nil, fmt.Errorf("没有可导出的数据，请先导入并整合")
		}
		weekCode = list[0].WeekCode
	}
	if !store.ValidWeekCode(weekCode) {
		return nil, fmt.Errorf("非法周码 %q", weekCode)
	}
	for _, p := range parts {
		entry, rows, err := s.db.FetchExportRowsOf(weekCode, p.dataset)
		if err != nil {
			return nil, err
		}
		if entry == nil {
			return nil, fmt.Errorf("周码 %s 没有 %s 数据；请先整合", weekCode, p.sheet)
		}
		sheets = append(sheets, mps.BundleSheet{
			Name: p.sheet, Weeks: parseWeekCodes(entry.WeekCodes),
			IndexNames: entry.IndexNames, Rows: toMPSRows(rows),
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
	if progress != nil {
		progress("完成", 2, 2)
	}
	s.log.Success("导出", "三表文件 %s → %s（MPS %d 行 / Local %d 行 / Global %d 行）",
		weekCode, opt.DestPath, len(sheets[0].Rows), len(sheets[1].Rows), len(sheets[2].Rows))
	return stats, nil
}

// exportSource is what an export needs to know about where its rows came from:
// only the week's identity, not the table it happens to live in.
//
// This used to be a store.ArchiveEntry, which forced the staging path to
// fabricate one carrying the storage-internal table name "stg_row" — a
// service-layer struct asserting persistence internals purely to satisfy a type.
type exportSource struct {
	WeekCode   string
	WeekCodes  []string
	IndexNames []string
}

// sourceOf narrows a stored week entry to what an export needs.
func sourceOf(e *store.ArchiveEntry) *exportSource {
	return &exportSource{WeekCode: e.WeekCode, WeekCodes: e.WeekCodes, IndexNames: e.IndexNames}
}

// resolveExportRows decides what 导出 writes.
//
// A named week (the history panel, or an explicit choice) always wins. With no
// week named, the export follows the main grid: the staging area while there
// is one, otherwise the newest integrated week. Falling back like this is what
// keeps the toolbar button working after 整合, when the staging batch has been
// promoted and no longer counts as staging.
func (s *Service) resolveExportRows(weekCode, dataset string) (*exportSource, []store.ExportRow, error) {
	if weekCode != "" {
		if !store.ValidWeekCode(weekCode) {
			return nil, nil, fmt.Errorf("非法周码 %q", weekCode)
		}
		// The toolbar passes the week code it is showing. While that week is still
		// staged it is not in the archive yet, and exporting it has to read the
		// staging area rather than report an unknown week.
		if sum, err := s.db.LoadStaging(); err == nil && sum.HasStaging && sum.WeekCode == weekCode {
			staged, rows, err := s.db.FetchStagingExportRows(dataset)
			if err != nil {
				return nil, nil, err
			}
			return &exportSource{
				WeekCode:   staged.WeekCode,
				WeekCodes:  staged.WeekCodes,
				IndexNames: staged.IndexNames,
			}, rows, nil
		}
		entry, rows, err := s.db.FetchExportRowsOf(weekCode, dataset)
		if err != nil {
			return nil, nil, err
		}
		return sourceOf(entry), rows, nil
	}

	sum, err := s.db.LoadStaging()
	if err != nil {
		return nil, nil, err
	}
	if sum.HasStaging {
		staged, rows, err := s.db.FetchStagingExportRows(dataset)
		if err != nil {
			return nil, nil, err
		}
		s.log.Info("导出", "导出尚未整合的临时数据（周码 %s）", staged.WeekCode)
		return &exportSource{
			WeekCode:   staged.WeekCode,
			WeekCodes:  staged.WeekCodes,
			IndexNames: staged.IndexNames,
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
	entry, rows, err := s.db.FetchExportRowsOf(latest.WeekCode, dataset)
	if err != nil {
		return nil, nil, err
	}
	return sourceOf(entry), rows, nil
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

// WaitIdle blocks until any import, commit, clear or export in progress has
// finished. The application calls it before releasing the database, so a running
// operation is never left writing to a closed handle.
func (s *Service) WaitIdle() {
	s.mu.Lock()
	defer s.mu.Unlock()
}

// The read side of the service. These deliberately do not take mu: the long part
// of an import is reading workbooks, and holding the write lock across it would
// block the status bar and the grid for the whole run. Reads are serialised
// against writes by the single database connection instead, exactly as before —
// these wrappers exist so the application does not reach into the store itself.
func (s *Service) StagingSummary() (*store.StagingSummary, error) { return s.db.LoadStaging() }

func (s *Service) Archive() ([]store.ArchiveEntry, error) { return s.db.ListArchive() }

func (s *Service) ArchiveEntry(weekCode string) (*store.ArchiveEntry, error) {
	return s.db.ArchiveEntryFor(weekCode, store.DatasetMerged)
}

func (s *Service) BatchFiles() ([]store.StagedFileDetail, string, string, error) {
	return s.db.CurrentBatchFiles()
}

func (s *Service) Years() ([]int, error) { return s.db.AvailableYears() }

func (s *Service) Weeks(year int) ([]int, error) { return s.db.WeeksOfYear(year) }

// GridSource is the column layout of one grid source.
type GridSource struct {
	HasStaging bool
	WeekCode   string
	WeekStart  string
	WeekCodes  []string
	IndexNames []string
}

// GridSource resolves a grid source: the staging area when weekCode is empty,
// otherwise that committed week. A source with nothing behind it comes back
// empty rather than as an error, which is what the grid shows before the first
// import.
func (s *Service) GridSource(weekCode, dataset string) (*GridSource, error) {
	if weekCode == "" {
		sum, err := s.db.LoadStaging()
		if err != nil {
			return nil, err
		}
		if !sum.HasStaging {
			return &GridSource{}, nil
		}
		return &GridSource{
			HasStaging: true,
			WeekCode:   sum.WeekCode,
			WeekStart:  sum.WeekStart,
			WeekCodes:  sum.WeekCodes,
			IndexNames: sum.IndexNames,
		}, nil
	}
	entry, err := s.db.ArchiveEntryFor(weekCode, dataset)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return &GridSource{}, nil
	}
	return &GridSource{
		WeekCode:   entry.WeekCode,
		WeekStart:  entry.WeekStart,
		WeekCodes:  entry.WeekCodes,
		IndexNames: entry.IndexNames,
	}, nil
}
