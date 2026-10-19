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
