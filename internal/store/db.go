// Package store persists everything in a local SQLite database: interned
// styles and conditional formats, the staging area, and one permanent table per
// committed week.
package store

import (
	"database/sql"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var schemaSQL string

// weekCodeRe guards every identifier that reaches SQL as a table or column
// name. Week codes are parsed from a header cell, so they are validated rather
// than trusted.
var weekCodeRe = regexp.MustCompile(`^\d{4}$`)

// metaSchemaVersion is the app_meta key holding the schema the file is at.
const metaSchemaVersion = "schema_version"

// currentSchemaVersion is the schema this release expects. It is bumped only
// when schemaMigrations gains a step at the end.
const currentSchemaVersion = 2

// annotationColumns are the per-cell extras added after 1.0. Both are nullable
// TEXT: a database that predates them simply has no values.
var annotationColumns = []string{"comments", "cf_colors"}

// sqlQueryer is the part of *sql.DB and *sql.Tx the helpers below need, so a
// migration step can run inside the transaction that applies it.
type sqlQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
	Exec(query string, args ...any) (sql.Result, error)
}

// schemaMigrations are applied in order. Index i upgrades a database at version
// i to version i+1, so new work is appended and an existing file replays exactly
// the steps it has not seen.
var schemaMigrations = []func(sqlQueryer) error{
	addAnnotationColumns,      // 0 -> 1
	purgeCommittedStagingRows, // 1 -> 2
}

// Store owns the database handle.
type Store struct {
	db   *sql.DB
	path string
	// warn reports a recoverable problem, such as a stored payload that will not
	// decode. It is nil until the application installs one, so the store stays
	// usable — and logger-free — on its own.
	warn func(format string, args ...any)
}

// SetWarn installs the sink recoverable problems are reported to.
//
// A damaged payload must not fail a whole query, but it must not vanish either:
// a cell whose colour or annotation is silently missing cannot be explained from
// the outside, and that is exactly how such a defect survives unnoticed.
func (s *Store) SetWarn(warn func(format string, args ...any)) { s.warn = warn }

func (s *Store) warnf(format string, args ...any) {
	if s.warn != nil {
		s.warn(format, args...)
	}
}

// decodeJSON restores a stored JSON payload, reporting one that will not decode.
// The destination is left empty so a single damaged field degrades that field
// alone rather than the whole read.
func (s *Store) decodeJSON(what, raw string, dst any) {
	if raw == "" {
		return
	}
	if err := json.Unmarshal([]byte(raw), dst); err != nil {
		s.warnf("读取%s失败，该项已置空: %v", what, err)
	}
}

// Open connects to the database, creating it if needed, and applies the schema.
func Open(path string) (*Store, error) {
	// Whether the file is being created decides how it is brought up to date: a
	// database this release creates already carries every migration, while an
	// existing one has to replay the steps it has not seen.
	_, statErr := os.Stat(path)
	fresh := errors.Is(statErr, os.ErrNotExist)

	dsn := path + "?_pragma=busy_timeout(10000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("打开数据库失败: %w", err)
	}
	// SQLite tolerates one writer; keeping the pool small avoids lock churn.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("连接数据库失败: %w", err)
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		db.Close()
		return nil, fmt.Errorf("初始化数据库结构失败: %w", err)
	}
	if fresh {
		if err := stampSchemaVersion(db, currentSchemaVersion); err != nil {
			db.Close()
			return nil, fmt.Errorf("记录数据库结构版本失败: %w", err)
		}
	} else if err := migrate(db, path); err != nil {
		db.Close()
		return nil, fmt.Errorf("升级数据库结构失败: %w", err)
	}
	return &Store{db: db, path: path}, nil
}

// migrate brings a database written by an earlier release up to the current
// schema, one versioned step at a time and inside a single transaction, after
// copying the file aside so a failed upgrade can be undone.
func migrate(db *sql.DB, path string) error {
	version, err := schemaVersionOf(db)
	if err != nil {
		return err
	}
	if version >= currentSchemaVersion {
		return nil
	}
	// The copy is taken before anything changes, so it holds the pre-upgrade
	// database. A failure here stops the upgrade: running it without a way back
	// is worse than refusing to start.
	if _, err := backupDatabase(db, path, version); err != nil {
		return fmt.Errorf("升级前备份数据库失败: %w", err)
	}

	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for v := version; v < currentSchemaVersion; v++ {
		if err := schemaMigrations[v](tx); err != nil {
			return fmt.Errorf("应用第 %d 项结构升级失败: %w", v+1, err)
		}
	}
	if err := stampSchemaVersion(tx, currentSchemaVersion); err != nil {
		return err
	}
	return tx.Commit()
}

// addAnnotationColumns brings a database written by an earlier release up to the
// annotation columns. CLEAR 1.0 kept cell notes only in the staging area, so a
// committed weekly table has no comments column, and 1.5.4 added the
// conditional-format colours; adding them in place means a week integrated
// before either change can still be exported with its notes and colours.
func addAnnotationColumns(q sqlQueryer) error {
	tables, err := dataTables(q)
	if err != nil {
		return err
	}
	for _, table := range tables {
		for _, col := range annotationColumns {
			if err := addColumnIfMissing(q, table, col); err != nil {
				return err
			}
		}
	}
	// The staging table needs the same treatment: CREATE TABLE IF NOT EXISTS
	// leaves an existing one untouched, and importing then fails with
	// "table stg_row has no column named cf_colors".
	for _, col := range annotationColumns {
		if err := addColumnIfMissing(q, "stg_row", col); err != nil {
			return err
		}
	}
	return nil
}

// purgeCommittedStagingRows removes the staging rows of weeks that were already
// integrated. Commit used to leave them behind, so every integrated week kept a
// second, full copy of its data in stg_row and the database grew week after
// week. Nothing reads them once a batch is committed: the grid reads
// data_<week>, and the 已导入文件 list reads batch_file.
func purgeCommittedStagingRows(q sqlQueryer) error {
	_, err := q.Exec(
		`DELETE FROM stg_row WHERE batch_id IN (SELECT id FROM batch WHERE status='committed')`)
	return err
}

// dataTables lists the per-week tables (data_2639 and friends).
func dataTables(q sqlQueryer) ([]string, error) {
	rows, err := q.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name LIKE 'data\_%' ESCAPE '\'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var tables []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		if weekCodeRe.MatchString(strings.TrimPrefix(name, "data_")) {
			tables = append(tables, name)
		}
	}
	return tables, rows.Err()
}

func addColumnIfMissing(q sqlQueryer, table, column string) error {
	has, err := hasColumn(q, table, column)
	if err != nil {
		return err
	}
	if has {
		return nil
	}
	_, err = q.Exec(fmt.Sprintf(`ALTER TABLE %s ADD COLUMN %s TEXT`, quoteIdent(table), column))
	return err
}

// schemaVersionOf reports the schema version recorded in the file; a database
// written before the marker existed reads back as version 0.
func schemaVersionOf(db *sql.DB) (int, error) {
	var raw string
	err := db.QueryRow(`SELECT value FROM app_meta WHERE key=?`, metaSchemaVersion).Scan(&raw)
	if err == sql.ErrNoRows {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("数据库结构版本 %q 无法解析", raw)
	}
	return n, nil
}

func stampSchemaVersion(q sqlQueryer, version int) error {
	_, err := q.Exec(
		`INSERT INTO app_meta(key,value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		metaSchemaVersion, strconv.Itoa(version))
	return err
}

// backupDatabase copies the database next to itself before an upgrade and
// returns the copy's path. The copy is taken after a WAL checkpoint, otherwise
// committed pages still sitting in the -wal file would be missing from it.
func backupDatabase(db *sql.DB, path string, fromVersion int) (string, error) {
	// A checkpoint that cannot run must not block the upgrade; the copy is then
	// a snapshot of the main file alone, which is still better than nothing.
	_, _ = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`)

	dest := fmt.Sprintf("%s.v%d.bak", path, fromVersion)
	if err := copyFile(path, dest); err != nil {
		return "", err
	}
	return dest, nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func hasColumn(q sqlQueryer, table, column string) (bool, error) {
	rows, err := q.Query(fmt.Sprintf(`PRAGMA table_info(%s)`, quoteIdent(table)))
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var (
			cid        int
			name, ctyp string
			notNull    int
			dflt       sql.NullString
			pk         int
		)
		if err := rows.Scan(&cid, &name, &ctyp, &notNull, &dflt, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Now returns the timestamp format used across the database.
//
// It is a variable so a test can move the clock: whether a record that was
// written and the value handed back to the caller came from the *same* reading is
// otherwise only observable when two readings straddle a second boundary, which a
// real run almost never does. (A Windows CI runner did, once.)
var Now = func() string { return time.Now().Format("2006-01-02 15:04:05") }

// ValidWeekCode reports whether a code is safe to interpolate into SQL.
func ValidWeekCode(code string) bool { return weekCodeRe.MatchString(code) }

// DataTableName is the permanent table for a week (2639 -> data_2639).
func DataTableName(code string) string { return "data_" + code }

// WeekColumnName is the permanent column for one week column (2639 -> wk_2639).
func WeekColumnName(code string) string { return "wk_" + code }

func mustWeek(code string) (string, error) {
	if !ValidWeekCode(code) {
		return "", fmt.Errorf("非法周码 %q", code)
	}
	return code, nil
}

// quoteIdent is a second line of defence for dynamic identifiers.
func quoteIdent(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }
