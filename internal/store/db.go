// Package store persists everything in a local SQLite database: interned
// styles and conditional formats, the staging area, and one permanent table per
// committed week.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"
	"regexp"
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

// Store owns the database handle.
type Store struct {
	db   *sql.DB
	path string
}

// Open connects to the database, creating it if needed, and applies the schema.
func Open(path string) (*Store, error) {
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
	return &Store{db: db, path: path}, nil
}

// Close releases the database handle.
func (s *Store) Close() error { return s.db.Close() }

// Path is the database file location.
func (s *Store) Path() string { return s.path }

// Now returns the timestamp format used across the database.
func Now() string { return time.Now().Format("2006-01-02 15:04:05") }

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

// SetMeta stores a key/value pair.
func (s *Store) SetMeta(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO app_meta(key,value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

// GetMeta reads a key/value pair.
func (s *Store) GetMeta(key string) (string, bool, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM app_meta WHERE key=?`, key).Scan(&v)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return v, true, nil
}
