package mps

import (
	"encoding/xml"
	"io"
	"strings"
	"testing"
)

// A trimmed sample that mirrors the real workbooks: four blocks share one
// sqref, the rules compare a row with its neighbour, and an absolute header
// reference anchors the tolerance band.
const sampleSheet = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
<sheetData>
<row r="55"><c r="P55" s="204"><v>2639</v></c></row>
<row r="58"><c r="L58" s="185" t="s"><v>0</v></c></row>
</sheetData>
<conditionalFormatting sqref="P60:CO60">
<cfRule type="expression" dxfId="296" priority="362" stopIfTrue="1"><formula>(ROUND(P60,0)&lt;0)</formula></cfRule>
</conditionalFormatting>
<conditionalFormatting sqref="P60:CO60">
<cfRule type="expression" dxfId="295" priority="363" stopIfTrue="1"><formula>(ROUND(P60,0)&lt;P61)</formula></cfRule>
</conditionalFormatting>
<conditionalFormatting sqref="P60:CO60">
<cfRule type="expression" dxfId="294" priority="364" stopIfTrue="1"><formula>(ROUND(P60,0)&gt;=P61*(1+$DN$56))</formula></cfRule>
</conditionalFormatting>
<conditionalFormatting sqref="P60:CO60">
<cfRule type="expression" dxfId="293" priority="365" stopIfTrue="1"><formula>AND(P60&gt;=0,P60&gt;=P61,P60&lt;P61*(1+$DN$56))</formula></cfRule>
</conditionalFormatting>
<conditionalFormatting sqref="P64:CO64">
<cfRule type="expression" dxfId="292" priority="366" stopIfTrue="1"><formula>(ROUND(P64,0)&gt;LOG10(P66))</formula></cfRule>
</conditionalFormatting>
<conditionalFormatting sqref="A1:O1">
<cfRule type="expression" dxfId="1" priority="1" stopIfTrue="1"><formula>LEN($A$1)&gt;0</formula></cfRule>
</conditionalFormatting>
<pageMargins left="0.25" right="0.16" top="1" bottom="1" header="0.5" footer="0.5"/>
</worksheet>`

func TestParseCFRowsKeepsEveryBlockSharingASqref(t *testing.T) {
	rows, err := ParseCFRows([]byte(sampleSheet))
	if err != nil {
		t.Fatalf("ParseCFRows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected rows 60 and 64 only, got %v", keysOf(rows))
	}
	row := rows[60]
	if len(row.Blocks) != 4 {
		t.Fatalf("expected 4 blocks for row 60, got %d", len(row.Blocks))
	}
	for i, b := range row.Blocks {
		if b.C1 != FirstWeekCol {
			t.Errorf("block %d: c1=%d want %d", i, b.C1, FirstWeekCol)
		}
	}
}

func TestRebaseTurnsNeighbourRefsIntoOffsets(t *testing.T) {
	rows, _ := ParseCFRows([]byte(sampleSheet))
	got := rows[60].Blocks[1].Rules[0].Formulas[0]
	// P60 stays at offset 0, P61 becomes +1.
	if want := "(ROUND(P#o0,0)<P#o1)"; got != want {
		t.Fatalf("rebase = %q, want %q", got, want)
	}
	got = rows[60].Blocks[2].Rules[0].Formulas[0]
	// $DN$56 is a row-absolute reference into the header, so it travels with
	// the sheet: it is stored as a token and resolved on export.
	if want := "(ROUND(P#o0,0)>=P#o1*(1+$DN$#a56))"; got != want {
		t.Fatalf("rebase = %q, want %q", got, want)
	}
	if offs := rows[60].Blocks[2].Rules[0].Offsets; len(offs) != 2 || offs[0] != 0 || offs[1] != 1 {
		t.Fatalf("offsets = %v, want [0 1]", offs)
	}
}

func TestApplyOffsetsReanchorsOntoTheNewRow(t *testing.T) {
	rows, _ := ParseCFRows([]byte(sampleSheet))
	rule := rows[60].Blocks[2].Rules[0]
	// The row that used to be 60 is now 1000; its neighbour reference follows
	// it to 1001, and the date row it compares against (source row 56) lands on
	// exported row 2.
	if got, want := applyOffsets(rule.Formulas[0], 1000), "(ROUND(P1000,0)>=P1001*(1+$DN$2))"; got != want {
		t.Fatalf("applyOffsets = %q, want %q", got, want)
	}
}

// A stored program written before the export started dropping the decorative
// rows still carries "$DN$56" in full; it must be moved up too.
func TestApplyOffsetsShiftsLiteralAbsoluteRows(t *testing.T) {
	got := resolveRowRefs(shiftLiteralRows("(P1000,0)>=P1001*(1+$DN$56)"), 1000)
	if want := "(P1000,0)>=P1001*(1+$DN$2)"; got != want {
		t.Fatalf("literal shift = %q, want %q", got, want)
	}
}

func TestFunctionNamesAreNotMistakenForReferences(t *testing.T) {
	// LOG10( must survive intact, while P64 and P66 become offsets.
	rows, _ := ParseCFRows([]byte(sampleSheet))
	got := rows[64].Blocks[0].Rules[0].Formulas[0]
	if want := "(ROUND(P#o0,0)>LOG10(P#o2))"; got != want {
		t.Fatalf("rebase = %q, want %q", got, want)
	}
}

func TestHeaderAreaFormatsAreLeftOutOfTheDataCapture(t *testing.T) {
	rows, _ := ParseCFRows([]byte(sampleSheet))
	if _, ok := rows[1]; ok {
		t.Fatal("header-area conditional format leaked into the data capture")
	}
}

func TestStripKeepsHeaderBlocksAndDropsDataBlocks(t *testing.T) {
	stripped := string(StripDataAreaCF([]byte(sampleSheet)))
	if strings.Contains(stripped, `dxfId="296"`) || strings.Contains(stripped, `dxfId="292"`) {
		t.Error("data-area conditional formats survived the strip")
	}
	if !strings.Contains(stripped, `dxfId="1"`) {
		t.Error("header-area conditional format was removed")
	}
	if !strings.Contains(stripped, "<pageMargins") {
		t.Error("strip corrupted the rest of the document")
	}
}

func TestRenderPutsBlocksBeforePageMargins(t *testing.T) {
	rows, _ := ParseCFRows([]byte(sampleSheet))
	delete(rows, 64)
	xmlStr, next, err := RenderCFRows(rows, 10)
	if err != nil {
		t.Fatalf("RenderCFRows: %v", err)
	}
	if next != 14 {
		t.Errorf("next priority = %d, want 14", next)
	}
	merged, err := insertBeforeSheetEnd(string(StripDataAreaCF([]byte(sampleSheet))), xmlStr)
	if err != nil {
		t.Fatalf("insertBeforeSheetEnd: %v", err)
	}
	i := strings.Index(merged, `<conditionalFormatting sqref="P60:CO60"`)
	j := strings.Index(merged, "<pageMargins")
	if i < 0 || j < 0 || i > j {
		t.Fatalf("conditional formatting must sit before <pageMargins (i=%d j=%d)", i, j)
	}
	if !strings.Contains(merged, "<formula>(ROUND(P60,0)&lt;P61)</formula>") {
		t.Error("rendered formula lost its escaping or its re-anchored value")
	}
}

func TestRenderRewritesPrioritiesAboveTheSurvivors(t *testing.T) {
	// The header block keeps priority 1, so injected rules must start above it.
	rows, _ := ParseCFRows([]byte(sampleSheet))
	start := MaxCFPriority(StripDataAreaCF([]byte(sampleSheet))) + 1
	if start != 2 {
		t.Fatalf("MaxCFPriority after strip = %d, want 1 (only the header block survives)", start-1)
	}
	xmlStr, _, _ := RenderCFRows(rows, start)
	if !strings.Contains(xmlStr, `priority="2"`) {
		t.Error("injected rules did not restart the priority sequence")
	}
}

func TestParseWeekCode(t *testing.T) {
	cases := []struct {
		code  string
		start string
	}{
		{"2639", "2026-09-21"},
		{"2701", "2027-01-04"},
		{"2601", "2025-12-29"},
	}
	for _, c := range cases {
		w, err := ParseWeekCode(c.code)
		if err != nil {
			t.Fatalf("ParseWeekCode(%q): %v", c.code, err)
		}
		if got := w.StartText(); got != c.start {
			t.Errorf("ParseWeekCode(%q).Start = %s, want %s", c.code, got, c.start)
		}
	}
	for _, bad := range []string{"", "abc", "2699", "2600", "263", "26.39x"} {
		if _, err := ParseWeekCode(bad); err == nil {
			t.Errorf("ParseWeekCode(%q) should have failed", bad)
		}
	}
}

func TestWeekFromSerialMatchesTheHeaderRow(t *testing.T) {
	// 46286 is the serial stored on row 56 for week 2639.
	d, err := WeekFromSerial(46286)
	if err != nil {
		t.Fatalf("WeekFromSerial: %v", err)
	}
	if got := d.Format("2006-01-02"); got != "2026-09-21" {
		t.Fatalf("46286 = %s, want 2026-09-21", got)
	}
}

func TestColumnNameRoundTrip(t *testing.T) {
	for _, c := range []struct {
		n    int
		name string
	}{{1, "A"}, {15, "O"}, {16, "P"}, {35, "AI"}, {93, "CO"}} {
		got, err := ColumnName(c.n)
		if err != nil || got != c.name {
			t.Fatalf("ColumnName(%d) = %q,%v want %q", c.n, got, err, c.name)
		}
		back, err := ColumnNumber(got)
		if err != nil || back != c.n {
			t.Fatalf("ColumnNumber(%q) = %d,%v want %d", got, back, err, c.n)
		}
	}
}

func keysOf(m map[int]CFRow) []int {
	out := make([]int, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sortInts(out)
	return out
}

// A malformed injected element silently destroys the whole sheet: excelize
// fails to parse it and every cell reads back as empty. Assert the emitted XML
// is well formed and the attributes are not prefixed by the tag name.
func TestRenderEmitsWellFormedRuleTags(t *testing.T) {
	rows, err := ParseCFRows([]byte(sampleSheet))
	if err != nil {
		t.Fatalf("ParseCFRows: %v", err)
	}
	xmlStr, _, err := RenderCFRows(rows, 1)
	if err != nil {
		t.Fatalf("RenderCFRows: %v", err)
	}
	if strings.Contains(xmlStr, "<cfRulecfRule") {
		t.Fatalf("element name leaked into the attribute list: %s", firstN(xmlStr, 120))
	}
	// The whole fragment must parse as XML.
	probe := `<root xmlns="` + nsMain + `">` + xmlStr + `</root>`
	dec := xml.NewDecoder(strings.NewReader(probe))
	for {
		_, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("rendered conditional formatting is not well formed: %v\n%s", err, firstN(xmlStr, 300))
		}
	}
	if !strings.Contains(xmlStr, `dxfId="296"`) {
		t.Error("rule lost its dxf reference")
	}
}

func firstN(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
