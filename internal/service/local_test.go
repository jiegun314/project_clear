package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"project_clear/internal/mps"
	"project_clear/internal/store"

	"github.com/xuri/excelize/v2"
)

// mpsIndexNames is the A..O header of a real MPS report, which is also the
// normalised layout the 本地 MPS reader produces.
var mpsIndexNames = []string{
	"P5", "P4", "P3", "P2", "P1", "MFG GROUP", "MFG CLASS CODE", "MFG DESCR",
	"ITEM", "US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC",
}

// writeLocalFile builds a 本地 MPS workbook for the standard three-week window.
func writeLocalFile(t *testing.T) string {
	return writeLocalWithWeeks(t, []string{"2639", "2640", "2641"})
}

// writeLocalWithWeeks builds the 本地 MPS layout: sheet "Month End", row 1
// headers A..Q, week codes from R, data from row 4.
func writeLocalWithWeeks(t *testing.T, weeks []string) string {
	t.Helper()
	dir := t.TempDir()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", mps.LocalSheetName); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for i, name := range mps.LocalHeader {
		if err := f.SetCellStr(mps.LocalSheetName, mps.CellRef(i+1, 1), name); err != nil {
			t.Fatal(err)
		}
	}
	for i, code := range weeks {
		_ = f.SetCellStr(mps.LocalSheetName, mps.CellRef(mps.LocalFirstWeekCol+i, 1), code)
	}
	row := []string{
		"P5", "P4", "P3", "P2", "P1", "GROUP L", "CLASS L", "DESCR L", "LOCAL-ONLY",
		"BASE-L", "SAP-L", "US-L", "EU-L", "WH_CNB", "0", "42", "AdjDmd",
	}
	for i, v := range row {
		if err := f.SetCellStr(mps.LocalSheetName, mps.CellRef(i+1, mps.LocalDataRow), v); err != nil {
			t.Fatal(err)
		}
	}
	for i := range weeks {
		if err := f.SetCellValue(mps.LocalSheetName, mps.CellRef(mps.LocalFirstWeekCol+i, mps.LocalDataRow), float64(500+i)); err != nil {
			t.Fatal(err)
		}
	}
	path := filepath.Join(dir, "BW_CN_MPS Week39 2026 (Month End).xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save local: %v", err)
	}
	return path
}

// writeExtraGlobal builds a second global workbook for the standard weeks.
func writeExtraGlobal(t *testing.T) string {
	return writeGlobalWithWeeks(t, []string{"2639", "2640", "2641"})
}

// writeGlobalWithWeeks builds a global MPS workbook with one item the main
// fixture does not have.
func writeGlobalWithWeeks(t *testing.T, weeks []string) string {
	t.Helper()
	dir := t.TempDir()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", mps.SheetName); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for c, name := range mpsIndexNames {
		_ = f.SetCellStr(mps.SheetName, mps.CellRef(c+1, mps.HeaderRow), name)
	}
	for i, code := range weeks {
		col := mps.FirstWeekCol + i
		_ = f.SetCellStr(mps.SheetName, mps.CellRef(col, mps.HeaderRow), code)
		w, err := mps.ParseWeekCode(code)
		if err != nil {
			t.Fatalf("week %s: %v", code, err)
		}
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		_ = f.SetCellFloat(mps.SheetName, mps.CellRef(col, mps.DateRow), w.Start.Sub(base).Hours()/24, -1, 64)
	}
	for c := 1; c <= mps.IndexCols; c++ {
		_ = f.SetCellStr(mps.SheetName, mps.CellRef(c, mps.FirstDataRow), fmt.Sprintf("X%d", c))
	}
	_ = f.SetCellStr(mps.SheetName, mps.CellRef(mps.LOCCol, mps.FirstDataRow), "WH_CNB")
	for i := range weeks {
		_ = f.SetCellFloat(mps.SheetName, mps.CellRef(mps.FirstWeekCol+i, mps.FirstDataRow), float64(900+i), -1, 64)
	}
	path := filepath.Join(dir, "extra.xlsm")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save extra: %v", err)
	}
	return path
}

// importBoth stages the global fixture and then the 本地 MPS file.
func importBoth(t *testing.T, svc *Service) {
	t.Helper()
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := svc.ImportLocal(writeLocalFile(t), nil); err != nil {
		t.Fatalf("import local: %v", err)
	}
}

// The local file does not have to wait for the global one: it opens the
// workbench, and the global import joins it later.
func TestImportLocalCanComeFirst(t *testing.T) {
	svc, _ := newTestService(t)

	res, err := svc.ImportLocal(writeLocalFile(t), nil)
	if err != nil {
		t.Fatalf("import local: %v", err)
	}
	if res.LocalRows != 1 {
		t.Errorf("local rows = %d, want 1", res.LocalRows)
	}
	sum, err := svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if !sum.HasStaging || !sum.Has(store.DatasetLocal) || sum.Has(store.DatasetGlobal) {
		t.Fatalf("workbench after a local-first import = %+v", sum)
	}
	if sum.WeekCode != "2639" || sum.DisplayDataset() != store.DatasetLocal {
		t.Errorf("workbench week/display = %s/%s, want 2639/local", sum.WeekCode, sum.DisplayDataset())
	}

	// The global import then joins the same workbench.
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import global: %v", err)
	}
	sum, err = svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if !sum.Has(store.DatasetGlobal) || !sum.Has(store.DatasetLocal) {
		t.Fatalf("workbench after both imports = %+v", sum)
	}
	if got := sum.Rows(store.DatasetMerged); got != 0 {
		t.Errorf("merged rows before 整合 = %d, want 0", got)
	}

	entries, _, err := svc.CommitEntries()
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("commit entries = %d, want 3", len(entries))
	}
	grid, err := svc.Query(store.Query{Source: "2639", Dataset: store.DatasetMerged, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if grid.Total != 2 {
		t.Fatalf("merged rows = %d, want 2", grid.Total)
	}
	if grid.Rows[0].Index[8] != "IDX-9" || grid.Rows[1].Index[8] != "LOCAL-ONLY" {
		t.Errorf("merged order = %q, %q", grid.Rows[0].Index[8], grid.Rows[1].Index[8])
	}
	// The global import also brought the export template; without it the archive
	// row would point at no template and 导出原文件格式 would fall back to a copy.
	tpl, err := svc.db.TemplateFor("2639")
	if err != nil {
		t.Fatal(err)
	}
	if tpl == nil {
		t.Error("the global import after a local-first workbench recorded no template")
	}
}

// A local file imported first may have more week columns than the global report;
// when the global arrives, the workbench adopts its columns and the local values
// are re-indexed by week code rather than by position.
func TestGlobalImportRealignsTheLocalColumns(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ImportLocal(writeLocalWithWeeks(t, []string{
		"2639", "2640", "2641", "2642", "2643",
	}), nil); err != nil {
		t.Fatalf("import local: %v", err)
	}
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import global: %v", err)
	}

	sum, err := svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if len(sum.WeekCodes) != 3 {
		t.Fatalf("workbench weeks = %v, want the global report's three", sum.WeekCodes)
	}
	grid, err := svc.Query(store.Query{Source: "", Dataset: store.DatasetLocal, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("query local: %v", err)
	}
	if grid.Total != 1 {
		t.Fatalf("local rows = %d, want 1", grid.Total)
	}
	if got := grid.Rows[0].Weeks; len(got) != 3 || got[0] != "500" || got[1] != "501" || got[2] != "502" {
		t.Errorf("local weeks after the realignment = %v, want the first three values in order", got)
	}
}

// A workbook that is not the local layout is refused before anything is written,
// and the reason names the row and column that do not match.
func TestImportLocalRejectsAWorkbookWithAnotherLayout(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", "Month End"); err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellStr("Month End", "A1", "not the local layout")
	path := filepath.Join(t.TempDir(), "wrong.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	f.Close()

	_, err := svc.ImportLocal(path, nil)
	if err == nil {
		t.Fatal("a workbook with another layout should be refused")
	}
	if !strings.Contains(err.Error(), "第 1 行") || !strings.Contains(err.Error(), "期望") {
		t.Errorf("error = %q, want the sheet/row/column detail", err)
	}

	sum, err := svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Has(store.DatasetLocal) {
		t.Errorf("a refused local file still wrote local rows")
	}
}

// The imports are independent: each writes only its own side, the grid shows
// the side that was imported last (or the one asked for), and neither creates a
// merged result.
func TestImportsAreIndependentAndTheGridShowsTheSelectedSide(t *testing.T) {
	svc, _ := newTestService(t)
	importBoth(t, svc)

	sum, err := svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if got := sum.Rows(store.DatasetMerged); got != 0 {
		t.Errorf("merged rows before 整合 = %d, want 0", got)
	}
	if got := sum.DisplayDataset(); got != store.DatasetLocal {
		t.Errorf("grid default = %q, want the last imported side", got)
	}

	defaultGrid, err := svc.Query(store.Query{Source: "", Page: 1, PageSize: 10, SortField: "seq"})
	if err != nil {
		t.Fatalf("query default: %v", err)
	}
	if defaultGrid.Total != 1 || defaultGrid.Rows[0].Index[8] != "LOCAL-ONLY" {
		t.Errorf("default grid = %d rows, first item %q", defaultGrid.Total, defaultGrid.Rows[0].Index[8])
	}
	globalGrid, err := svc.Query(store.Query{Source: "", Dataset: store.DatasetGlobal, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatalf("query global: %v", err)
	}
	if globalGrid.Total != 1 || globalGrid.Rows[0].Index[8] != "IDX-9" {
		t.Errorf("global grid = %d rows, first item %q", globalGrid.Total, globalGrid.Rows[0].Index[8])
	}

	// Re-importing one side replaces only that side.
	if _, err := svc.ImportLocal(writeLocalFile(t), nil); err != nil {
		t.Fatalf("re-import local: %v", err)
	}
	sum, err = svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Rows(store.DatasetGlobal) != 1 || sum.Rows(store.DatasetLocal) != 1 {
		t.Errorf("after re-importing local: global %d / local %d, want 1/1",
			sum.Rows(store.DatasetGlobal), sum.Rows(store.DatasetLocal))
	}
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("re-import global: %v", err)
	}
	sum, err = svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Rows(store.DatasetGlobal) != 1 || sum.Rows(store.DatasetLocal) != 1 {
		t.Errorf("after re-importing global: global %d / local %d, want 1/1",
			sum.Rows(store.DatasetGlobal), sum.Rows(store.DatasetLocal))
	}
}

// 整合 computes the merge and writes one table per dataset and one archive row
// each. Before 整合 there is no merged dataset at all.
func TestCommitWritesTheThreeDatasets(t *testing.T) {
	svc, _ := newTestService(t)
	importBoth(t, svc)

	entries, overwrote, err := svc.CommitEntries()
	if err != nil {
		t.Fatalf("commit: %v", err)
	}
	if overwrote {
		t.Error("the first commit reported replacing an existing week")
	}
	if len(entries) != 3 {
		t.Fatalf("commit returned %d entries, want 3", len(entries))
	}
	want := map[string]struct {
		rows  int
		table string
	}{
		store.DatasetMerged: {2, "data_2639"},
		store.DatasetGlobal: {1, "data_2639_global"},
		store.DatasetLocal:  {1, "data_2639_local"},
	}
	for _, e := range entries {
		w, ok := want[e.Dataset]
		if !ok {
			t.Errorf("unexpected dataset %q", e.Dataset)
			continue
		}
		if e.RowCount != w.rows {
			t.Errorf("%s rows = %d, want %d", e.Dataset, e.RowCount, w.rows)
		}
		if e.TableName != w.table {
			t.Errorf("%s table = %q, want %q", e.Dataset, e.TableName, w.table)
		}
	}

	for dataset, w := range want {
		entry, err := svc.db.ArchiveEntryFor("2639", dataset)
		if err != nil {
			t.Fatalf("archive %s: %v", dataset, err)
		}
		if entry == nil {
			t.Errorf("no archive entry for %s", dataset)
			continue
		}
		grid, err := svc.Query(store.Query{Source: "2639", Dataset: dataset, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("query %s: %v", dataset, err)
		}
		if grid.Total != w.rows {
			t.Errorf("%s grid total = %d, want %d", dataset, grid.Total, w.rows)
		}
	}

	sum, err := svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.HasStaging {
		t.Error("the workbench survived 整合")
	}
}

// 整合 needs both inputs; which one is missing is what the user is told.
func TestCommitRequiresBothInputs(t *testing.T) {
	t.Run("missing local", func(t *testing.T) {
		svc, _ := newTestService(t)
		if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
			t.Fatalf("import: %v", err)
		}
		_, _, err := svc.Commit()
		if err == nil || !strings.Contains(err.Error(), "本地 MPS") {
			t.Errorf("error = %v, want it to name the missing local file", err)
		}
	})
	t.Run("missing global", func(t *testing.T) {
		svc, _ := newTestService(t)
		if _, err := svc.ImportLocal(writeLocalFile(t), nil); err != nil {
			t.Fatalf("import local: %v", err)
		}
		_, _, err := svc.Commit()
		if err == nil || !strings.Contains(err.Error(), "Global") {
			t.Errorf("error = %v, want it to name the missing Global data", err)
		}
	})
}

// A global import after the local one keeps the local rows and adopts the global
// week columns.
func TestGlobalImportAfterLocalKeepsTheLocalRows(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ImportLocal(writeLocalFile(t), nil); err != nil {
		t.Fatalf("import local: %v", err)
	}
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import global: %v", err)
	}

	sum, err := svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.Rows(store.DatasetGlobal) != 1 || sum.Rows(store.DatasetLocal) != 1 {
		t.Fatalf("after the global import: global %d / local %d, want 1/1",
			sum.Rows(store.DatasetGlobal), sum.Rows(store.DatasetLocal))
	}
	if sum.Rows(store.DatasetMerged) != 0 {
		t.Errorf("the global import created a merged result")
	}

	if _, _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	merged, err := svc.Query(store.Query{Source: "2639", Dataset: store.DatasetMerged, Page: 1, PageSize: 10})
	if err != nil {
		t.Fatal(err)
	}
	if merged.Total != 2 {
		t.Errorf("merged rows = %d, want 2", merged.Total)
	}
}

// 添加 after both imports grows only the global side and leaves the local rows
// alone; the merge is still computed at 整合.
func TestAddingAGlobalFileKeepsTheLocalRows(t *testing.T) {
	svc, _ := newTestService(t)
	importBoth(t, svc)

	if _, err := svc.AddFiles([]string{writeExtraGlobal(t)}, nil); err != nil {
		t.Fatalf("add: %v", err)
	}
	sum, err := svc.StagingSummary()
	if err != nil {
		t.Fatal(err)
	}
	if got := sum.Rows(store.DatasetGlobal); got != 2 {
		t.Errorf("global rows = %d, want 2", got)
	}
	if got := sum.Rows(store.DatasetLocal); got != 1 {
		t.Errorf("local rows = %d, want 1 (the add touched the local side)", got)
	}
	if got := sum.Rows(store.DatasetMerged); got != 0 {
		t.Errorf("merged rows before 整合 = %d, want 0", got)
	}

	if _, _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	grid, err := svc.Query(store.Query{Source: "2639", Dataset: store.DatasetMerged, Page: 1, PageSize: 10, SortField: "seq"})
	if err != nil {
		t.Fatal(err)
	}
	var items []string
	for _, r := range grid.Rows {
		items = append(items, r.Index[8])
	}
	if strings.Join(items, ",") != "IDX-9,X9,LOCAL-ONLY" {
		t.Errorf("merged items = %v, want the two global rows then the local one", items)
	}
}

// The two inputs must belong to the same week; mixing them is refused with both
// week codes named.
func TestImportsRejectAnotherWeeksFile(t *testing.T) {
	t.Run("local after global", func(t *testing.T) {
		svc, _ := newTestService(t)
		if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
			t.Fatalf("import global: %v", err)
		}
		_, err := svc.ImportLocal(writeLocalWithWeeks(t, []string{"2640", "2641", "2642"}), nil)
		if err == nil || !strings.Contains(err.Error(), "2639") || !strings.Contains(err.Error(), "2640") {
			t.Errorf("error = %v, want it to name both week codes", err)
		}
	})
	t.Run("global after local", func(t *testing.T) {
		svc, _ := newTestService(t)
		if _, err := svc.ImportLocal(writeLocalFile(t), nil); err != nil {
			t.Fatalf("import local: %v", err)
		}
		path := writeGlobalWithWeeks(t, []string{"2640", "2641", "2642"})
		_, err := svc.ImportFolder(filepath.Dir(path), nil)
		if err == nil || !strings.Contains(err.Error(), "2639") || !strings.Contains(err.Error(), "2640") {
			t.Errorf("error = %v, want it to name both week codes", err)
		}
	})
}

// There is no three-table file before 整合: the merged result it needs does not
// exist yet.
func TestExportBundleNeedsIntegration(t *testing.T) {
	svc, dir := newTestService(t)
	importBoth(t, svc)

	if _, err := svc.ExportBundle(BundleExportOptions{DestPath: filepath.Join(dir, "early.xlsx")}, nil); err == nil {
		t.Fatal("a bundle exported before 整合 should be refused")
	}

	if _, _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	dest := filepath.Join(dir, "bundle.xlsx")
	stats, err := svc.ExportBundle(BundleExportOptions{WeekCode: "2639", DestPath: dest}, nil)
	if err != nil {
		t.Fatalf("bundle export: %v", err)
	}
	if stats.Sheets != 3 {
		t.Errorf("sheets = %d, want 3", stats.Sheets)
	}
	assertBundle(t, dest, 2, 1, 1)
}

// The history window exports one dataset as the single-sheet report, so the
// Global and Local rows stay independently accessible after 整合.
func TestExportOneDatasetFromTheArchive(t *testing.T) {
	svc, dir := newTestService(t)
	importBoth(t, svc)
	if _, _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	dest := filepath.Join(dir, "global.xlsx")
	stats, err := svc.Export(ExportOptions{WeekCode: "2639", Dataset: store.DatasetGlobal, DestPath: dest}, nil)
	if err != nil {
		t.Fatalf("export global: %v", err)
	}
	if stats.Rows != 1 {
		t.Errorf("global export rows = %d, want 1", stats.Rows)
	}
	f, err := excelize.OpenFile(dest)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows(mps.SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows {
		for _, v := range r {
			if v == "LOCAL-ONLY" {
				t.Errorf("the Global export contains the local row")
			}
		}
	}
}

// assertBundle checks the sheet names, order, data row counts and used range of
// a three-table workbook.
func assertBundle(t *testing.T, path string, merged, local, global int) {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open %s: %v", path, err)
	}
	defer f.Close()
	names := f.GetSheetList()
	wantNames := []string{"MPS", "Local", "Global"}
	if strings.Join(names, ",") != strings.Join(wantNames, ",") {
		t.Fatalf("sheets = %v, want %v", names, wantNames)
	}
	for i, want := range []int{merged, local, global} {
		rows, err := f.GetRows(wantNames[i], excelize.Options{RawCellValue: true})
		if err != nil {
			t.Fatalf("read %s: %v", wantNames[i], err)
		}
		// Header, dates, spacer, then the data.
		if got := len(rows) - 3; got != want {
			t.Errorf("%s data rows = %d, want %d", wantNames[i], got, want)
		}
		if got := rows[0][0]; got != "P5" {
			t.Errorf("%s A1 = %q, want the index header", wantNames[i], got)
		}
		if got := rows[3][8]; got == "" {
			t.Errorf("%s first data row has no ITEM", wantNames[i])
		}
		// A written dimension that stops at A1 makes read_only readers report an
		// empty sheet even though every row is there.
		lastCol, err := mps.ColumnName(len(rows[0]))
		if err != nil {
			t.Fatal(err)
		}
		wantDim := fmt.Sprintf("A1:%s%d", lastCol, len(rows))
		dim, err := f.GetSheetDimension(wantNames[i])
		if err != nil {
			t.Fatalf("%s dimension: %v", wantNames[i], err)
		}
		if dim != wantDim {
			t.Errorf("%s dimension = %q, want %q", wantNames[i], dim, wantDim)
		}
	}
}
