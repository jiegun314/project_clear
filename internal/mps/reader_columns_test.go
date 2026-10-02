package mps

import (
	"fmt"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

// weekSerial turns a date into the Excel 1900-system serial stored on row 56.
func weekSerial(t time.Time) float64 {
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return float64(t.Sub(base).Hours() / 24)
}

// The week block starts at column P. Reading it from one column earlier put the
// element type (column O) into the first week cell, shifted every number one
// column left and dropped the last week column entirely.
func TestReadFileMapsWeekColumnsStartingAtP(t *testing.T) {
	dir := t.TempDir()
	book := filepath.Join(dir, "columns.xlsx")

	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", SheetName); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for c := 1; c <= IndexCols; c++ {
		ref := CellRef(c, HeaderRow)
		if err := f.SetCellStr(SheetName, ref, fmt.Sprintf("H%d", c)); err != nil {
			t.Fatal(err)
		}
	}
	// readHeader insists that column L really is the LOC column.
	if err := f.SetCellStr(SheetName, CellRef(LOCCol, HeaderRow), "LOC"); err != nil {
		t.Fatal(err)
	}
	week, err := ParseWeekCode("2639")
	if err != nil {
		t.Fatalf("week code: %v", err)
	}
	const readColumns = 20
	for i := 0; i < readColumns; i++ {
		col := FirstWeekCol + i
		if err := f.SetCellStr(SheetName, CellRef(col, HeaderRow), week.Code); err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellFloat(SheetName, CellRef(col, DateRow), weekSerial(week.Start), -1, 64); err != nil {
			t.Fatal(err)
		}
	}

	// Column O carries the element type; P..AI carry the week numbers. Every
	// value is distinct so a one-column shift cannot go unnoticed.
	for c := 1; c <= IndexCols; c++ {
		if err := f.SetCellStr(SheetName, CellRef(c, FirstDataRow), fmt.Sprintf("IDX-%d", c)); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < readColumns; i++ {
		col := FirstWeekCol + i
		if err := f.SetCellFloat(SheetName, CellRef(col, FirstDataRow), float64(100+i), -1, 64); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SaveAs(book); err != nil {
		t.Fatalf("save: %v", err)
	}
	f.Close()

	fr, err := ReadFile(book, ReadOptions{LOCFilter: "IDX-12", ReadColumns: readColumns, Dict: NewStyleDict()})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if fr.Err != "" {
		t.Fatalf("read reported %s", fr.Err)
	}
	if fr.RowsKept != 1 {
		t.Fatalf("kept %d rows, want 1", fr.RowsKept)
	}
	row := fr.Rows[0]

	if got := row.Index[IndexCols-1]; got != "IDX-15" {
		t.Errorf("column O = %q, want IDX-15", got)
	}
	for i := 0; i < readColumns; i++ {
		want := strconv.Itoa(100 + i)
		if got := row.Weeks[i]; got != want {
			col, _ := ColumnName(FirstWeekCol + i)
			t.Errorf("week %d (%s) = %q, want %q", i, col, got, want)
		}
	}
	if got := row.Weeks[readColumns-1]; got != "119" {
		// The last column of the block (AI with the default 20) must be read,
		// not silently dropped off the end.
		t.Errorf("last week column = %q, want 119", got)
	}
}
