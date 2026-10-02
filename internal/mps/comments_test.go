package mps

import (
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

const (
	legacyPart   = "xl/comments1.xml"
	threadedPart = "xl/threadedComments/threadedComment1.xml"
	personsPart  = "xl/persons/person.xml"
)

// commentFixture is the raw XML injected into a throwaway workbook. An empty
// string means "the workbook has no such part", which is the normal case for
// files with no annotations at all.
type commentFixture struct {
	legacy   string
	threaded string
	persons  string
}

// workbookWithComments builds a minimal MPS workbook carrying the supplied
// annotation parts. excelize cannot author threaded comments, so the parts are
// stitched in at the zip level exactly the way Excel stores them.
func workbookWithComments(t *testing.T, dir string, fx commentFixture) string {
	t.Helper()

	f := excelize.NewFile()
	if err := f.SetSheetName("Sheet1", SheetName); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for ref, v := range map[string]string{
		// The decoration the export is supposed to drop.
		"A1":  "SUMMARY (click on left \"+\" to open or hide the Summary section)",
		"A3":  "decorative block",
		"A55": "P5", "L55": "LOC", "O55": "SchedRcpts",
		"P55": "2639", "P56": "46286",
		"A58": "P5_EP_BW", "L58": "WH_CNB", "P58": "120",
	} {
		if err := f.SetCellStr(SheetName, ref, v); err != nil {
			t.Fatalf("set %s: %v", ref, err)
		}
	}
	book := filepath.Join(dir, "fixture.xlsm")
	if err := f.SaveAs(book); err != nil {
		t.Fatalf("save fixture: %v", err)
	}
	f.Close()

	sheetPart, err := SheetPartPath(book, SheetName)
	if err != nil {
		t.Fatalf("sheet part: %v", err)
	}
	relsPath := path.Join(path.Dir(sheetPart), "_rels", path.Base(sheetPart)+".rels")

	repl := map[string][]byte{}
	var rels strings.Builder
	rels.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\n")
	rels.WriteString(`<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	nextID := 0
	next := func(typ, target string) {
		nextID++
		rels.WriteString(`<Relationship Id="rId` + strconv.Itoa(nextID) + `" Type="` + typ + `" Target="` + target + `"/>`)
	}
	var overrides []string
	if fx.legacy != "" {
		next(relComments, "../"+path.Base(legacyPart))
		repl[legacyPart] = []byte(fx.legacy)
		overrides = append(overrides,
			`<Override PartName="/`+legacyPart+`" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.comments+xml"/>`)
	}
	if fx.threaded != "" {
		next(relThreadedComments, "../threadedComments/"+path.Base(threadedPart))
		repl[threadedPart] = []byte(fx.threaded)
		overrides = append(overrides,
			`<Override PartName="/`+threadedPart+`" ContentType="application/vnd.ms-excel.threadedcomments+xml"/>`)
	}
	if fx.persons != "" {
		repl[personsPart] = []byte(fx.persons)
		overrides = append(overrides,
			`<Override PartName="/`+personsPart+`" ContentType="application/vnd.ms-excel.person+xml"/>`)
	}
	rels.WriteString(`</Relationships>`)
	repl[relsPath] = []byte(rels.String())

	if len(overrides) > 0 {
		types, err := zipPart(book, "[Content_Types].xml")
		if err != nil {
			t.Fatalf("content types: %v", err)
		}
		patched := strings.Replace(string(types), "</Types>", strings.Join(overrides, "")+"</Types>", 1)
		repl["[Content_Types].xml"] = []byte(patched)
	}
	if err := ReplaceParts(book, repl); err != nil {
		t.Fatalf("inject comment parts: %v", err)
	}
	return book
}

const placeholderLegacy = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<comments xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">
  <authors><author>tc={8D43BE72}</author><author>Alice</author></authors>
  <commentList>
    <comment ref="P58" authorId="1" shapeId="0"><text><t xml:space="preserve">legacy on P58</t></text></comment>
    <comment ref="R58" authorId="0" shapeId="0"><text><t xml:space="preserve">[线程批注]&#10;&#10;你的Excel版本可读取此线程批注; 但如果在更新版本的Excel中打开文件，则对批注所作的任何改动都将被删除。&#10;&#10;注释:&#10;    PO 9578 &amp; 9579 receipts&#10;</t></text></comment>
  </commentList>
</comments>`

const threadedFixture = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ThreadedComments xmlns="http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments">
  <threadedComment ref="R58" dT="2026-09-15T18:51:24.12Z" personId="{P1}" id="{C1}"><text xml:space="preserve">PO 9578 &amp; 9579 receipts</text></threadedComment>
  <threadedComment ref="R58" dT="2026-09-15T19:02:00.00Z" personId="{P2}" id="{C2}" parentId="{C1}"><text xml:space="preserve">received</text></threadedComment>
</ThreadedComments>`

const personsFixture = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<persons xmlns="http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments">
  <person displayName="Alice" id="{P1}" userId="alice" providerId="AD"/>
  <person displayName="Bob" id="{P2}" userId="bob" providerId="AD"/>
</persons>`

// A modern comment keeps a stub in the legacy part; the real words live in the
// threaded part and must win, author included.
func TestReadCommentsPrefersThreadedOverLegacyPlaceholder(t *testing.T) {
	book := workbookWithComments(t, t.TempDir(), commentFixture{
		legacy: placeholderLegacy, threaded: threadedFixture, persons: personsFixture,
	})

	got, err := readComments(book, SheetName)
	if err != nil {
		t.Fatalf("readComments: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("rows with comments = %d, want 1 (%v)", len(got), got)
	}
	row := got[FirstDataRow]
	if len(row) != 2 {
		t.Fatalf("columns with comments = %d, want 2 (%v)", len(row), row)
	}
	// Column R (18) keeps only the threaded text, not the "[线程批注]" stub.
	if c := row[18]; c.Text != "PO 9578 & 9579 receipts\n\nBob: received" {
		t.Errorf("R58 text = %q", c.Text)
	}
	if c := row[18]; c.Author != "Alice" {
		t.Errorf("R58 author = %q, want Alice", c.Author)
	}
	// Column P (16) has no threaded counterpart, so the legacy note stands and
	// the "tc={guid}" author prefix is stripped.
	if c := row[16]; c.Text != "legacy on P58" {
		t.Errorf("P58 text = %q", c.Text)
	}
	if c := row[16]; c.Author != "Alice" {
		t.Errorf("P58 author = %q, want Alice", c.Author)
	}
}

// A workbook that only has classic notes must still read them. When Excel
// mirrors a modern comment into the legacy part it copies the author's words
// after a "注释:" marker, so that text is recovered even without the threaded
// part; the boilerplate around it is not.
func TestReadCommentsWithoutThreadedPart(t *testing.T) {
	book := workbookWithComments(t, t.TempDir(), commentFixture{legacy: placeholderLegacy})
	got, err := readComments(book, SheetName)
	if err != nil {
		t.Fatalf("readComments: %v", err)
	}
	row := got[FirstDataRow]
	if len(row) != 2 {
		t.Fatalf("legacy-only read = %v", row)
	}
	if row[16].Text != "legacy on P58" {
		t.Errorf("P58 = %q", row[16].Text)
	}
	if row[18].Text != "PO 9578 & 9579 receipts" {
		t.Errorf("the stub's body should be recovered, got %q", row[18].Text)
	}
	if strings.Contains(row[18].Text, "[线程批注]") || strings.Contains(row[18].Text, "go.microsoft.com") {
		t.Errorf("boilerplate leaked into the comment: %q", row[18].Text)
	}
}

// Notes above the data block belong to the report header and are copied with
// the template; only stale notes inside the data area are stripped.
func TestStripThreadedCommentsKeepsHeaderAnchors(t *testing.T) {
	book := workbookWithComments(t, t.TempDir(), commentFixture{
		threaded: `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<ThreadedComments xmlns="http://schemas.microsoft.com/office/spreadsheetml/2018/threadedcomments">
  <threadedComment ref="P55" personId="{P1}" id="{C1}"><text>report title</text></threadedComment>
  <threadedComment ref="R58" personId="{P1}" id="{C2}"><text>stale body note</text></threadedComment>
</ThreadedComments>`,
		persons: personsFixture,
	})

	if err := StripThreadedComments(book, SheetName, FirstDataRow); err != nil {
		t.Fatalf("StripThreadedComments: %v", err)
	}
	raw, err := zipPart(book, threadedPart)
	if err != nil {
		t.Fatalf("threaded part was removed even though P55 survives: %v", err)
	}
	if strings.Contains(string(raw), "R58") {
		t.Errorf("data-area threaded comment survived: %s", raw)
	}
	if !strings.Contains(string(raw), "P55") {
		t.Errorf("header-area threaded comment was dropped: %s", raw)
	}
}

// With every threaded comment inside the data area, the part and the
// relationship pointing at it must both go, or Excel sees a dangling rel.
func TestStripThreadedCommentsDropsEmptyPartAndRel(t *testing.T) {
	book := workbookWithComments(t, t.TempDir(), commentFixture{
		threaded: threadedFixture, persons: personsFixture,
	})
	if err := StripThreadedComments(book, SheetName, FirstDataRow); err != nil {
		t.Fatalf("StripThreadedComments: %v", err)
	}
	if HasPart(book, threadedPart) {
		t.Errorf("threaded part should be deleted when it is left empty")
	}
	sheetPart, err := SheetPartPath(book, SheetName)
	if err != nil {
		t.Fatal(err)
	}
	rels, err := zipPart(book, path.Join(path.Dir(sheetPart), "_rels", path.Base(sheetPart)+".rels"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rels), relThreadedComments) {
		t.Errorf("relationship to the deleted part remains: %s", rels)
	}
	if !HasPart(book, sheetPart) {
		t.Errorf("strip must not damage the sheet itself")
	}
}

func TestCleanLegacyTextStripsPlaceholder(t *testing.T) {
	withText := "[线程批注]\n\n你的Excel版本可读取此线程批注; ...\n\n注释:\n    PO 9578 & 9579 receipts\n"
	if got := cleanLegacyText(withText); got != "PO 9578 & 9579 receipts" {
		t.Errorf("cleanLegacyText = %q", got)
	}
	if got := cleanLegacyText("[线程批注]\n\nno body here"); got != "" {
		t.Errorf("a stub without a body should vanish, got %q", got)
	}
	if got := cleanLegacyText("  a plain note  "); got != "a plain note" {
		t.Errorf("plain notes must be untouched, got %q", got)
	}
}

// The staging table was writing plain strings before comments were carried
// through storage; an existing database must still load.
func TestCommentMapAcceptsBothStoredShapes(t *testing.T) {
	var m CommentMap
	if err := m.UnmarshalJSON([]byte(`{"16":"old shape","18":{"text":"new","author":"Alice"}}`)); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if m[16].Text != "old shape" || m[16].Author != "" {
		t.Errorf("16 = %+v", m[16])
	}
	if m[18].Text != "new" || m[18].Author != "Alice" {
		t.Errorf("18 = %+v", m[18])
	}
	var empty CommentMap
	if err := empty.UnmarshalJSON([]byte("null")); err != nil || empty != nil {
		t.Errorf("null should decode to an empty map, got %v %v", empty, err)
	}
}
