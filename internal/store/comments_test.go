package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	"project_clear/internal/mps"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "clear.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func indexNames() []string {
	names := make([]string, mps.IndexCols)
	for i := range names {
		names[i] = "col"
	}
	names[mps.LOCCol-1] = "LOC"
	return names
}

func stagedBatch(comments mps.CommentMap) StagingInput {
	var idx [mps.IndexCols]string
	idx[0] = "P5_EP_BW"
	idx[mps.LOCCol-1] = "WH_CNB"
	return StagingInput{
		Action:     "import",
		WeekCode:   "2639",
		WeekStart:  "2026-09-21",
		WeekCodes:  []string{"2639"},
		IndexNames: indexNames(),
		Files: []StagedFile{{
			Path: "/src/a.xlsm", Name: "a.xlsm", Size: 100, Status: "ok",
			RowsTotal: 1, RowsKept: 1, WeekCode: "2639",
		}},
		Rows: []StagedRow{{
			FileName: "a.xlsm", SourceRow: mps.FirstDataRow,
			Index:    idx,
			Weeks:    []string{"120"},
			StyleIDs: []int{0},
			Comments: comments,
		}},
		Dict: mps.NewStyleDict(),
	}
}

// The annotations must survive the staging -> commit -> export read path; this
// is exactly the hop that used to drop them.
func TestCommitPreservesComments(t *testing.T) {
	st := openTestStore(t)
	want := mps.CommentMap{
		16: {Text: "PO 9578 & 9579 receipts", Author: "Alice"},
		18: {Text: "219 units\nDelay due to depalletizations", Author: "Bob"},
	}
	if _, err := st.SaveStaging(stagedBatch(want)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	if _, err := st.Commit("2639"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	if _, err := st.LoadStaging(); err != nil {
		t.Fatalf("load staging: %v", err)
	}
	_, rows, err := st.FetchExportRows("2639")
	if err != nil {
		t.Fatalf("fetch export rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1", len(rows))
	}
	got := rows[0].Comments
	if len(got) != len(want) {
		t.Fatalf("comments = %+v, want %+v", got, want)
	}
	for col, w := range want {
		if got[col] != w {
			t.Errorf("column %d = %+v, want %+v", col, got[col], w)
		}
	}
}

// Rows without annotations must stay empty rather than round-tripping through
// an empty JSON object.
func TestCommitWithoutComments(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveStaging(stagedBatch(nil)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	if _, err := st.Commit("2639"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	_, rows, err := st.FetchExportRows("2639")
	if err != nil {
		t.Fatalf("fetch export rows: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d", len(rows))
	}
	if len(rows[0].Comments) != 0 {
		t.Errorf("comments = %+v, want none", rows[0].Comments)
	}
}

// A database written before comments reached the weekly tables has no such
// column. Opening it must add the column so an older week can still be
// exported instead of erroring out.
func TestOpenMigratesLegacyTablesWithoutComments(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := legacy.Exec(schemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}
	// The 1.0 weekly table: everything today's export needs except comments.
	if _, err := legacy.Exec(`CREATE TABLE "data_2639" (
		seq INTEGER PRIMARY KEY, c1 TEXT, c2 TEXT, c3 TEXT, c4 TEXT, c5 TEXT,
		c6 TEXT, c7 TEXT, c8 TEXT, c9 TEXT, c10 TEXT, c11 TEXT, c12 TEXT,
		c13 TEXT, c14 TEXT, c15 TEXT, "wk_2639" REAL, cf_id INTEGER,
		file_name TEXT, src_row INTEGER, style_ids TEXT)`); err != nil {
		t.Fatalf("legacy table: %v", err)
	}
	if _, err := legacy.Exec(
		`INSERT INTO "data_2639"(seq, c1, c12, "wk_2639", file_name, src_row, style_ids)
		 VALUES(0, 'P5_EP_BW', 'WH_CNB', 120, 'a.xlsm', 58, '[0]')`); err != nil {
		t.Fatalf("legacy row: %v", err)
	}
	weekCodes, _ := json.Marshal([]string{"2639"})
	names, _ := json.Marshal(indexNames())
	if _, err := legacy.Exec(
		`INSERT INTO archive(week_code, week_start, week_codes, index_names, row_count,
		                     file_count, committed_at, table_name)
		 VALUES('2639','2026-09-21',?,'2039-01-01',1,1,'2026-09-21 10:00:00','data_2639')`,
		string(weekCodes)); err != nil {
		t.Fatalf("archive row: %v", err)
	}
	_ = names
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer st.Close()

	has, err := hasColumn(st.db, "data_2639", "comments")
	if err != nil || !has {
		t.Fatalf("comments column missing after migration (has=%v err=%v)", has, err)
	}
	_, rows, err := st.FetchExportRows("2639")
	if err != nil {
		t.Fatalf("fetch export rows: %v", err)
	}
	if len(rows) != 1 || len(rows[0].Comments) != 0 {
		t.Fatalf("rows = %+v", rows)
	}
}

// v1.5.4 给暂存表加了 cf_colors（条件格式求值结果）。老数据库是用旧的
// schema 建的，CREATE TABLE IF NOT EXISTS 不会补列，导入时就会报
// "table stg_row has no column named cf_colors" —— 这里锁住这次修复。
func TestOpenMigratesLegacyStagingWithoutCFColors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-staging.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	// 先建库（含 stg_row），再把 cf_colors 删掉，模拟 v1.5.3 及以前的库。
	if _, err := legacy.Exec(schemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}
	if _, err := legacy.Exec(`ALTER TABLE stg_row DROP COLUMN cf_colors`); err != nil {
		t.Fatalf("drop column: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open migrated store: %v", err)
	}
	defer st.Close()
	has, err := hasColumn(st.db, "stg_row", "cf_colors")
	if err != nil || !has {
		t.Fatalf("cf_colors 未补到 stg_row (has=%v err=%v)", has, err)
	}
	// 老库升级后必须能正常写入暂存数据。
	in := stagedBatch(nil)
	in.Rows[0].CFColors = []string{"#FFFF00"}
	if _, err := st.SaveStaging(in); err != nil {
		t.Fatalf("save staging after migration: %v", err)
	}
	grid, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 5, SortField: "seq"})
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	if len(grid.Rows) != 1 || grid.Rows[0].WeekMeta[0].Color != "#FFFF00" {
		t.Fatalf("条件格式颜色未随导入写入: %+v", grid.Rows)
	}
}
