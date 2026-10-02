package mps

import (
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// The export carries the report's data, not its decoration: the header block
// that the source keeps on rows 55..57 is the first thing in the exported
// sheet, and nothing from rows 1..54 comes along.
func TestExportDropsTheDecorativeRows(t *testing.T) {
	template := workbookWithComments(t, t.TempDir(), commentFixture{})
	dest, _ := exportFixture(t, template, CommentMap{})

	f, err := excelize.OpenFile(dest)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer f.Close()
	rows, err := f.GetRows(SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		t.Fatalf("read export: %v", err)
	}

	if got := cell(rows, 0, 0); got != "P5" {
		t.Errorf("A%d = %q, want the index header", ExportHeaderRow, got)
	}
	if got := cell(rows, 0, FirstWeekCol-1); got != "2639" {
		t.Errorf("P%d = %q, want the week code", ExportHeaderRow, got)
	}
	if got := cell(rows, 1, FirstWeekCol-1); got != "46286" {
		t.Errorf("P%d = %q, want the week start serial", ExportDateRow, got)
	}
	if got := cell(rows, ExportFirstDataRow-1, 0); got != "P5_EP_BW" {
		t.Errorf("A%d = %q, want the first merged row", ExportFirstDataRow, got)
	}

	// Nothing from the decorative block survives anywhere in the sheet.
	for r, row := range rows {
		for c, v := range row {
			if strings.Contains(v, "SUMMARY") || strings.Contains(v, "decorative") {
				t.Errorf("decorative content kept at %s: %q", CellRef(c+1, r+1), v)
			}
		}
	}
}

func cell(rows [][]string, r, c int) string {
	if r < 0 || r >= len(rows) || c < 0 || c >= len(rows[r]) {
		return ""
	}
	return rows[r][c]
}
