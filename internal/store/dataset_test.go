package store

import (
	"database/sql"
	"path/filepath"
	"testing"

	"project_clear/internal/mps"
)

var testIndexNames = []string{
	"P5", "P4", "P3", "P2", "P1", "MFG GROUP", "MFG CLASS CODE", "MFG DESCR",
	"ITEM", "US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC",
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "clear.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func testRow(item, value string) StagedRow {
	var idx [mps.IndexCols]string
	copy(idx[:], testIndexNames)
	idx[8] = item
	return StagedRow{
		FileName: "a.xlsm", FilePath: "/in/a.xlsm", SourceRow: 4,
		Index: idx, Weeks: []string{value},
	}
}

func testFile(name string) StagedFile {
	return StagedFile{Path: "/in/" + name, Name: name, Size: 1024, Status: "ok", RowsTotal: 1, RowsKept: 1, WeekCode: "2639"}
}

func makeGlobal(t *testing.T, st *Store, rows []StagedRow) *Dataset {
	t.Helper()
	d, err := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := st.AddDraftData(d.ID, []StagedFile{testFile("a.xlsm")}, rows, nil, nil, nil); err != nil {
		t.Fatalf("add: %v", err)
	}
	ready, err := st.FinishDraft(d.ID, true)
	if err != nil {
		t.Fatalf("finish: %v", err)
	}
	return ready
}

func makeLocal(t *testing.T, st *Store, rows []StagedRow) *Dataset {
	t.Helper()
	d, err := st.CreateLocalDataset("2639", "2026-09-21", []string{"2639"}, testIndexNames, "",
		[]StagedFile{testFile("local.xlsx")}, rows, nil)
	if err != nil {
		t.Fatalf("create local: %v", err)
	}
	return d
}

func TestDraftLifecycle(t *testing.T) {
	st := openTestStore(t)
	d, err := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if d.Status != DatasetDraft || d.Version != 0 || d.RowCount != 0 {
		t.Fatalf("new draft = %+v", d)
	}
	if err := st.AddDraftData(d.ID, []StagedFile{testFile("a.xlsm")},
		[]StagedRow{testRow("A", "100"), testRow("B", "200")}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if got, err := st.Draft(); err != nil || got == nil || got.ID != d.ID {
		t.Fatalf("draft lookup = %+v %v", got, err)
	}
	ready, err := st.FinishDraft(d.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if ready.Status != DatasetReady || ready.Version != 1 || ready.RowCount != 2 || ready.FileCount != 1 {
		t.Fatalf("finished = %+v", ready)
	}
	if got, _ := st.Draft(); got != nil {
		t.Fatalf("draft survived finish: %+v", got)
	}
	cur, err := st.Current("2639", DatasetGlobal)
	if err != nil || cur == nil || cur.ID != d.ID {
		t.Fatalf("current = %+v %v", cur, err)
	}
}

func TestDraftsAreDiscardedOnStartup(t *testing.T) {
	st := openTestStore(t)
	d, _ := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	_ = st.AddDraftData(d.ID, []StagedFile{testFile("a.xlsm")}, []StagedRow{testRow("A", "100")}, nil, nil, nil)
	n, err := st.DiscardAllDrafts()
	if err != nil || n != 1 {
		t.Fatalf("discard drafts = %d %v", n, err)
	}
	if cur, _ := st.Current("2639", DatasetGlobal); cur != nil {
		t.Fatalf("draft data survived: %+v", cur)
	}
	var rows int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM dataset_row`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("dataset_row = %d %v", rows, err)
	}
}

func TestAddingAfterFinishCopiesTheCompletedVersion(t *testing.T) {
	st := openTestStore(t)
	v1 := makeGlobal(t, st, []StagedRow{testRow("A", "100"), testRow("B", "200")})
	draft, err := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if draft.RowCount != 2 {
		t.Fatalf("copied draft rows = %d, want 2", draft.RowCount)
	}
	if err := st.AddDraftData(draft.ID, []StagedFile{testFile("b.xlsm")},
		[]StagedRow{testRow("C", "300")}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	v2, err := st.FinishDraft(draft.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if v2.Version != 2 || v2.RowCount != 3 {
		t.Fatalf("v2 = %+v", v2)
	}
	if old, _ := st.Dataset(v1.ID); old != nil {
		t.Fatalf("unreferenced v1 should have been replaced: %+v", old)
	}
	cur, _ := st.Current("2639", DatasetGlobal)
	if cur == nil || cur.ID != v2.ID {
		t.Fatalf("current = %+v", cur)
	}
}

func TestFinishRequiresConfirmationWhenFilesFailed(t *testing.T) {
	st := openTestStore(t)
	d, _ := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	bad := testFile("bad.xlsm")
	bad.Status = "failed"
	bad.Err = "damaged"
	if err := st.AddDraftData(d.ID, []StagedFile{testFile("a.xlsm"), bad},
		[]StagedRow{testRow("A", "100")}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FinishDraft(d.ID, false); err == nil {
		t.Fatal("finish without confirmation succeeded")
	}
	ready, err := st.FinishDraft(d.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	if ready.FailedCount != 1 || ready.FileCount != 1 || ready.RowCount != 1 {
		t.Fatalf("ready = %+v", ready)
	}
}

func TestIntegrationAndExpiry(t *testing.T) {
	st := openTestStore(t)
	g := makeGlobal(t, st, []StagedRow{testRow("A", "100"), testRow("B", "200")})
	l := makeLocal(t, st, []StagedRow{testRow("B", "250"), testRow("C", "300")})
	res, err := st.Integrate("2639", 61)
	if err != nil {
		t.Fatal(err)
	}
	if res.Merged.RowCount != 3 {
		t.Fatalf("merged rows = %d, want 3 (global wins by ITEM, local fills C)", res.Merged.RowCount)
	}
	in, _ := st.Integration("2639")
	if in == nil || in.GlobalID.Int64 != g.ID || in.LocalID.Int64 != l.ID || in.MergedID != res.Merged.ID {
		t.Fatalf("integration = %+v", in)
	}

	// A new Global version is current, but the integration still points at v1,
	// so the old version has to survive for history.
	draft, _ := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", g.ID)
	_ = st.AddDraftData(draft.ID, []StagedFile{testFile("b.xlsm")}, []StagedRow{testRow("D", "400")}, nil, nil, nil)
	v2, err := st.FinishDraft(draft.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	cur, _ := st.Current("2639", DatasetGlobal)
	if cur == nil || cur.ID != v2.ID {
		t.Fatalf("current = %+v", cur)
	}
	if old, _ := st.Dataset(g.ID); old == nil {
		t.Fatal("integrated v1 was deleted")
	}

	// Discarding this week's un-integrated import removes v2 and restores v1 as
	// current, leaving the integration itself untouched.
	n, err := st.DiscardWeekImports("2639")
	if err != nil || n != 1 {
		t.Fatalf("discard week = %d %v", n, err)
	}
	if cur, _ := st.Current("2639", DatasetGlobal); cur == nil || cur.ID != g.ID {
		t.Fatalf("current after discard = %+v", cur)
	}
	if in, _ := st.Integration("2639"); in == nil || in.MergedID != res.Merged.ID {
		t.Fatalf("integration after discard = %+v", in)
	}
}

func TestDatasetRowsKeepStylesCommentsAndCFColors(t *testing.T) {
	st := openTestStore(t)
	row := testRow("A", "100")
	row.StyleIDs = []int{7, 8}
	row.Comments = mps.CommentMap{16: {Text: "关注", Author: "me"}}
	row.CFColors = []string{"#FF0000"}
	d, err := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.AddDraftData(d.ID, []StagedFile{testFile("a.xlsm")}, []StagedRow{row}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	ready, err := st.FinishDraft(d.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := st.DatasetRows(ready.ID)
	if err != nil || len(rows) != 1 {
		t.Fatalf("rows = %+v %v", rows, err)
	}
	if len(rows[0].StyleIDs) != 2 || rows[0].Comments[16].Text != "关注" || rows[0].CFColors[0] != "#FF0000" {
		t.Fatalf("row = %+v", rows[0])
	}
	grid, err := st.QueryDataset(DatasetQuery{DatasetID: ready.ID, Page: 1, PageSize: 10})
	if err != nil || len(grid.Rows) != 1 {
		t.Fatalf("grid = %+v %v", grid, err)
	}
	if grid.Rows[0].WeekMeta[0].Color != "#FF0000" || grid.Rows[0].WeekMeta[0].Comment != "关注" {
		t.Fatalf("week meta = %+v", grid.Rows[0].WeekMeta[0])
	}
}

func TestQueryDatasetSearchSortFilterAndPages(t *testing.T) {
	st := openTestStore(t)
	makeGlobal(t, st, []StagedRow{
		testRow("ITEM-0001", "100"), testRow("ITEM-0002", "900"), testRow("OTHER", "500"),
	})
	cur, _ := st.Current("2639", DatasetGlobal)
	all, err := st.QueryDataset(DatasetQuery{DatasetID: cur.ID, Page: 1, PageSize: 10})
	if err != nil || all.Total != 3 {
		t.Fatalf("all = %+v %v", all, err)
	}
	found, _ := st.QueryDataset(DatasetQuery{DatasetID: cur.ID, Search: "ITEM", Page: 1, PageSize: 10})
	if found.Total != 2 {
		t.Fatalf("search total = %d, want 2", found.Total)
	}
	sorted, _ := st.QueryDataset(DatasetQuery{DatasetID: cur.ID, SortField: "c9", SortDesc: true, Page: 1, PageSize: 10})
	if sorted.Rows[0].Index[8] != "OTHER" {
		t.Fatalf("sort first = %q", sorted.Rows[0].Index[8])
	}
	filtered, _ := st.QueryDataset(DatasetQuery{DatasetID: cur.ID, Filters: map[string]string{"2639": ">300"}, Page: 1, PageSize: 10})
	if filtered.Total != 2 {
		t.Fatalf("filtered total = %d, want 2", filtered.Total)
	}
	page, _ := st.QueryDataset(DatasetQuery{DatasetID: cur.ID, Page: 2, PageSize: 2})
	if len(page.Rows) != 1 || page.Total != 3 {
		t.Fatalf("page 2 = %+v", page)
	}
}

func TestIntegrateRangeCutoffAndIntersectionWarning(t *testing.T) {
	st := openTestStore(t)
	g, err := st.BeginGlobalImport("2639", "2026-09-21", []string{"2639", "2640"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	grow := testRow("A", "100")
	grow.Weeks = []string{"100", "200"}
	if err := st.AddDraftData(g.ID, []StagedFile{testFile("a.xlsm")}, []StagedRow{grow}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := st.FinishDraft(g.ID, true); err != nil {
		t.Fatal(err)
	}
	lrow := testRow("B", "300")
	lrow.Weeks = []string{"300", "400"}
	l, err := st.CreateLocalDataset("2639", "2026-09-21", []string{"2639", "2640"}, testIndexNames, "",
		[]StagedFile{testFile("local.xlsx")}, []StagedRow{lrow}, nil)
	if err != nil {
		t.Fatal(err)
	}

	res, err := st.Integrate("2639", 1)
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestedWeeks != 1 || res.MergedWeeks != 1 || len(res.Warnings) != 0 {
		t.Fatalf("limit 1 = %+v", res)
	}
	cur, _ := st.Current("2639", DatasetMerged)
	if cur == nil || len(cur.WeekCodes) != 1 || cur.WeekCodes[0] != "2639" {
		t.Fatalf("merged weeks = %+v", cur)
	}

	res, err = st.Integrate("2639", 61)
	if err != nil {
		t.Fatal(err)
	}
	if res.RequestedWeeks != 61 || res.MergedWeeks != 2 || len(res.Warnings) == 0 {
		t.Fatalf("limit 61 = %+v", res)
	}
	cur, _ = st.Current("2639", DatasetMerged)
	if cur == nil || len(cur.WeekCodes) != 2 || cur.WeekCodes[1] != "2640" {
		t.Fatalf("merged weeks = %+v", cur)
	}
	_ = l
}

func TestDeleteDatasetRefusesOneAnIntegrationUses(t *testing.T) {
	st := openTestStore(t)
	g := makeGlobal(t, st, []StagedRow{testRow("A", "100")})
	l := makeLocal(t, st, []StagedRow{testRow("B", "200")})
	if _, err := st.Integrate("2639", 61); err != nil {
		t.Fatal(err)
	}
	if err := st.DeleteDataset(g.ID); err == nil {
		t.Fatal("deleted the integrated Global dataset")
	}
	if err := st.DeleteDataset(l.ID); err == nil {
		t.Fatal("deleted the integrated Local dataset")
	}
	if err := st.DeleteDataset(9999); err != nil && err != sql.ErrNoRows {
		t.Fatalf("delete missing: %v", err)
	}
}
