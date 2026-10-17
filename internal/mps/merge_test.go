package mps

import (
	"strings"
	"testing"
)

// wk builds the week list a header carries.
func wk(t *testing.T, codes ...string) []Week {
	t.Helper()
	out := make([]Week, len(codes))
	for i, c := range codes {
		w, err := ParseWeekCode(c)
		if err != nil {
			t.Fatalf("ParseWeekCode(%q): %v", c, err)
		}
		out[i] = w
	}
	return out
}

// row builds one global-layout data row: index A..O, then the week values.
func row(item string, weeks ...string) *DataRow {
	return &DataRow{
		Index: [IndexCols]string{"P5", "P4", "P3", "P2", "P1", "BUY", "1", "ACME", item,
			"US", "EU", "WH_CNB", "0", "1", "AdjDmd"},
		Weeks: weeks,
	}
}

func TestMergeLocalKeepsGlobalAndFillsFromLocal(t *testing.T) {
	global := Header{IndexNames: IndexNamesOf(t), Weeks: wk(t, "2639", "2640")}
	globalRows := []*DataRow{
		row("ITEM-A", "1", "2"),
		row("ITEM-B", "3", "4"),
	}
	local := &LocalSheet{
		Header: Header{Weeks: wk(t, "2639", "2640")},
		Rows: []*DataRow{
			row("ITEM-A", "999", "999"), // must be replaced by the global rows
			row("ITEM-C", "5", "6"),     // only locally known: kept
		},
	}

	got, err := MergeLocal(global, globalRows, local)
	if err != nil {
		t.Fatalf("MergeLocal: %v", err)
	}
	if got.FromGlobal != 2 || got.FromLocal != 1 {
		t.Fatalf("from global/local = %d/%d, want 2/1", got.FromGlobal, got.FromLocal)
	}
	if len(got.Rows) != 3 {
		t.Fatalf("rows = %d, want 3", len(got.Rows))
	}
	items := []string{itemOf(got.Rows[0]), itemOf(got.Rows[1]), itemOf(got.Rows[2])}
	if strings.Join(items, ",") != "ITEM-A,ITEM-B,ITEM-C" {
		t.Fatalf("order = %v, want the global rows first then the local-only ones", items)
	}
	// ITEM-A must carry the global values, not the local ones.
	if got.Rows[0].Weeks[0] != "1" {
		t.Fatalf("ITEM-A week 2639 = %q, want the global value %q", got.Rows[0].Weeks[0], "1")
	}
	if got.Rows[2].Weeks[0] != "5" {
		t.Fatalf("ITEM-C week 2639 = %q, want the local value %q", got.Rows[2].Weeks[0], "5")
	}
	if len(got.Replaced) != 1 || got.Replaced[0] != "ITEM-A" {
		t.Fatalf("replaced = %v, want [ITEM-A]", got.Replaced)
	}
}

// Two rows that are identical in every cell carry no extra information, which is
// what the business workbook did with the five duplicated items.
func TestMergeLocalCollapsesIdenticalRows(t *testing.T) {
	global := Header{IndexNames: IndexNamesOf(t), Weeks: wk(t, "2639")}
	got, err := MergeLocal(global, []*DataRow{
		row("ITEM-A", "1"),
		row("ITEM-A", "1"), // identical
		row("ITEM-A", "2"), // same item, different values: kept
	}, &LocalSheet{Header: Header{Weeks: wk(t, "2639")}, Rows: []*DataRow{row("ITEM-Z", "9")}})
	if err != nil {
		t.Fatalf("MergeLocal: %v", err)
	}
	if got.Duplicates != 1 || got.FromGlobal != 2 {
		t.Fatalf("duplicates = %d, fromGlobal = %d; want 1 and 2", got.Duplicates, got.FromGlobal)
	}
}

// Values travel by week code, so a local file whose columns start at a different
// week still lands on the right ones.
func TestMergeLocalAlignsWeeksByCode(t *testing.T) {
	global := Header{IndexNames: IndexNamesOf(t), Weeks: wk(t, "2639", "2640", "2641")}
	local := &LocalSheet{
		Header: Header{Weeks: wk(t, "2640", "2641", "2642")},
		Rows:   []*DataRow{row("ITEM-L", "L0", "L1", "L2")},
	}
	got, err := MergeLocal(global, []*DataRow{row("ITEM-G", "G0", "G1", "G2")}, local)
	if err != nil {
		t.Fatalf("MergeLocal: %v", err)
	}
	// The shared weeks are 2640 and 2641, in the global order.
	if len(got.Header.Weeks) != 2 || got.Header.Weeks[0].Code != "2640" || got.Header.Weeks[1].Code != "2641" {
		t.Fatalf("weeks = %+v, want 2640 and 2641", got.Header.Weeks)
	}
	if got.Rows[0].Weeks[0] != "G1" || got.Rows[0].Weeks[1] != "G2" {
		t.Fatalf("global row weeks = %v, want the 2640 and 2641 values", got.Rows[0].Weeks)
	}
	if got.Rows[1].Weeks[0] != "L0" || got.Rows[1].Weeks[1] != "L1" {
		t.Fatalf("local row weeks = %v, want its own 2640 and 2641 values", got.Rows[1].Weeks)
	}
	// The week block narrowed, so column-addressed visuals could not be trusted.
	if got.VisualsDropped != 1 {
		t.Fatalf("visualsDropped = %d, want 1 (the global row)", got.VisualsDropped)
	}
}

func TestMergeLocalRefusesWhenNoWeekIsShared(t *testing.T) {
	global := Header{IndexNames: IndexNamesOf(t), Weeks: wk(t, "2639")}
	local := &LocalSheet{Header: Header{Weeks: wk(t, "2640")}, Rows: []*DataRow{row("ITEM-L", "1")}}
	_, err := MergeLocal(global, []*DataRow{row("ITEM-G", "1")}, local)
	if err == nil {
		t.Fatal("no shared week must be refused")
	}
	if !strings.Contains(err.Error(), "没有共同的周") {
		t.Errorf("error should say the weeks do not overlap, got: %v", err)
	}
}

func TestMergeLocalRequiresALocalSheet(t *testing.T) {
	global := Header{IndexNames: IndexNamesOf(t), Weeks: wk(t, "2639")}
	if _, err := MergeLocal(global, nil, nil); err == nil {
		t.Fatal("merging without a local file must be refused")
	}
}

// IndexNamesOf is the A..O header list, filled with the names the reader uses.
func IndexNamesOf(t *testing.T) []string {
	t.Helper()
	out := make([]string, 0, IndexCols)
	for i := 0; i < IndexCols; i++ {
		out = append(out, string(rune('A'+i)))
	}
	return out
}
