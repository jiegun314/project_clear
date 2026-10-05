package store

import (
	"fmt"
	"sync"
	"testing"

	"project_clear/internal/mps"
)

// The data grid builds its sort keys as c1..c15 for the fixed A..O block and
// wk_<code> for a week column, and sends them to the backend untouched. These
// tests pin that contract from the store side: sortColumn has to map every one
// of those keys onto a real column, because a key it does not recognise is
// silently swapped for "seq" and the grid then shows the sort arrow over rows
// that are not sorted by that column.
func TestSortColumnMapsEveryIndexKeyTheGridSends(t *testing.T) {
	weeks := []string{"2639"}

	for i := 1; i <= mps.IndexCols; i++ {
		field := fmt.Sprintf("c%d", i)
		if got := sortColumn(field, weeks, false); got != field {
			t.Errorf("sortColumn(%q) = %q, want %q", field, got, field)
		}
	}
}

// A 0-based key (c0) or one past the index block (c16) is not a column, and the
// grid must never send them. They fall back to seq rather than reaching SQL.
func TestSortColumnRejectsKeysOutsideTheIndexBlock(t *testing.T) {
	weeks := []string{"2639"}

	for _, field := range []string{"c0", "c16", "c99", "c", "c-1"} {
		if got := sortColumn(field, weeks, false); got != "seq" {
			t.Errorf("sortColumn(%q) = %q, want %q", field, got, "seq")
		}
	}
}

func TestSortColumnMapsWeekKeyToItsColumn(t *testing.T) {
	weeks := []string{"2639", "2640"}

	// A committed week is a real column, quoted because it is interpolated.
	if got, want := sortColumn("wk_2639", weeks, false), `"wk_2639"`; got != want {
		t.Errorf("sortColumn(wk_2639, committed) = %q, want %q", got, want)
	}
	// Staging keeps the weeks in one JSON array, addressed by position.
	if got, want := sortColumn("wk_2640", weeks, true), `CAST(json_extract(weeks, '$[1]') AS REAL)`; got != want {
		t.Errorf("sortColumn(wk_2640, staging) = %q, want %q", got, want)
	}
}

// The key forms the grid used before the contract was unified must stay
// unrecognised: "w<code>" is not "wk_<code>", and silently accepting it would
// hide a regression back to a non-sorting week header.
func TestSortColumnDoesNotAcceptTheLegacyWeekKey(t *testing.T) {
	weeks := []string{"2639"}

	for _, field := range []string{"w2639", "2639", "wk_", "wk_9999"} {
		if got := sortColumn(field, weeks, false); got != "seq" {
			t.Errorf("sortColumn(%q) = %q, want %q", field, got, "seq")
		}
	}
}

func TestSortColumnMapsTheRowFields(t *testing.T) {
	for field, want := range map[string]string{
		"":          "seq",
		"seq":       "seq",
		"fileName":  "file_name",
		"file_name": "file_name",
		"srcRow":    "src_row",
		"src_row":   "src_row",
	} {
		if got := sortColumn(field, nil, false); got != want {
			t.Errorf("sortColumn(%q) = %q, want %q", field, got, want)
		}
	}
}

// Nothing from the request may reach SQL as a raw identifier, so anything that
// is not an exact known key is discarded.
func TestSortColumnDiscardsAnythingElse(t *testing.T) {
	weeks := []string{"2639"}

	for _, field := range []string{
		`c1; DROP TABLE archive`,
		`"c1"`,
		`c1 OR 1=1`,
		`wk_2639) OR 1=1--`,
		`wk_2639"`,
		`seq; DROP TABLE archive`,
		`file_name, seq`,
		" c1",
		"c1 ",
	} {
		if got := sortColumn(field, weeks, false); got != "seq" {
			t.Errorf("sortColumn(%q) = %q, want %q", field, got, "seq")
		}
	}
}

func TestWeekExprAddressesTheRightSlot(t *testing.T) {
	weeks := []string{"2639", "2640", "2641"}

	if got, want := weekExpr(false, "2639", weeks), `"wk_2639"`; got != want {
		t.Errorf("weekExpr(committed) = %q, want %q", got, want)
	}
	if got, want := weekExpr(true, "2639", weeks), `CAST(json_extract(weeks, '$[0]') AS REAL)`; got != want {
		t.Errorf("weekExpr(staging, first) = %q, want %q", got, want)
	}
	if got, want := weekExpr(true, "2641", weeks), `CAST(json_extract(weeks, '$[2]') AS REAL)`; got != want {
		t.Errorf("weekExpr(staging, third) = %q, want %q", got, want)
	}
	// A week the staging batch does not carry has no slot to read.
	if got, want := weekExpr(true, "9999", weeks), "NULL"; got != want {
		t.Errorf("weekExpr(staging, unknown) = %q, want %q", got, want)
	}
}

// The week order belongs to one query. It must not be read from package state:
// a concurrent import could replace it between the two calls, and the array
// would then be decoded with another batch's column order.
func TestWeekExprUsesTheCallersWeekOrder(t *testing.T) {
	first := []string{"2639", "2640"}
	second := []string{"2640", "2639"}

	if got, want := weekExpr(true, "2639", first), `CAST(json_extract(weeks, '$[0]') AS REAL)`; got != want {
		t.Errorf("weekExpr(2639, first order) = %q, want %q", got, want)
	}
	if got, want := weekExpr(true, "2639", second), `CAST(json_extract(weeks, '$[1]') AS REAL)`; got != want {
		t.Errorf("weekExpr(2639, second order) = %q, want %q", got, want)
	}
}

func TestSplitCondition(t *testing.T) {
	cases := []struct {
		in    string
		op    string
		value string
		ok    bool
	}{
		{">0", ">", "0", true},
		{">=12.5", ">=", "12.5", true},
		{"<=100", "<=", "100", true},
		{"<5", "<", "5", true},
		{"=0", "=", "0", true},
		{"!=3", "!=", "3", true},
		{"  > 7  ", ">", "7", true},
		{"empty", "empty", "", true},
		{"notempty", "notempty", "", true},
		{"", "", "", false},
		{"abc", "", "", false},
		{">", ">", "", true},
	}
	for _, c := range cases {
		op, value, ok := splitCondition(c.in)
		if op != c.op || value != c.value || ok != c.ok {
			t.Errorf("splitCondition(%q) = (%q, %q, %v), want (%q, %q, %v)",
				c.in, op, value, ok, c.op, c.value, c.ok)
		}
	}
}

// The week order used to live in a package-level variable that LoadStaging wrote
// and every query read. The application runs each bound method on its own
// goroutine, so a grid query could decode the staging JSON array with another
// batch's column order and show the wrong numbers under the right headers.
//
// The order now travels with the query. This test exists to be run under -race:
// against the old package-level variable it reports a data race between the
// LoadStaging write and the weekExpr read.
func TestConcurrentStagingQueriesAreStable(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveStaging(stagedRowWithStyle(t)); err != nil {
		t.Fatalf("save staging: %v", err)
	}

	const workers = 8
	var wg sync.WaitGroup
	failures := make(chan error, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < 8; i++ {
				if _, err := st.LoadStaging(); err != nil {
					failures <- err
					return
				}
				gr, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 10, SortField: "wk_2639"})
				if err != nil {
					failures <- err
					return
				}
				for _, r := range gr.Rows {
					for _, v := range r.Weeks {
						if v == "" {
							failures <- fmt.Errorf("a week cell came back empty: %v", r.Weeks)
							return
						}
					}
				}
			}
		}()
	}
	wg.Wait()
	close(failures)
	for err := range failures {
		t.Fatal(err)
	}
}
