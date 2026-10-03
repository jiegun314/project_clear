package mps

import (
	"testing"

	"github.com/xuri/excelize/v2"
)

func TestParseMergeRanges(t *testing.T) {
	xml := []byte(`<worksheet><mergeCells count="3">` +
		`<mergeCell ref="A58:A71"/>` +
		`<mergeCell ref="B58:M58"/>` +
		`<mergeCell ref="P58"/>` +
		`</mergeCells></worksheet>`)
	got := ParseMergeRanges(xml)
	if len(got) != 3 {
		t.Fatalf("ranges = %+v, want 3", got)
	}
	if got[0] != (MergeRange{Col1: 1, Row1: 58, Col2: 1, Row2: 71}) {
		t.Errorf("vertical merge = %+v", got[0])
	}
	if got[1] != (MergeRange{Col1: 2, Row1: 58, Col2: 13, Row2: 58}) {
		t.Errorf("horizontal merge = %+v", got[1])
	}
	if got[2] != (MergeRange{Col1: 16, Row1: 58, Col2: 16, Row2: 58}) {
		t.Errorf("single cell = %+v", got[2])
	}
}

// 合并单元格的内容在源文件里只存在于左上角，读取时必须补到它覆盖的每一行，
// 否则窗口里除第一行外都是空的。
func TestReadFileRepeatsMergedCellContent(t *testing.T) {
	dir := t.TempDir()
	book := writeMergedWorkbook(t, dir, "merged.xlsx")

	res, err := ReadFile(book, ReadOptions{
		LOCFilter:   "WH_CNB",
		ReadColumns: 1,
		Dict:        NewStyleDict(),
		Dxf:         NewDxfDict(),
	})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if res.Err != "" {
		t.Fatalf("read error: %s", res.Err)
	}
	if len(res.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(res.Rows))
	}
	// Column G (MFG CLASS CODE) is merged over the three data rows.
	for i, row := range res.Rows {
		if got := row.Index[6]; got != "CLASS-X" {
			t.Errorf("第 %d 行第 7 列 = %q, want CLASS-X（合并单元格内容需重复出现）", i+1, got)
		}
	}
	if got := res.Rows[1].Index[5]; got != "GROUP-A" {
		t.Errorf("第一列合并 = %q, want GROUP-A", got)
	}
}

// writeMergedWorkbook builds a minimal MPS-shaped workbook whose column F is
// merged over the data rows and column G over the whole block.
func writeMergedWorkbook(t *testing.T, dir, name string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", SheetName); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for c := 1; c <= IndexCols; c++ {
		if err := f.SetCellStr(SheetName, CellRef(c, HeaderRow), "H"); err != nil {
			t.Fatal(err)
		}
	}
	if err := f.SetCellStr(SheetName, CellRef(LOCCol, HeaderRow), "LOC"); err != nil {
		t.Fatal(err)
	}
	week, err := ParseWeekCode("2639")
	if err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStr(SheetName, CellRef(FirstWeekCol, HeaderRow), week.Code); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellFloat(SheetName, CellRef(FirstWeekCol, DateRow), weekSerial(week.Start), -1, 64); err != nil {
		t.Fatal(err)
	}
	for r := FirstDataRow; r < FirstDataRow+3; r++ {
		if err := f.SetCellStr(SheetName, CellRef(LOCCol, r), "WH_CNB"); err != nil {
			t.Fatal(err)
		}
		if err := f.SetCellFloat(SheetName, CellRef(FirstWeekCol, r), float64(r), -1, 64); err != nil {
			t.Fatal(err)
		}
	}
	// F 列每行一个合并块（横向 + 由锚点补齐），G 列整块纵向合并。
	if err := f.SetCellStr(SheetName, CellRef(6, FirstDataRow), "GROUP-A"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellStr(SheetName, CellRef(7, FirstDataRow), "CLASS-X"); err != nil {
		t.Fatal(err)
	}
	_ = f.MergeCell(SheetName, CellRef(6, FirstDataRow), CellRef(6, FirstDataRow+2))
	_ = f.MergeCell(SheetName, CellRef(7, FirstDataRow), CellRef(7, FirstDataRow+2))

	path := dir + "/" + name
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}
