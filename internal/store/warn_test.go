package store

import (
	"fmt"
	"strings"
	"testing"
)

// A stored payload that will not decode used to leave its field empty with no
// trace: the grid simply showed a cell without its colour or its note, and
// nothing anywhere said why.
func TestCorruptPayloadIsReportedAndOnlyThatFieldDegrades(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveStaging(stagedRowWithStyle(t)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	// Damage one stored field the way a truncated write would.
	if _, err := st.db.Exec(`UPDATE stg_row SET weeks='{not json'`); err != nil {
		t.Fatalf("damage payload: %v", err)
	}

	var warned []string
	st.SetWarn(func(format string, args ...any) {
		warned = append(warned, fmt.Sprintf(format, args...))
	})

	gr, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 5, SortField: "seq"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	// The damaged field degrades; the row itself is still served.
	if len(gr.Rows) != 1 {
		t.Fatalf("rows = %d, want the row to survive a damaged field", len(gr.Rows))
	}
	if len(warned) == 0 {
		t.Fatal("the damaged payload was not reported")
	}
	joined := strings.Join(warned, " | ")
	if !strings.Contains(joined, "周数据") {
		t.Errorf("the warning does not name the damaged field: %s", joined)
	}
	// The untouched fields still come through.
	if gr.Rows[0].WeekMeta[0].Color == "" {
		t.Errorf("an unrelated field was lost as well: %+v", gr.Rows[0].WeekMeta)
	}
}

// Without a sink the store must stay silent rather than panic: it is used
// without a logger in tests and in tools.
func TestStoreIsSilentWithoutAWarnSink(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveStaging(stagedRowWithStyle(t)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE stg_row SET weeks='{not json'`); err != nil {
		t.Fatalf("damage payload: %v", err)
	}
	if _, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 5, SortField: "seq"}); err != nil {
		t.Fatalf("query without a sink: %v", err)
	}
	// And a nil sink installed explicitly is the same as none.
	st.SetWarn(nil)
	if _, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 5, SortField: "seq"}); err != nil {
		t.Fatalf("query with a nil sink: %v", err)
	}
}
