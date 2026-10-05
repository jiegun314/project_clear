package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// openLegacy builds a database the way an earlier release left it: the current
// schema, optionally with columns dropped, and no version marker.
func openLegacy(t *testing.T, path string, drop ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open legacy: %v", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("legacy schema: %v", err)
	}
	for _, col := range drop {
		if _, err := db.Exec(fmt.Sprintf(`ALTER TABLE stg_row DROP COLUMN %s`, col)); err != nil {
			t.Fatalf("drop %s: %v", col, err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close legacy: %v", err)
	}
}

func openRaw(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	return db
}

// A database this release creates already carries every migration, so it is
// stamped as current and nothing is copied aside.
func TestOpenStampsAFreshDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.db")
	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if got, err := schemaVersionOf(st.db); err != nil || got != currentSchemaVersion {
		t.Errorf("fresh database version = %d (err %v), want %d", got, err, currentSchemaVersion)
	}
	if _, err := os.Stat(fmt.Sprintf("%s.v0.bak", path)); !os.IsNotExist(err) {
		t.Errorf("a fresh database should not be backed up (stat err %v)", err)
	}
}

// A database written before the marker existed is upgraded, stamped as current,
// and copied aside first so the upgrade can be undone by hand.
func TestOpenMigratesAndBacksUpALegacyDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.db")
	openLegacy(t, path, "cf_colors")

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	if has, err := hasColumn(st.db, "stg_row", "cf_colors"); err != nil || !has {
		t.Fatalf("cf_colors was not restored (has=%v err=%v)", has, err)
	}
	if got, err := schemaVersionOf(st.db); err != nil || got != currentSchemaVersion {
		t.Errorf("version after the upgrade = %d (err %v), want %d", got, err, currentSchemaVersion)
	}
	backup := fmt.Sprintf("%s.v0.bak", path)
	if _, err := os.Stat(backup); err != nil {
		t.Errorf("no pre-upgrade copy at %s: %v", backup, err)
	}
}

// Once a database is at the current version the upgrade is a no-op: no second
// copy, and the columns it already has stay put.
func TestOpenIsIdempotentAfterMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.db")
	openLegacy(t, path, "comments")

	first, err := Open(path)
	if err != nil {
		t.Fatalf("first open: %v", err)
	}
	first.Close()
	if err := os.Remove(fmt.Sprintf("%s.v0.bak", path)); err != nil {
		t.Fatalf("remove the first copy: %v", err)
	}

	second, err := Open(path)
	if err != nil {
		t.Fatalf("second open: %v", err)
	}
	defer second.Close()

	if got, err := schemaVersionOf(second.db); err != nil || got != currentSchemaVersion {
		t.Errorf("version = %d (err %v), want %d", got, err, currentSchemaVersion)
	}
	if _, err := os.Stat(fmt.Sprintf("%s.v0.bak", path)); !os.IsNotExist(err) {
		t.Errorf("a current database was upgraded again (stat err %v)", err)
	}
	if has, err := hasColumn(second.db, "stg_row", "comments"); err != nil || !has {
		t.Errorf("the comments column disappeared (has=%v err=%v)", has, err)
	}
}

// An upgrade is a single transaction. A step that fails part way must leave the
// database exactly as it was, with the version marker unmoved, so the user can
// retry or restore the copy instead of being left with a half-migrated file.
func TestMigrateRollsBackAFailedStep(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.db")
	openLegacy(t, path, "cf_colors")

	saved := schemaMigrations
	defer func() { schemaMigrations = saved }()
	schemaMigrations = []func(sqlQueryer) error{
		func(q sqlQueryer) error {
			if _, err := q.Exec(`ALTER TABLE stg_row ADD COLUMN half_applied TEXT`); err != nil {
				return err
			}
			return fmt.Errorf("boom")
		},
	}

	if _, err := Open(path); err == nil {
		t.Fatal("Open reported success, want the failed step to surface")
	}

	db := openRaw(t, path)
	defer db.Close()
	if has, err := hasColumn(db, "stg_row", "half_applied"); err != nil || has {
		t.Errorf("the failed step's column survived (has=%v err=%v)", has, err)
	}
	if got, err := schemaVersionOf(db); err != nil || got != 0 {
		t.Errorf("version after a failed upgrade = %d (err %v), want 0", got, err)
	}
}

// SaveStaging and MergeStaging are public. A caller that supplies no style
// dictionary must get an empty mapping rather than a nil-pointer panic; the
// conditional-format dictionary next to it has always been guarded.
func TestSaveStagingWithoutAStyleDictionaryDoesNotPanic(t *testing.T) {
	st := openTestStore(t)
	in := stagedBatch(nil)
	in.Dict = nil

	if _, err := st.SaveStaging(in); err != nil {
		t.Fatalf("save staging without a dictionary: %v", err)
	}
	if _, err := st.MergeStaging(in, nil); err != nil {
		t.Fatalf("merge staging without a dictionary: %v", err)
	}
}

// An existing database kept a second full copy of every integrated week, because
// Commit used to leave the staging rows behind. Opening that file removes the
// redundant copies so it shrinks back instead of staying at roughly double size.
// The staging area of a batch that has not been integrated must survive.
func TestUpgradePurgesTheStagingRowsOfCommittedWeeks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "clear.db")
	db := openRaw(t, path)
	if _, err := db.Exec(schemaSQL); err != nil {
		t.Fatalf("schema: %v", err)
	}
	committed := `INSERT INTO batch(created_at, action, week_code, week_start, week_codes, index_names,
	                  status, row_total, row_kept)
	              VALUES('2026-01-01 00:00:00','import','2639','2026-09-21','["2639"]','["A"]',?,1,1)`
	if _, err := db.Exec(committed, "committed"); err != nil {
		t.Fatalf("insert committed batch: %v", err)
	}
	if _, err := db.Exec(committed, "staging"); err != nil {
		t.Fatalf("insert staging batch: %v", err)
	}
	for _, batchID := range []int{1, 2} {
		if _, err := db.Exec(
			`INSERT INTO stg_row(batch_id, seq, weeks, style_ids) VALUES(?,0,'["1"]','[0]')`, batchID); err != nil {
			t.Fatalf("insert staging row: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	st, err := Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer st.Close()

	count := func(batchID int) int {
		t.Helper()
		var n int
		if err := st.db.QueryRow(`SELECT COUNT(1) FROM stg_row WHERE batch_id=?`, batchID).Scan(&n); err != nil {
			t.Fatalf("count batch %d: %v", batchID, err)
		}
		return n
	}
	if n := count(1); n != 0 {
		t.Errorf("the committed batch kept %d staging rows", n)
	}
	if n := count(2); n != 1 {
		t.Errorf("the staging batch lost its rows: %d", n)
	}
	if got, err := schemaVersionOf(st.db); err != nil || got != currentSchemaVersion {
		t.Errorf("version = %d (err %v), want %d", got, err, currentSchemaVersion)
	}
}
