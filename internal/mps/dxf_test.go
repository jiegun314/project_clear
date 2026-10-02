package mps

import (
	"encoding/xml"
	"strings"
	"testing"
)

// mustParseXML is the check that matters for a styles part: a broken document
// is one Excel refuses to open.
func mustParseXML(t *testing.T, what string, doc []byte) {
	t.Helper()
	var v any
	if err := xml.Unmarshal(doc, &v); err != nil {
		t.Fatalf("%s 不是合法 XML: %v\n%s", what, err, truncateDoc(doc))
	}
}

func truncateDoc(doc []byte) string {
	s := string(doc)
	if len(s) > 400 {
		return s[:200] + " … " + s[len(s)-200:]
	}
	return s
}

const dxfRed = `<dxf><font><color rgb="FFFF0000"/></font></dxf>`
const dxfBlue = `<dxf><fill><patternFill><bgColor rgb="FF00B0F0"/></patternFill></fill></dxf>`

func stylesWith(dxfs ...string) []byte {
	return []byte(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<styleSheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<fonts count="1"><font><sz val="11"/></font></fonts>` +
		`<cellXfs count="2"><xf numFmtId="0"/><xf numFmtId="0"/></cellXfs>` +
		`<cellStyles count="1"><cellStyle name="Normal" xfId="0" builtinId="0"/></cellStyles>` +
		`<dxfs count="` + itoa(len(dxfs)) + `">` + strings.Join(dxfs, "") + `</dxfs>` +
		`<tableStyles count="0"/></styleSheet>`)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

func TestParseDxfsReadsTableOrder(t *testing.T) {
	got := ParseDxfs(stylesWith(dxfRed, dxfBlue))
	if len(got) != 2 || got[0] != dxfRed || got[1] != dxfBlue {
		t.Fatalf("ParseDxfs = %v", got)
	}
}

// The whole point: rules from several workbooks share one table in the export,
// and a format the workbook already has must keep its own index.
func TestMergeDxfsAppendsAndReuses(t *testing.T) {
	merged, at, err := MergeDxfs(stylesWith(dxfRed), []string{dxfBlue, dxfRed})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	mustParseXML(t, "合并后的样式表", merged)
	if len(at) != 2 || at[0] != 1 || at[1] != 0 {
		t.Fatalf("index = %v, want the batch's blue at 1 and the known red at 0", at)
	}
	if got := ParseDxfs(merged); len(got) != 2 {
		t.Fatalf("merged table = %v", got)
	}
	if !strings.Contains(string(merged), `count="2"`) {
		t.Errorf("count attribute not updated: %s", truncateDoc(merged))
	}
}

// excelize writes the placeholder in the paired form; replacing only its
// opening tag used to leave a stray </dxfs> and break the styles part.
func TestMergeDxfsIntoEmptyPairedPlaceholder(t *testing.T) {
	target := []byte(`<styleSheet><cellStyles count="1"><cellStyle name="Normal"/></cellStyles>` +
		`<dxfs count="0"></dxfs><tableStyles count="0"/></styleSheet>`)
	merged, at, err := MergeDxfs(target, []string{dxfRed})
	if err != nil || at[0] != 0 {
		t.Fatalf("merge: at=%v err=%v", at, err)
	}
	mustParseXML(t, "样式表", merged)
	if strings.Count(string(merged), "</dxfs>") != 1 {
		t.Errorf("expected exactly one </dxfs>: %s", truncateDoc(merged))
	}
}

func TestMergeDxfsInsertedAfterCellStyles(t *testing.T) {
	target := []byte(`<styleSheet><cellXfs count="1"><xf/></cellXfs>` +
		`<cellStyles count="1"><cellStyle name="Normal"/></cellStyles>` +
		`<tableStyles count="0"/></styleSheet>`)
	merged, _, err := MergeDxfs(target, []string{dxfRed})
	if err != nil {
		t.Fatalf("merge: %v", err)
	}
	mustParseXML(t, "样式表", merged)
	doc := string(merged)
	if !(strings.Index(doc, "</cellStyles>") < strings.Index(doc, "<dxfs") &&
		strings.Index(doc, "<dxfs") < strings.Index(doc, "<tableStyles")) {
		t.Errorf("dxfs placed outside cellStyles..tableStyles: %s", doc)
	}
}

func TestRemapDxfIDs(t *testing.T) {
	rule := CFRule{Attrs: ` type="expression" dxfId="1943" priority="7" stopIfTrue="1"`}
	if id, ok := rule.DxfID(); !ok || id != 1943 {
		t.Fatalf("DxfID = %d %v", id, ok)
	}
	rows := map[int]CFRow{58: {Blocks: []CFBlock{{C1: 16, C2: 35, Rules: []CFRule{rule}}}}}
	RemapDxfIDs(rows, func(id int) (int, bool) { return 12, true })

	got := rows[58].Blocks[0].Rules[0]
	if id, _ := got.DxfID(); id != 12 {
		t.Errorf("dxfId = %d, want 12", id)
	}
	if !strings.Contains(got.Attrs, `stopIfTrue="1"`) || !strings.Contains(got.Attrs, `priority="7"`) {
		t.Errorf("other attributes were lost: %q", got.Attrs)
	}
	if strings.Count(got.Attrs, "dxfId=") != 1 {
		t.Errorf("dxfId duplicated: %q", got.Attrs)
	}

	// An id with no home drops the reference rather than dangling.
	RemapDxfIDs(rows, func(int) (int, bool) { return 0, false })
	if got := rows[58].Blocks[0].Rules[0]; strings.Contains(got.Attrs, "dxfId") {
		t.Errorf("unresolved id should be dropped, got %q", got.Attrs)
	}
}

// A rule whose id is outside its own file's table cannot be resolved, so the
// reference must go rather than point at whatever sits at that index.
func TestRemapSourceDxfsDropsOutOfRangeIDs(t *testing.T) {
	rows := map[int]CFRow{
		58: {Blocks: []CFBlock{{Rules: []CFRule{
			{Attrs: ` type="expression" dxfId="1"`},
			{Attrs: ` type="expression" dxfId="0"`},
			{Attrs: ` type="expression" dxfId="99"`},
			{Attrs: ` type="colorScale"`},
		}}}},
	}
	dict := NewDxfDict()
	remapSourceDxfs(rows, []string{dxfRed, dxfBlue}, dict)

	rules := rows[58].Blocks[0].Rules
	// The stored id is a dictionary id now, so what matters is what it points
	// at — and that it is not the source file's number.
	blueID, ok := rules[0].DxfID()
	if !ok {
		t.Fatalf("in-range id was dropped: %q", rules[0].Attrs)
	}
	if got, _ := dict.At(blueID); got != dxfBlue {
		t.Errorf("id %d resolves to %q, want the blue fill", blueID, got)
	}
	redID, _ := rules[1].DxfID()
	if got, _ := dict.At(redID); got != dxfRed {
		t.Errorf("id %d resolves to %q, want the red font", redID, got)
	}
	if blueID == redID {
		t.Errorf("two different formats collapsed onto one id")
	}
	if _, ok := rules[2].DxfID(); ok {
		t.Errorf("out-of-range id survived: %q", rules[2].Attrs)
	}
	if _, ok := rules[3].DxfID(); ok {
		t.Errorf("a rule without a format gained one: %q", rules[3].Attrs)
	}
	if dict.Len() != 2 {
		t.Errorf("dict = %d entries, want 2", dict.Len())
	}
}
