package main

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"project_clear/internal/logging"
	"project_clear/internal/service"
	"project_clear/internal/store"
)

// A page that loads before the backend is ready must be served, not told the
// application is not initialised: Wails runs OnStartup and OnDomReady on
// separate goroutines, and the first render was racing the database open.
func TestReadyWaitsForBoot(t *testing.T) {
	a := NewApp()
	done := make(chan error, 1)
	go func() { done <- a.ready() }()

	select {
	case err := <-done:
		t.Fatalf("ready returned before startup finished: %v", err)
	case <-time.After(150 * time.Millisecond):
	}

	a.bootErr = errors.New("boom")
	close(a.bootDone)
	err := <-done
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("ready after a failed boot = %v, want the boot error", err)
	}
}

func TestReadySucceedsAfterBoot(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "clear.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer db.Close()

	a := NewApp()
	a.data = dir
	a.log = logging.New(filepath.Join(dir, "logs"))
	a.db = db
	a.svc = service.New(nil, a.log, db, dir)
	close(a.bootDone)

	if err := a.ready(); err != nil {
		t.Fatalf("ready: %v", err)
	}
	// The log panel asks for its backlog the same way, before boot may be done.
	if entries := a.GetLogs(10); len(entries) != 0 {
		t.Errorf("GetLogs on an empty ring = %v, want none", entries)
	}
}
