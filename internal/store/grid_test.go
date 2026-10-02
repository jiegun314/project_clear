package store

import (
	"testing"

	"project_clear/internal/mps"

	"github.com/xuri/excelize/v2"
)

// stagedRowWithStyle stages one row whose week cells carry a real fill colour
// and an annotation, which is what the grid has to show.
func stagedRowWithStyle(t *testing.T) StagingInput {
	t.Helper()
	dict := mps.NewStyleDict()
	filled, err := dict.InternStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"DCE6F2"}},
	})
	if err != nil {
		t.Fatalf("intern style: %v", err)
	}
	blank, err := dict.InternStyle(&excelize.Style{})
	if err != nil {
		t.Fatalf("intern style: %v", err)
	}

	in := stagedBatch(mps.CommentMap{
		mps.FirstWeekCol:     {Text: "PO 9578 & 9579 receipts", Author: "Alice"},
		mps.FirstWeekCol + 2: {Text: "delay", Author: "Bob"},
	})
	in.Dict = dict
	in.WeekCodes = []string{"2639", "2640", "2641"}
	in.Rows[0].Weeks = []string{"120", "0", "-48"}
	in.Rows[0].StyleIDs = make([]int, mps.IndexCols+len(in.WeekCodes))
	for i := range in.Rows[0].StyleIDs {
		in.Rows[0].StyleIDs[i] = blank
	}
	// Only P (the first week column) carries the blue fill.
	in.Rows[0].StyleIDs[mps.FirstWeekCol-1] = filled
	return in
}

func assertWeekCells(t *testing.T, rows []GridRow) {
	t.Helper()
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	row := rows[0]
	if len(row.Weeks) != 3 {
		t.Fatalf("weeks = %v, want 3 values", row.Weeks)
	}
	want := []string{"120", "0", "-48"}
	for i, w := range want {
		var got string
		if i < len(row.Weeks) {
			got = row.Weeks[i]
		}
		if got != w {
			t.Errorf("week %d = %q, want %q", i, got, w)
		}
	}
	if len(row.WeekMeta) != 3 {
		t.Fatalf("weekMeta = %v, want one entry per week column", row.WeekMeta)
	}
	if got := row.WeekMeta[0].Color; got != "#DCE6F2" {
		t.Errorf("P colour = %q, want #DCE6F2", got)
	}
	if got := row.WeekMeta[1].Color; got != "" {
		t.Errorf("Q colour = %q, want none", got)
	}
	if got := row.WeekMeta[0].Comment; got != "PO 9578 & 9579 receipts" {
		t.Errorf("P comment = %q", got)
	}
	if got := row.WeekMeta[0].Author; got != "Alice" {
		t.Errorf("P author = %q", got)
	}
	if got := row.WeekMeta[2].Comment; got != "delay" {
		t.Errorf("R comment = %q, want the third column's note", got)
	}
	if got := row.WeekMeta[1].Comment; got != "" {
		t.Errorf("Q comment = %q, want none", got)
	}
}

// The staging area keeps its week values in one JSON column; forgetting to
// decode it left every P-onward cell blank in the grid.
func TestStagingGridReturnsWeekValuesAndMeta(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveStaging(stagedRowWithStyle(t)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	gr, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 10, SortField: "seq"})
	if err != nil {
		t.Fatalf("query staging: %v", err)
	}
	assertWeekCells(t, gr.Rows)
}

// The committed week is read from real columns instead of JSON, and must
// produce exactly the same grid payload.
func TestCommittedGridReturnsWeekValuesAndMeta(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveStaging(stagedRowWithStyle(t)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	if _, err := st.Commit("2639"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	gr, err := st.QueryRows(Query{Source: "2639", Page: 1, PageSize: 10, SortField: "seq"})
	if err != nil {
		t.Fatalf("query committed: %v", err)
	}
	assertWeekCells(t, gr.Rows)
}
