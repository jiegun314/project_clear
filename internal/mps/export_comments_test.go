package mps

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// exportFixture exports a single merged row through the template engine and
// returns the written workbook together with its stats.
func exportFixture(t *testing.T, template string, comments CommentMap) (string, *ExportStats) {
	t.Helper()
	dest := filepath.Join(t.TempDir(), "CLEAR_2639.xlsm")
	weeks := make([]Week, 0, 3)
	for _, code := range []string{"2639", "2640", "2641"} {
		w, err := ParseWeekCode(code)
		if err != nil {
			t.Fatalf("week %s: %v", code, err)
		}
		weeks = append(weeks, w)
	}
	stats, err := Export(ExportInput{
		TemplatePath: template,
		DestPath:     dest,
		Weeks:        weeks,
		IndexNames:   []string{"P5", "P4"},
		Rows: []ExportRow{{
			Index:     [IndexCols]string{"P5_EP_BW", "P4"},
			Weeks:     []any{120.0, 0.0, 48.0},
			StyleIDs:  make([]int, IndexCols+len(weeks)),
			Comments:  comments,
			SourceRow: FirstDataRow,
		}},
		StyleByID: func(int) (json.RawMessage, bool) { return json.RawMessage(`{}`), true },
	})
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	return dest, stats
}

// The annotations of the merged rows must land in the exported workbook, with
// the text and the author intact.
func TestExportWritesMergedComments(t *testing.T) {
	template := workbookWithComments(t, t.TempDir(), commentFixture{})
	dest, stats := exportFixture(t, template, CommentMap{
		16: {Text: "PO 9578 & 9579 receipts", Author: "Alice"},
		18: {Text: "219 units\nDelay due to depalletizations", Author: "Bob"},
	})

	if stats.Comments != 2 {
		t.Errorf("stats.Comments = %d, want 2", stats.Comments)
	}

	f, err := excelize.OpenFile(dest)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer f.Close()
	list, err := f.GetComments(SheetName)
	if err != nil {
		t.Fatalf("get comments: %v", err)
	}
	got := map[string]excelize.Comment{}
	for _, c := range list {
		got[c.Cell] = c
	}
	if len(got) != 2 {
		t.Fatalf("exported comments = %v", got)
	}
	for _, want := range []struct{ cell, text, author string }{
		{"P4", "PO 9578 & 9579 receipts", "Alice"},
		{"R4", "219 units\nDelay due to depalletizations", "Bob"},
	} {
		c, ok := got[want.cell]
		if !ok {
			t.Errorf("%s has no comment", want.cell)
			continue
		}
		if c.Text != want.text {
			t.Errorf("%s text = %q, want %q", want.cell, c.Text, want.text)
		}
		if c.Author != want.author {
			t.Errorf("%s author = %q, want %q", want.cell, c.Author, want.author)
		}
	}
}

// A template's own threaded comments sit on its own rows, which the merged
// data replaces. Leaving them in place would annotate whichever row happens to
// land on that cell, so the export must strip the data-area ones.
func TestExportLeavesNoStaleThreadedComments(t *testing.T) {
	template := workbookWithComments(t, t.TempDir(), commentFixture{
		threaded: threadedFixture, persons: personsFixture,
	})
	raw, err := zipPart(template, threadedPart)
	if err != nil || !strings.Contains(string(raw), "R58") {
		t.Fatalf("fixture is missing its data-area threaded comment: %v", err)
	}

	dest, _ := exportFixture(t, template, CommentMap{
		18: {Text: "written by CLEAR", Author: "Alice"},
	})

	if leftover, err := zipPart(dest, threadedPart); err == nil {
		if strings.Contains(string(leftover), "<threadedComment") {
			t.Errorf("stale threaded comments survived the export: %s", leftover)
		}
	}
	if HasPart(dest, threadedPart) {
		sheetPart, err := SheetPartPath(dest, SheetName)
		if err != nil {
			t.Fatal(err)
		}
		rels, err := zipPart(dest, filepath.ToSlash(filepath.Dir(sheetPart))+"/_rels/"+filepath.Base(sheetPart)+".rels")
		if err == nil && strings.Contains(string(rels), relThreadedComments) {
			t.Errorf("export has a relationship pointing at a part it does not contain: %s", rels)
		}
	}

	f, err := excelize.OpenFile(dest)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer f.Close()
	list, err := f.GetComments(SheetName)
	if err != nil {
		t.Fatalf("get comments: %v", err)
	}
	if len(list) != 1 || list[0].Cell != "R4" || list[0].Text != "written by CLEAR" {
		t.Errorf("exported comments = %+v", list)
	}
}

// A template without any annotation parts must still accept new ones.
func TestExportAddsCommentsToTemplateWithoutAny(t *testing.T) {
	template := workbookWithComments(t, t.TempDir(), commentFixture{})
	if HasPart(template, legacyPart) {
		t.Fatalf("fixture should start without a comments part")
	}
	dest, stats := exportFixture(t, template, CommentMap{
		16: {Text: "no author recorded"},
	})
	if stats.Comments != 1 {
		t.Fatalf("stats.Comments = %d", stats.Comments)
	}
	f, err := excelize.OpenFile(dest)
	if err != nil {
		t.Fatalf("open export: %v", err)
	}
	defer f.Close()
	list, err := f.GetComments(SheetName)
	if err != nil {
		t.Fatalf("get comments: %v", err)
	}
	if len(list) != 1 || list[0].Text != "no author recorded" || list[0].Author != "CLEAR" {
		t.Fatalf("comments = %+v", list)
	}
}
