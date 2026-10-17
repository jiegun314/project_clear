package store

import (
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// buildV4 writes a minimal database in the previous release's shape: an archive
// registry plus one physical table per week/dataset. Opening it must copy those
// rows into the dataset model and drop the old tables.
func buildV4(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	weekCodes, _ := json.Marshal([]string{"2639"})
	indexNames, _ := json.Marshal(testIndexNames)
	stmts := []string{
		`CREATE TABLE app_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`CREATE TABLE archive (
			week_code TEXT NOT NULL, dataset TEXT NOT NULL DEFAULT 'merged', week_start TEXT NOT NULL,
			week_codes TEXT NOT NULL, index_names TEXT NOT NULL, row_count INTEGER NOT NULL,
			file_count INTEGER NOT NULL, batch_id INTEGER, committed_at TEXT NOT NULL,
			template_id INTEGER, param_snapshot TEXT, table_name TEXT NOT NULL,
			PRIMARY KEY (week_code, dataset))`,
		`CREATE TABLE batch (id INTEGER PRIMARY KEY, status TEXT)`,
		`CREATE TABLE stg_row (id INTEGER PRIMARY KEY, batch_id INTEGER)`,
		`CREATE TABLE batch_file (id INTEGER PRIMARY KEY, batch_id INTEGER)`,
	}
	for _, s := range stmts {
		if _, err := db.Exec(s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
	if _, err := db.Exec(`INSERT INTO app_meta VALUES('schema_version','4'),('files_cleared_upto','2638')`); err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct {
		table string
		kind  string
		rows  int
	}{
		{"data_2639", "merged", 2},
		{"data_2639_global", "global", 2},
		{"data_2639_local", "local", 2},
	} {
		create := `CREATE TABLE ` + item.table + ` (seq INTEGER PRIMARY KEY`
		for i := 1; i <= 15; i++ {
			create += `, c` + itoa(i) + ` TEXT`
		}
		create += `, wk_2639 REAL, cf_id INTEGER, file_name TEXT, src_row INTEGER, style_ids TEXT, comments TEXT, cf_colors TEXT)`
		if _, err := db.Exec(create); err != nil {
			t.Fatal(err)
		}
		for seq := 1; seq <= item.rows; seq++ {
			if _, err := db.Exec(`INSERT INTO `+item.table+`(seq, c9, wk_2639, file_name, src_row, style_ids, comments, cf_colors)
				VALUES(?,?,?,?,?,?,?,?)`, seq, item.kind+"-"+itoa(seq), 100+seq,
				"a.xlsm", 3+seq, "[]", `{"16":{"text":"note"}}`, `["#FF0000"]`); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := db.Exec(`INSERT INTO archive(week_code,dataset,week_start,week_codes,index_names,
			row_count,file_count,committed_at,param_snapshot,table_name)
			VALUES('2639',?,'2026-09-21',?,?,?,?,'2026-10-10 21:06:55','{}',?)`,
			item.kind, string(weekCodes), string(indexNames), item.rows, 13, item.table); err != nil {
			t.Fatal(err)
		}
	}
}

func itoa(n int) string {
	if n < 10 {
		return string(rune('0' + n))
	}
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}

func TestMigrationFromV4CreatesDatasetsAndIntegration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.db")
	buildV4(t, path)

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open v4: %v", err)
	}
	defer st.Close()

	in, err := st.Integration("2639")
	if err != nil || in == nil {
		t.Fatalf("integration = %+v %v", in, err)
	}
	if !in.GlobalID.Valid || !in.LocalID.Valid {
		t.Fatalf("integration refs = %+v", in)
	}
	g, _ := st.Dataset(in.GlobalID.Int64)
	l, _ := st.Dataset(in.LocalID.Int64)
	m, _ := st.Dataset(in.MergedID)
	if g == nil || l == nil || m == nil || g.RowCount != 2 || l.RowCount != 2 || m.RowCount != 2 {
		t.Fatalf("datasets = %+v %+v %+v", g, l, m)
	}
	if g.FileCount != 13 || l.FileCount != 13 || m.FileCount != 13 {
		t.Fatalf("file counts = %d/%d/%d, want 13", g.FileCount, l.FileCount, m.FileCount)
	}
	if g.Version != 1 || g.Status != DatasetReady {
		t.Fatalf("global = %+v", g)
	}
	rows, err := st.DatasetRows(m.ID)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows = %+v %v", rows, err)
	}
	if rows[0].Weeks[0] != "101" || rows[0].Comments[16].Text != "note" || rows[0].CFColors[0] != "#FF0000" {
		t.Fatalf("row = %+v", rows[0])
	}

	// The old registry and physical tables are gone; a second open must not try
	// to migrate again.
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type='table' AND name IN
		('archive','batch','stg_row','batch_file','data_2639','data_2639_global','data_2639_local')`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("old tables remain: %d", n)
	}
	var version int
	if err := st.db.QueryRow(`SELECT CAST(value AS INTEGER) FROM app_meta WHERE key='schema_version'`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != 5 {
		t.Fatalf("schema version = %d, want 5", version)
	}
	st.Close()

	again, err := Open(path)
	if err != nil {
		t.Fatalf("reopen migrated: %v", err)
	}
	again.Close()
}

func TestMigrationHandlesEmptyV4(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE app_meta (key TEXT PRIMARY KEY, value TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO app_meta VALUES('schema_version','4')`); err != nil {
		t.Fatal(err)
	}
	// An archive with no rows, as a freshly installed v4 would have.
	if _, err := db.Exec(`CREATE TABLE archive (
		week_code TEXT NOT NULL, dataset TEXT NOT NULL DEFAULT 'merged', week_start TEXT NOT NULL,
		week_codes TEXT NOT NULL, index_names TEXT NOT NULL, row_count INTEGER NOT NULL,
		file_count INTEGER NOT NULL, batch_id INTEGER, committed_at TEXT NOT NULL,
		template_id INTEGER, param_snapshot TEXT, table_name TEXT NOT NULL,
		PRIMARY KEY (week_code, dataset))`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open empty v4: %v", err)
	}
	defer st.Close()
	list, err := st.ListIntegrations()
	if err != nil || len(list) != 0 {
		t.Fatalf("integrations = %+v %v", list, err)
	}
}
