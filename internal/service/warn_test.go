package service

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"project_clear/internal/store"
)

// The store reports a payload it cannot decode instead of failing the query, and
// the service is what turns that into a log line. Without the wiring the message
// goes nowhere and a missing colour stays unexplained.
func TestDatabaseWarningsReachTheLog(t *testing.T) {
	svc, dir := newTestService(t)
	folder := writeSourceFolder(t)
	if _, err := svc.ImportFolder(folder, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	// Damage a stored payload through a second connection, the way a truncated
	// write would leave it.
	raw, err := sql.Open("sqlite", filepath.Join(dir, "clear.db"))
	if err != nil {
		t.Fatalf("open raw: %v", err)
	}
	if _, err := raw.Exec(`UPDATE stg_row SET weeks='{not json'`); err != nil {
		raw.Close()
		t.Fatalf("damage payload: %v", err)
	}
	raw.Close()

	if _, err := svc.Query(store.Query{Source: "", Page: 1, PageSize: 5, SortField: "seq"}); err != nil {
		t.Fatalf("query: %v", err)
	}

	var found string
	for _, e := range svc.log.Recent(50) {
		if strings.Contains(e.Message, "周数据") {
			found = e.Source + " " + e.Message
			break
		}
	}
	if found == "" {
		t.Errorf("the warning never reached the log; recent entries: %v", svc.log.Recent(5))
	}
}
