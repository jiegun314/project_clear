package master

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

func setRow(t *testing.T, f *excelize.File, sheet string, row int, values ...string) {
	t.Helper()
	cells := make([]any, len(values))
	for i, v := range values {
		cells[i] = v
	}
	if err := f.SetSheetRow(sheet, cellName(row, 1), &cells); err != nil {
		t.Fatalf("set %s row %d: %v", sheet, row, err)
	}
}

func cellName(row, col int) string {
	name, _ := excelize.CoordinatesToCellName(col, row)
	return name
}

func TestReadWorkbookReadsThreeMappingsAndDedupes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "match.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	if err := f.SetSheetName("Sheet1", "Item code-Base code"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.NewSheet("Base code-SAP code"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.NewSheet("SAP code-Category"); err != nil {
		t.Fatal(err)
	}

	setRow(t, f, "Item code-Base code", 1, "Item Code", "BASECODE")
	setRow(t, f, "Item code-Base code", 2, "I1", "B1")
	setRow(t, f, "Item code-Base code", 3, "I1", "B2") // conflicting duplicate
	setRow(t, f, "Item code-Base code", 4, "I2", "B2")

	setRow(t, f, "Base code-SAP code", 1, "Base code", "Manu Planning Code", "SAP Code", "formula")
	setRow(t, f, "Base code-SAP code", 2, "B1", "M1", "S1", "ignored")
	setRow(t, f, "Base code-SAP code", 3, "B1", "M1", "S1", "ignored") // exact duplicate
	setRow(t, f, "Base code-SAP code", 4, "B2", "M2", "S2", "ignored")
	setRow(t, f, "Base code-SAP code", 5, "*Item", "USYS", "USYS_BASECODE", "ignored")
	setRow(t, f, "Base code-SAP code", 6, "B2", "M2", "S9", "ignored") // conflicting duplicate

	setRow(t, f, "SAP code-Category", 1, "Code", "Category")
	setRow(t, f, "SAP code-Category", 2, "S1", "Cat A")
	setRow(t, f, "SAP code-Category", 3, "S1", "Cat A") // exact duplicate
	setRow(t, f, "SAP code-Category", 4, "S2", "Cat B")

	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	w, err := ReadWorkbook(path)
	if err != nil {
		t.Fatalf("ReadWorkbook: %v", err)
	}
	if len(w.Items) != 2 || len(w.Bases) != 2 || len(w.Categories) != 2 {
		t.Fatalf("got items=%d bases=%d categories=%d", len(w.Items), len(w.Bases), len(w.Categories))
	}
	if w.Items[0] != (ItemEntry{ItemCode: "I1", BaseCode: "B1", SourceRow: 2}) {
		t.Fatalf("item = %+v", w.Items[0])
	}
	if w.Bases[1] != (BaseEntry{BaseCode: "B2", ManuCode: "M2", SapCode: "S2", SourceRow: 4}) {
		t.Fatalf("base = %+v", w.Bases[1])
	}
	if w.Categories[0] != (CategoryEntry{SapCode: "S1", Category: "Cat A", SourceRow: 2}) {
		t.Fatalf("category = %+v", w.Categories[0])
	}
	if w.DuplicateRows != 4 {
		t.Fatalf("DuplicateRows = %d, want 4", w.DuplicateRows)
	}
	if len(w.Warnings) != 2 {
		t.Fatalf("warnings = %v, want 2", w.Warnings)
	}
	if !strings.Contains(w.Warnings[0], "Item I1") || !strings.Contains(w.Warnings[1], "Base Code B2") {
		t.Fatalf("warnings = %v", w.Warnings)
	}
}

func TestReadWorkbookRejectsMissingSheets(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	setRow(t, f, "Sheet1", 1, "Something", "Else")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadWorkbook(path); err == nil {
		t.Fatal("expected a missing-sheet error")
	}
}

func TestReadPriceWorkbookReadsLastDuplicateAndKeepsBlankCost(t *testing.T) {
	path := filepath.Join(t.TempDir(), "price.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	setRow(t, f, "Sheet1", 1, "MARC_MATNR", "Report")
	setRow(t, f, "Sheet1", 2, "Material", "Description", "Standard_Cost")
	setRow(t, f, "Sheet1", 3, "000.009", "HOLDER", "107.36")
	setRow(t, f, "Sheet1", 4, "000.010", "NO PRICE", "")
	setRow(t, f, "Sheet1", 5, "000.011", "COMMA", "1,234.50")
	setRow(t, f, "Sheet1", 6, "000.009", "UPDATED", "108.00")
	setRow(t, f, "Sheet1", 7, "BAD", "NOT A NUMBER", "n/a")
	setRow(t, f, "Sheet1", 8, "", "NO MATERIAL", "10.00")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}

	w, err := ReadPriceWorkbook(path)
	if err != nil {
		t.Fatalf("ReadPriceWorkbook: %v", err)
	}
	if len(w.Prices) != 3 {
		t.Fatalf("prices = %+v, want 3", w.Prices)
	}
	if w.DuplicateRows != 1 || w.MissingCosts != 1 || w.SkippedRows != 2 {
		t.Fatalf("duplicates=%d missing=%d skipped=%d", w.DuplicateRows, w.MissingCosts, w.SkippedRows)
	}
	if len(w.Warnings) != 3 {
		t.Fatalf("warnings = %v, want 3", w.Warnings)
	}
	first := w.Prices[0]
	if first.Material != "000.009" || first.StandardCost == nil || *first.StandardCost != 108 {
		t.Fatalf("first price = %+v", first)
	}
	if w.Prices[1].Material != "000.010" || w.Prices[1].StandardCost != nil {
		t.Fatalf("blank price = %+v", w.Prices[1])
	}
	if w.Prices[2].StandardCost == nil || *w.Prices[2].StandardCost != 1234.5 {
		t.Fatalf("comma price = %+v", w.Prices[2])
	}
}

func TestReadPriceWorkbookRejectsMissingHeaders(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad-price.xlsx")
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })
	setRow(t, f, "Sheet1", 1, "Something", "Else")
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadPriceWorkbook(path); err == nil {
		t.Fatal("expected a missing-header error")
	}
}

func TestSortCategories(t *testing.T) {
	got := SortCategories([]string{"Zeta", "", "alpha", "ALPHA", "Beta "})
	want := []string{"alpha", "Beta", "Zeta"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %v, want %v", got, want)
	}
}
