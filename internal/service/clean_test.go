package service

import (
	"testing"
	"time"

	"project_clear/internal/mps"
	"project_clear/internal/store"
)

var testFields = []string{
	"AdjDmd", "SchedRcpts", "PlanOrders", "DeployIn", "DeployOut",
	"AdjInbound", "AdjOutbound", "ProxyDemand", "ProxySupply",
	"CalcOH", "CalcOH2", "SS", "WOS", "BackOrder $",
}

func cleanupRow(item, warehouse, file, field, value string) store.StagedRow {
	var idx [mps.IndexCols]string
	idx[8] = item
	idx[11] = warehouse
	idx[14] = field
	return store.StagedRow{
		FileName: file, FilePath: file, Index: idx,
		Weeks: []string{value},
	}
}

func fullGroup(item, warehouse, file, value string) []store.StagedRow {
	out := make([]store.StagedRow, 0, len(testFields))
	for _, field := range testFields {
		out = append(out, cleanupRow(item, warehouse, file, field, value))
	}
	return out
}

func countByField(rows []store.StagedRow) map[string]int {
	out := map[string]int{}
	for _, r := range rows {
		out[r.Index[14]]++
	}
	return out
}

func TestCleanImportRowsKeepsFirstGroupWithinOneFile(t *testing.T) {
	rows := append(fullGroup("ITEM-1", "WH_CNB", "a.xlsx", "first"),
		fullGroup("ITEM-1", "WH_CNB", "a.xlsx", "second")...)
	kept, stats := cleanImportRows(rows, nil)
	if len(kept) != 14 || stats.KeptRows != 14 || stats.DroppedRows != 14 {
		t.Fatalf("kept=%d stats=%+v", len(kept), stats)
	}
	if got := kept[0].Weeks[0]; got != "first" {
		t.Fatalf("kept value = %q, want first", got)
	}
	if got := countByField(kept); len(got) != 14 {
		t.Fatalf("fields = %+v", got)
	}
}

func TestCleanImportRowsKeepsGroupFromNewerFile(t *testing.T) {
	rows := append(fullGroup("ITEM-1", "WH_CNB", "old.xlsx", "old"),
		fullGroup("ITEM-1", "WH_CNB", "new.xlsx", "new")...)
	created := func(file string) time.Time {
		switch file {
		case "old.xlsx":
			return time.Unix(100, 0)
		case "new.xlsx":
			return time.Unix(200, 0)
		}
		return time.Time{}
	}
	kept, stats := cleanImportRows(rows, created)
	if len(kept) != 14 || stats.DuplicateGroups != 1 {
		t.Fatalf("kept=%d stats=%+v", len(kept), stats)
	}
	if got := kept[0].Weeks[0]; got != "new" {
		t.Fatalf("kept value = %q, want the newer file's group", got)
	}
}

func TestCleanImportRowsDropsIncompleteGroup(t *testing.T) {
	rows := make([]store.StagedRow, 0, 13)
	for _, field := range testFields[:13] {
		rows = append(rows, cleanupRow("ITEM-1", "WH_CNB", "a.xlsx", field, "x"))
	}
	kept, stats := cleanImportRows(rows, nil)
	if len(kept) != 0 || stats.DroppedRows != 13 || stats.IncompleteGroups != 1 {
		t.Fatalf("kept=%d stats=%+v", len(kept), stats)
	}
}

func TestCleanImportRowsDropsExtraPartialRows(t *testing.T) {
	rows := append(fullGroup("ITEM-1", "WH_CNB", "a.xlsx", "x"),
		cleanupRow("ITEM-1", "WH_CNB", "a.xlsx", "AdjDmd", "x"),
		cleanupRow("ITEM-1", "WH_CNB", "a.xlsx", "CalcOH", "x"),
	)
	kept, stats := cleanImportRows(rows, nil)
	if len(kept) != 14 || stats.DroppedRows != 2 {
		t.Fatalf("kept=%d stats=%+v", len(kept), stats)
	}
}

func TestCleanImportRowsHandlesTwoWarehousesIndependently(t *testing.T) {
	rows := append(fullGroup("ITEM-1", "WH_CNB", "a.xlsx", "cnb"),
		fullGroup("ITEM-1", "WH_CNS", "a.xlsx", "cns")...)
	kept, stats := cleanImportRows(rows, nil)
	if len(kept) != 28 || stats.DroppedRows != 0 {
		t.Fatalf("kept=%d stats=%+v", len(kept), stats)
	}
	byWarehouse := map[string]int{}
	for _, r := range kept {
		byWarehouse[r.Index[11]]++
	}
	if byWarehouse["WH_CNB"] != 14 || byWarehouse["WH_CNS"] != 14 {
		t.Fatalf("warehouses = %+v", byWarehouse)
	}
}

func TestCleanImportRowsPartialFileDoesNotMixWithCompleteFile(t *testing.T) {
	rows := append(fullGroup("ITEM-1", "WH_CNB", "complete.xlsx", "ok"),
		cleanupRow("ITEM-1", "WH_CNB", "partial.xlsx", "AdjDmd", "partial"))
	kept, stats := cleanImportRows(rows, nil)
	if len(kept) != 14 || stats.DroppedRows != 1 || stats.IncompleteGroups != 1 {
		t.Fatalf("kept=%d stats=%+v", len(kept), stats)
	}
	if kept[0].FilePath != "complete.xlsx" {
		t.Fatalf("kept file = %q", kept[0].FilePath)
	}
}

func TestCleanDraftLockedDeletesRejectedRows(t *testing.T) {
	s := newTestService(t)
	d, err := s.db.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	rows := append(fullGroup("ITEM-1", "WH_CNB", "a.xlsx", "first"),
		fullGroup("ITEM-1", "WH_CNB", "a.xlsx", "second")...)
	files := []store.StagedFile{{Path: "a.xlsx", Name: "a.xlsx", Status: "ok", RowsTotal: len(rows), RowsKept: len(rows), WeekCode: "2639"}}
	if err := s.db.AddDraftData(d.ID, files, rows, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	stats, err := s.cleanDraftLocked(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if stats.KeptRows != 14 || stats.DroppedRows != 14 {
		t.Fatalf("stats = %+v", stats)
	}
	got, err := s.db.DatasetRows(d.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 14 || got[0].Weeks[0] != "first" {
		t.Fatalf("rows = %d first = %q", len(got), got[0].Weeks[0])
	}
	if got[0].Seq != 0 || got[0].FilePath != "a.xlsx" {
		t.Fatalf("seq/path not loaded: seq=%d path=%q", got[0].Seq, got[0].FilePath)
	}
}
