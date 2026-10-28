package mps

import (
	"path/filepath"
	"testing"

	"github.com/xuri/excelize/v2"
)

// The Local workbook carries its own fills and notes; the reader has to keep
// them so the Local and merged sheets of the three-table export can reproduce
// the original page.
func TestReadLocalFileKeepsStylesAndComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Month End"
	_ = f.SetSheetName(f.GetSheetList()[0], sheet)
	for i, name := range LocalHeader {
		_ = f.SetCellValue(sheet, CellRef(i+1, 1), name)
	}
	_ = f.SetCellValue(sheet, "R1", "2639")
	_ = f.SetCellValue(sheet, "A4", "P5")
	_ = f.SetCellValue(sheet, "I4", "ITEM-1")
	_ = f.SetCellValue(sheet, "N4", "WH_CNB")
	_ = f.SetCellValue(sheet, "R4", 100)
	style, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFCC00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellStyle(sheet, "A4", "A4", style)
	_ = f.AddComment(sheet, excelize.Comment{Cell: "R4", Author: "me", Text: "备注"})
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	out, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Rows) != 1 {
		t.Fatalf("rows = %d", len(out.Rows))
	}
	if out.Dict == nil || out.Dict.Len() == 0 {
		t.Fatalf("style dictionary = %+v, want the Local cell style", out.Dict)
	}
	row := out.Rows[0]
	if len(row.StyleIDs) != IndexCols+1 || row.StyleIDs[0] == 0 && out.Dict.Len() == 0 {
		t.Fatalf("style ids = %v", row.StyleIDs)
	}
	if row.Comments[16].Text != "备注" || row.Comments[16].Author != "me" {
		t.Fatalf("comments = %+v", row.Comments)
	}
}

// The newer Local sheet uses the A..O MPS layout: LOC is L and the week codes
// start at P. It must be detected and normalised exactly like the older one.
func TestReadLocalFileNewLayout(t *testing.T) {
	path := filepath.Join(t.TempDir(), "local-new.xlsx")
	f := excelize.NewFile()
	defer f.Close()
	sheet := "Local"
	_ = f.SetSheetName(f.GetSheetList()[0], sheet)
	for i, name := range localMPSHeader {
		_ = f.SetCellValue(sheet, CellRef(i+1, 1), name)
	}
	_ = f.SetCellValue(sheet, "P1", "2643")
	_ = f.SetCellValue(sheet, "A4", "P5")
	_ = f.SetCellValue(sheet, "I4", "ITEM-1")
	_ = f.SetCellValue(sheet, "L4", "WH_CNB")
	_ = f.SetCellValue(sheet, "O4", "AdjDmd")
	_ = f.SetCellValue(sheet, "P4", 100)
	style, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFCC00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	_ = f.SetCellStyle(sheet, "A4", "A4", style)
	_ = f.AddComment(sheet, excelize.Comment{Cell: "P4", Author: "me", Text: "备注"})
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	out, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err != nil {
		t.Fatal(err)
	}
	if out.Sheet != "Local" || len(out.Rows) != 1 || out.RowsTotal != 1 {
		t.Fatalf("sheet/rows = %q %d/%d", out.Sheet, len(out.Rows), out.RowsTotal)
	}
	if len(out.Header.IndexNames) != IndexCols || out.Header.IndexNames[11] != "LOC" || out.Header.IndexNames[14] != "LOC" {
		t.Fatalf("index names = %v", out.Header.IndexNames)
	}
	if len(out.Header.Weeks) != 1 || out.Header.Weeks[0].Code != "2643" {
		t.Fatalf("weeks = %+v", out.Header.Weeks)
	}
	row := out.Rows[0]
	if row.Index[11] != "WH_CNB" || row.Index[14] != "AdjDmd" || row.Weeks[0] != "100" {
		t.Fatalf("row = %+v", row)
	}
	if row.Comments[16].Text != "备注" || row.Comments[16].Author != "me" {
		t.Fatalf("comments = %+v", row.Comments)
	}
	if out.Dict == nil || out.Dict.Len() == 0 {
		t.Fatalf("style dictionary = %+v", out.Dict)
	}
}

func TestLOCMatchesCommaSeparatedCaseInsensitive(t *testing.T) {
	cases := []struct {
		value, filters string
		want           bool
	}{
		{"WH_CNB", "WH_CNB", true},
		{"WH_CNS", "WH_CNB,WH_CNS", true},
		{"WH_CNS", "WH_CNB, WH_CNS", true},
		{"wh_cnb", "WH_CNB", true},
		{"WH_CNB", "WH_CNS", false},
		{"WH_CNB", "", true},
		{"WH_CNB", " , ", true},
	}
	for _, c := range cases {
		if got := LOCMatches(c.value, c.filters); got != c.want {
			t.Errorf("LOCMatches(%q, %q) = %v, want %v", c.value, c.filters, got, c.want)
		}
	}
}
