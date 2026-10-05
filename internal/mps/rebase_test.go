package mps

import (
	"strings"
	"testing"
)

// The real template sheet part: the header block on rows 55..57, a frozen pane
// whose split is measured in source rows, a used range spanning the whole
// report and a filter over the source's own data rows.
func templateSheetXML() []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<sheetPr codeName="Sheet20"/>` +
		`<dimension ref="A1:PD99999">` +
		`<sheetViews><sheetView tabSelected="true" zoomScale="85" workbookViewId="0">` +
		`<pane xSplit="15" ySplit="57" topLeftCell="AO114" activePane="bottomRight" state="frozen"/>` +
		`<selection pane="bottomRight" activeCell="K420" sqref="K420"/>` +
		`</sheetView></sheetViews>` +
		`<sheetFormatPr defaultRowHeight="15"/>` +
		`<sheetData>` +
		`<row r="55"><c r="A55" s="1" t="s"><v>0</v></c><c r="P55" s="2" t="s"><v>1</v></c></row>` +
		`<row r="56"><c r="P56" s="2"><v>46286</v></c></row>` +
		`<row r="57"><c r="A57" s="211"/></row>` +
		`</sheetData>` +
		`<autoFilter ref="A57:DO435"/>` +
		`<mergeCells count="1"><mergeCell ref="A55:B55"/></mergeCells>` +
		`<pageMargins left="0.7" right="0.7" top="0.75" bottom="0.75" header="0.3" footer="0.3"/>` +
		`</worksheet>`)
}

func sheetExcerpt(doc, marker string, n int) string {
	at := strings.Index(doc, marker)
	if at < 0 {
		return "(no " + marker + ")"
	}
	if end := at + n; end < len(doc) {
		return doc[at:end]
	}
	return doc[at:]
}

// The frozen pane and the filter range are written in source row numbers, so
// rebasing only the cells would leave the export freezing the top of the data
// block and filtering rows that no longer hold that data.
func TestRebaseSheetXMLShiftsTheFrozenPaneAndFilter(t *testing.T) {
	out, err := rebaseSheetXML(templateSheetXML(), 35)
	if err != nil {
		t.Fatalf("rebaseSheetXML: %v", err)
	}
	got := string(out)

	// 57 source rows minus the 54 dropped ones: the split now sits under the
	// three header rows it used to sit under.
	if !strings.Contains(got, `ySplit="3"`) {
		t.Errorf("frozen split not rebased: %s", sheetExcerpt(got, "<pane", 140))
	}
	if !strings.Contains(got, `topLeftCell="AO60"`) {
		t.Errorf("pane scroll anchor not rebased: %s", sheetExcerpt(got, "<pane", 140))
	}
	if !strings.Contains(got, `<autoFilter ref="A3:DO381"`) {
		t.Errorf("autoFilter not rebased: %s", sheetExcerpt(got, "<autoFilter", 80))
	}
	// The frozen columns are untouched: the index block keeps its width.
	if !strings.Contains(got, `xSplit="15"`) {
		t.Errorf("frozen columns lost: %s", sheetExcerpt(got, "<pane", 140))
	}
	for _, stale := range []string{`ySplit="57"`, "AO114", `ref="A57:DO435"`} {
		if strings.Contains(got, stale) {
			t.Errorf("source row number %q survived the rebase: %s", stale, sheetExcerpt(got, "<pane", 140))
		}
	}
	// The used range describes the rebuilt sheet.
	if !strings.Contains(got, "A1:AI3") {
		t.Errorf("dimension not rebased: %s", sheetExcerpt(got, "<dimension", 60))
	}
}

// A pane measured inside the dropped rows has no row left to freeze, and a
// reference above the header must not become a non-existent row 0 or negative.
func TestRebaseSheetXMLClampsRowsAtTheHeader(t *testing.T) {
	doc := []byte(`<worksheet>` +
		`<dimension ref="A1:PD99999">` +
		`<sheetViews><sheetView><pane ySplit="10" topLeftCell="A5" state="frozen"/></sheetView></sheetViews>` +
		`<sheetData><row r="55"><c r="A55" s="1"/></row><row r="56"/><row r="57"/></sheetData>` +
		`<autoFilter ref="A1:Z20"/>` +
		`</worksheet>`)

	out, err := rebaseSheetXML(doc, 20)
	if err != nil {
		t.Fatalf("rebaseSheetXML: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, `ySplit="1"`) {
		t.Errorf("split not clamped to the first row: %s", sheetExcerpt(got, "<pane", 120))
	}
	if !strings.Contains(got, `topLeftCell="A1"`) {
		t.Errorf("scroll anchor not clamped: %s", sheetExcerpt(got, "<pane", 120))
	}
	if !strings.Contains(got, `<autoFilter ref="A1:Z1"`) {
		t.Errorf("filter range not clamped: %s", sheetExcerpt(got, "<autoFilter", 60))
	}
	if strings.Contains(got, `="0"`) || strings.Contains(got, `="-"`) || strings.Contains(got, `="-1"`) {
		t.Errorf("rebase produced a non-existent row: %s", sheetExcerpt(got, "<pane", 120))
	}
}

// Only the attributes that are actually present are rewritten.
func TestRebaseSheetXMLShiftsOnlyTheAttributesPresent(t *testing.T) {
	doc := []byte(`<worksheet><dimension ref="A1:PD99999">` +
		`<sheetViews><sheetView><pane xSplit="15" topLeftCell="P100" state="frozen"/></sheetView></sheetViews>` +
		`<sheetData><row r="55"><c r="A55" s="1"/></row><row r="56"/><row r="57"/></sheetData>` +
		`</worksheet>`)

	out, err := rebaseSheetXML(doc, 20)
	if err != nil {
		t.Fatalf("rebaseSheetXML: %v", err)
	}
	got := string(out)

	if !strings.Contains(got, `xSplit="15"`) {
		t.Errorf("frozen columns lost: %s", sheetExcerpt(got, "<pane", 120))
	}
	if !strings.Contains(got, `topLeftCell="P46"`) {
		t.Errorf("scroll anchor not shifted: %s", sheetExcerpt(got, "<pane", 120))
	}
}

// A sheet without a pane or a filter must come through untouched, not fail.
func TestRebaseSheetXMLLeavesAbsentPaneAndFilterAlone(t *testing.T) {
	doc := []byte(`<worksheet><dimension ref="A1:PD99999">` +
		`<sheetData><row r="55"><c r="A55" s="1"/></row><row r="56"/><row r="57"/></sheetData>` +
		`</worksheet>`)

	out, err := rebaseSheetXML(doc, 20)
	if err != nil {
		t.Fatalf("rebaseSheetXML: %v", err)
	}
	for _, unwanted := range []string{"<pane", "<autoFilter"} {
		if strings.Contains(string(out), unwanted) {
			t.Errorf("rebase invented %s: %s", unwanted, out)
		}
	}
}

// Both engines leave the used range pointing at the header, so the saved file
// would declare a range far smaller than the rows it actually carries.
// Streaming readers trust that declaration and see an empty workbook.
func TestFixSheetDimensionCoversTheDataRows(t *testing.T) {
	doc := []byte(`<worksheet><dimension ref="A1:AI3">` +
		`<sheetData><row r="1"><c r="A1" s="1"/></row><row r="2"/><row r="3"/>` +
		`<row r="4"><c r="A4" s="1"><v>1</v></c></row>` +
		`<row r="1519"><c r="A1519" s="1"><v>1</v></c></row>` +
		`</sheetData></worksheet>`)

	got := string(fixSheetDimension(doc, 35))
	if !strings.Contains(got, "A1:AI1519") {
		t.Errorf("dimension does not cover the data: %s", sheetExcerpt(got, "<dimension", 60))
	}
	if strings.Contains(got, "A1:AI3") {
		t.Errorf("header-only dimension survived: %s", sheetExcerpt(got, "<dimension", 60))
	}
}

// The clean engine starts from a workbook whose used range is a single cell.
func TestFixSheetDimensionExpandsASingleCellRange(t *testing.T) {
	doc := []byte(`<worksheet><dimension ref="A1"/>` +
		`<sheetData><row r="1"/><row r="1519"><c r="A1519"><v>1</v></c></row></sheetData>` +
		`</worksheet>`)

	got := string(fixSheetDimension(doc, 35))
	if !strings.Contains(got, "A1:AI1519") {
		t.Errorf("dimension not expanded: %s", sheetExcerpt(got, "<dimension", 60))
	}
}

// A sheet with no rows at all is left alone rather than given A1:A1.
func TestFixSheetDimensionIgnoresAnEmptySheet(t *testing.T) {
	doc := []byte(`<worksheet><dimension ref="A1:AI3"/><sheetData/></worksheet>`)
	if got := string(fixSheetDimension(doc, 35)); got != string(doc) {
		t.Errorf("empty sheet rewritten:\n got %s\nwant %s", got, doc)
	}
}

func TestMaxRowNumber(t *testing.T) {
	doc := `<worksheet><sheetData>` +
		`<row r="5" spans="1:3"><c r="A5"/></row>` +
		`<row r="12"/>` +
		`</sheetData>` +
		`<rowBreaks count="0"/><cols><col min="1" max="1"/></cols></worksheet>`

	if got := maxRowNumber(doc); got != 12 {
		t.Errorf("maxRowNumber = %d, want 12", got)
	}
	if got := maxRowNumber(`<worksheet><sheetData/></worksheet>`); got != 0 {
		t.Errorf("maxRowNumber(empty) = %d, want 0", got)
	}
	// A row tag whose attribute value contains '>' must not derail the scan.
	if got := maxRowNumber(`<sheetData><row r="7" customFormat="a&gt;b"/></sheetData>`); got != 7 {
		t.Errorf("maxRowNumber with a quoted '>' = %d, want 7", got)
	}
}
