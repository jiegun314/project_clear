package service

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"project_clear/internal/config"
	"project_clear/internal/logging"
	"project_clear/internal/mps"
	"project_clear/internal/store"

	"github.com/xuri/excelize/v2"
)

// newTestService wires a service to a throwaway data folder, so the tests never
// touch the folders the application itself uses.
func newTestService(t *testing.T) (*Service, string) {
	t.Helper()
	dir := t.TempDir()
	cfg, _, err := config.NewStoreAt(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	db, err := store.Open(filepath.Join(dir, "clear.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return New(cfg, logging.New(filepath.Join(dir, "logs")), db, dir), dir
}

// writeSourceFolder builds a workbook the importer accepts: the two header rows
// with three week columns, one LOC-matching data row, plus two extra sheets of
// the kind the real MPS exports carry.
func writeSourceFolder(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", mps.SheetName); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for i, extra := range []string{"Datadump", "PSI"} {
		idx, err := f.NewSheet(extra)
		if err != nil {
			t.Fatalf("new sheet %s: %v", extra, err)
		}
		_ = f.SetCellStr(extra, "A1", fmt.Sprintf("helper sheet %d", i))
		_ = idx
	}

	for c := 1; c <= mps.IndexCols; c++ {
		if err := f.SetCellStr(mps.SheetName, mps.CellRef(c, mps.HeaderRow), fmt.Sprintf("H%d", c)); err != nil {
			t.Fatal(err)
		}
	}
	// Decoration above the header, which the export must not carry over.
	_ = f.SetCellStr(mps.SheetName, "A1", "SUMMARY (click on left \"+\" to open or hide the Summary section)")
	if err := f.SetCellStr(mps.SheetName, mps.CellRef(mps.LOCCol, mps.HeaderRow), "LOC"); err != nil {
		t.Fatal(err)
	}
	const readColumns = 3
	weeks := []string{"2639", "2640", "2641"}
	for i, code := range weeks {
		w, err := mps.ParseWeekCode(code)
		if err != nil {
			t.Fatalf("week %s: %v", code, err)
		}
		col := mps.FirstWeekCol + i
		_ = f.SetCellStr(mps.SheetName, mps.CellRef(col, mps.HeaderRow), code)
		base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
		serial := w.Start.Sub(base).Hours() / 24
		if err := f.SetCellFloat(mps.SheetName, mps.CellRef(col, mps.DateRow), serial, -1, 64); err != nil {
			t.Fatal(err)
		}
	}

	row := mps.FirstDataRow
	for c := 1; c <= mps.IndexCols; c++ {
		if err := f.SetCellStr(mps.SheetName, mps.CellRef(c, row), fmt.Sprintf("IDX-%d", c)); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellStr(mps.SheetName, mps.CellRef(mps.LOCCol, row), "WH_CNB"); err != nil {
		t.Fatal(err)
	}
	for i := range weeks {
		if err := f.SetCellFloat(mps.SheetName, mps.CellRef(mps.FirstWeekCol+i, row), float64(100+i), -1, 64); err != nil {
			t.Fatal(err)
		}
	}
	// A note on the first week column, which the export must carry through.
	if err := f.AddComment(mps.SheetName, excelize.Comment{
		Cell: mps.CellRef(mps.FirstWeekCol, row), Author: "Alice", Text: "PO 9578 & 9579 receipts",
	}); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(dir, "source.xlsm")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save source: %v", err)
	}
	return dir
}

// The toolbar's 导出 names no week. It used to fail twice over: before 整合 the
// staged week is not in the archive yet, and after 整合 the batch stops being
// staging at all, so no week could be resolved.
func TestExportWithoutWeekFollowsTheGrid(t *testing.T) {
	svc, dir := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	// 1. imported but not yet integrated: the staging area is what is on screen.
	staged := filepath.Join(dir, "staged.xlsx")
	stats, err := svc.Export(ExportOptions{DestPath: staged}, nil)
	if err != nil {
		t.Fatalf("export staging: %v", err)
	}
	if stats.Rows != 1 {
		t.Errorf("staging export rows = %d, want 1", stats.Rows)
	}
	assertOnlyMPS(t, staged)
	assertHeaderIsFirstRow(t, staged)
	assertCell(t, staged, "P4", "100")
	assertComment(t, staged, "P4", "PO 9578 & 9579 receipts", "Alice")

	// 2. after 整合, with the staging batch promoted.
	if _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	committed := filepath.Join(dir, "committed.xlsx")
	stats, err = svc.Export(ExportOptions{DestPath: committed}, nil)
	if err != nil {
		t.Fatalf("export after commit: %v", err)
	}
	if stats.Rows != 1 {
		t.Errorf("committed export rows = %d, want 1", stats.Rows)
	}
	assertOnlyMPS(t, committed)
	assertHeaderIsFirstRow(t, committed)
	assertCell(t, committed, "P4", "100")
	assertComment(t, committed, "P4", "PO 9578 & 9579 receipts", "Alice")
}

// An explicit week (the history panel) still wins over the grid's contents.
func TestExportNamedWeek(t *testing.T) {
	svc, dir := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	dest := filepath.Join(dir, "named.xlsm")
	if _, err := svc.Export(ExportOptions{WeekCode: "2639", DestPath: dest, Mode: config.ExportTemplate}, nil); err != nil {
		t.Fatalf("export week 2639: %v", err)
	}
	if _, err := svc.Export(ExportOptions{WeekCode: "2699", DestPath: filepath.Join(dir, "x.xlsm")}, nil); err == nil {
		t.Errorf("a week that was never integrated should not export")
	}
}

// The 原文件格式 mode keeps the source workbook's own formatting, but hands on
// the MPS page only.
func TestTemplateExportKeepsOnlyMPS(t *testing.T) {
	svc, dir := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	if _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}
	dest := filepath.Join(dir, "original.xlsm")
	stats, err := svc.Export(ExportOptions{WeekCode: "2639", DestPath: dest, Mode: config.ExportTemplate}, nil)
	if err != nil {
		t.Fatalf("template export: %v", err)
	}
	if stats.Mode != "template" {
		t.Errorf("mode = %q, want template", stats.Mode)
	}
	assertOnlyMPS(t, dest)
	assertHeaderIsFirstRow(t, dest)
	assertCell(t, dest, "P4", "100")
}

// The exported sheet starts at the header: the report's decorative rows are not
// part of the data and are left out entirely, in both export modes.
func assertHeaderIsFirstRow(t *testing.T, path string) {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open %s: %v", filepath.Base(path), err)
	}
	defer f.Close()
	rows, err := f.GetRows(mps.SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("read %s: %v", filepath.Base(path), err)
	}
	at := func(r, c int) string {
		if r < 0 || r >= len(rows) || c < 0 || c >= len(rows[r]) {
			return ""
		}
		return rows[r][c]
	}
	if got := at(mps.ExportHeaderRow-1, 0); got != "H1" {
		t.Errorf("%s A%d = %q, want the index header", filepath.Base(path), mps.ExportHeaderRow, got)
	}
	if got := at(mps.ExportHeaderRow-1, mps.FirstWeekCol-1); got != "2639" {
		t.Errorf("%s P%d = %q, want the week code", filepath.Base(path), mps.ExportHeaderRow, got)
	}
	if got := at(mps.ExportFirstDataRow-1, 0); got != "IDX-1" {
		t.Errorf("%s A%d = %q, want the first data row", filepath.Base(path), mps.ExportFirstDataRow, got)
	}
	for r, row := range rows {
		for c, v := range row {
			if strings.Contains(v, "SUMMARY") {
				t.Errorf("%s kept decorative content at %s: %q", filepath.Base(path), mps.CellRef(c+1, r+1), v)
			}
		}
	}
}

// The default export is 纯数据 and carries colours and notes with it.
func TestDefaultExportIsCleanAndKeepsNotes(t *testing.T) {
	cfg, _, err := config.NewStoreAt(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Get().ExportMode; got != config.ExportClean {
		t.Fatalf("default export mode = %q, want %q", got, config.ExportClean)
	}
}

func assertOnlyMPS(t *testing.T, path string) {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open %s: %v", filepath.Base(path), err)
	}
	defer f.Close()
	sheets := f.GetSheetList()
	if len(sheets) != 1 || sheets[0] != mps.SheetName {
		t.Errorf("%s sheets = %v, want only %q", filepath.Base(path), sheets, mps.SheetName)
	}
}

func assertCell(t *testing.T, path, ref, want string) {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open %s: %v", filepath.Base(path), err)
	}
	defer f.Close()
	got, err := f.GetCellValue(mps.SheetName, ref, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("read %s: %v", ref, err)
	}
	if got != want {
		t.Errorf("%s %s = %q, want %q", filepath.Base(path), ref, got, want)
	}
}

func assertComment(t *testing.T, path, ref, wantText, wantAuthor string) {
	t.Helper()
	f, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatalf("open %s: %v", filepath.Base(path), err)
	}
	defer f.Close()
	list, err := f.GetComments(mps.SheetName)
	if err != nil {
		t.Fatalf("comments: %v", err)
	}
	for _, c := range list {
		if c.Cell == ref {
			if c.Text != wantText || c.Author != wantAuthor {
				t.Errorf("%s comment = %q by %q, want %q by %q", ref, c.Text, c.Author, wantText, wantAuthor)
			}
			return
		}
	}
	t.Errorf("%s has no comment in %s", ref, filepath.Base(path))
}
