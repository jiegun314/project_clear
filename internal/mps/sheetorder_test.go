package mps

import (
	"encoding/xml"
	"strings"
	"testing"
)

// topLevelTags lists the worksheet's direct children, which is the order the
// OOXML schema constrains. Well-formedness alone says nothing about it: Excel
// reports "XML 错误" and repairs the file when a child is out of place.
func topLevelTags(t *testing.T, doc string) []string {
	t.Helper()
	dec := xml.NewDecoder(strings.NewReader(doc))
	var (
		out   []string
		depth int
	)
	for {
		tok, err := dec.Token()
		if err != nil {
			break
		}
		switch e := tok.(type) {
		case xml.StartElement:
			if depth == 1 {
				out = append(out, e.Name.Local)
			}
			depth++
		case xml.EndElement:
			depth--
		}
	}
	return out
}

func indexOf(tags []string, want string) int {
	for i, tag := range tags {
		if tag == want {
			return i
		}
	}
	return -1
}

const cfFragment = `<conditionalFormatting sqref="P58:AI58"><cfRule type="expression" dxfId="0" priority="1"><formula>ROUND(P58,0)&lt;P59</formula></cfRule></conditionalFormatting>`

// The failure this guards against: with a legacyDrawing (the comment boxes) and
// none of the later worksheet elements present, the block used to be appended
// at the very end of the sheet — after legacyDrawing, which Excel rejects.
func TestConditionalFormatsPrecedeLegacyDrawing(t *testing.T) {
	doc := `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">` +
		`<dimension ref="A1:AI58"/>` +
		`<sheetData><row r="58"><c r="A58"><v>1</v><extLst><ext uri="{x}"/></extLst></c></row></sheetData>` +
		`<mergeCells count="1"><mergeCell ref="A1:B1"/></mergeCells>` +
		`<legacyDrawing r:id="rId2"></legacyDrawing>` +
		`</worksheet>`

	merged, err := insertBeforeSheetEnd(doc, cfFragment)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	mustParseXML(t, "写入条件格式后的工作表", []byte(merged))

	tags := topLevelTags(t, merged)
	cf, ld, mc := indexOf(tags, "conditionalFormatting"), indexOf(tags, "legacyDrawing"), indexOf(tags, "mergeCells")
	if cf < 0 {
		t.Fatalf("conditionalFormatting missing from %v", tags)
	}
	if cf < mc {
		t.Errorf("conditionalFormatting must follow sheetData/mergeCells: %v", tags)
	}
	if ld >= 0 && cf > ld {
		t.Errorf("conditionalFormatting must precede legacyDrawing: %v", tags)
	}
	// The cell's own <extLst> must not be mistaken for a worksheet child.
	if starts := indexOf(tags, "extLst"); starts >= 0 {
		t.Errorf("conditionalFormatting was inserted inside sheetData: %v", tags)
	}
	if i := strings.Index(merged, cfFragment); i < 0 || i > strings.Index(merged, "<legacyDrawing") {
		t.Errorf("fragment ended up in the wrong place: %s", merged)
	}
}

// Later worksheet children — page setup, drawings, tables — all move the
// insertion point earlier, in whatever order they appear.
func TestConditionalFormatsPrecedeEveryLaterElement(t *testing.T) {
	for _, later := range []string{
		`<dataValidations count="1"><dataValidation sqref="A1"/></dataValidations>`,
		`<pageMargins left="0.7" right="0.7"/>`,
		`<pageSetup orientation="landscape"/>`,
		`<headerFooter><oddHeader>&amp;C</oddHeader></headerFooter>`,
		`<drawing r:id="rId3"/>`,
		`<tableParts count="1"><tablePart r:id="rId4"/></tableParts>`,
		`<legacyDrawing r:id="rId2"></legacyDrawing>`,
		`<extLst><ext uri="{y}"/></extLst>`,
	} {
		doc := `<?xml version="1.0"?><worksheet><sheetData><row r="58"/></sheetData>` +
			`<mergeCells count="1"><mergeCell ref="A1:B1"/></mergeCells>` + later + `</worksheet>`
		merged, err := insertBeforeSheetEnd(doc, cfFragment)
		if err != nil {
			t.Fatalf("insert before %s: %v", later, err)
		}
		mustParseXML(t, "工作表", []byte(merged))
		tags := topLevelTags(t, merged)
		cf, mc := indexOf(tags, "conditionalFormatting"), indexOf(tags, "mergeCells")
		if cf < 0 || cf < mc {
			t.Errorf("conditionalFormatting must follow mergeCells, got %v", tags)
		}
		if i := strings.Index(merged, cfFragment); i < 0 || i > strings.Index(merged, later) {
			t.Errorf("cf should precede %s, got %s", later, merged)
		}
	}
}

// A sheet with nothing after the data block still gets the block before the
// closing tag.
func TestConditionalFormatsOnMinimalSheet(t *testing.T) {
	doc := `<?xml version="1.0"?><worksheet><sheetData><row r="58"/></sheetData></worksheet>`
	merged, err := insertBeforeSheetEnd(doc, cfFragment)
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	mustParseXML(t, "工作表", []byte(merged))
	tags := topLevelTags(t, merged)
	if indexOf(tags, "conditionalFormatting") < 0 {
		t.Fatalf("sheets: %v", tags)
	}
}
