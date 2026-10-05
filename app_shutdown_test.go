package main

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"project_clear/internal/logging"
	"project_clear/internal/service"
	"project_clear/internal/store"
)

// Wails never cancels the context it passes to OnStartup, so the log stream used
// to run for the life of the process. It now follows a context the application
// owns and cancels on shutdown.
func TestStreamLogsStopsWhenItsContextIsCancelled(t *testing.T) {
	a := NewApp()
	a.ctx = context.Background()
	a.log = logging.New(filepath.Join(t.TempDir(), "logs"))

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		a.streamLogs(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("streamLogs kept running after its context was cancelled")
	}
}

// Quit and OnShutdown can both run, in either order, so the clean-up has to be
// safe to repeat and must cancel the log stream.
func TestShutdownIsIdempotentAndCancelsTheRunContext(t *testing.T) {
	a := NewApp()
	a.runCtx, a.cancel = context.WithCancel(context.Background())

	a.shutdown(context.Background())
	select {
	case <-a.runCtx.Done():
	case <-time.After(time.Second):
		t.Fatal("shutdown did not cancel the run context")
	}

	// The second call is the one that arrives from the other path.
	a.shutdown(context.Background())
}

// Closing the window has to release the database, which never happened before:
// OnShutdown was not set, so only the 退出 button closed it.
func TestShutdownClosesTheDatabase(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(filepath.Join(dir, "clear.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}

	a := NewApp()
	a.data = dir
	a.log = logging.New(filepath.Join(dir, "logs"))
	a.db = db
	a.svc = service.New(nil, a.log, db, dir)

	a.shutdown(context.Background())

	// Any query now fails, which is how "the handle was released" shows up.
	if _, err := db.LoadStaging(); err == nil {
		t.Error("the database is still usable after shutdown")
	}
}

// Shutdown before startup, or after a boot that failed, must not panic: there is
// nothing to release yet.
func TestShutdownBeforeBootIsSafe(t *testing.T) {
	a := NewApp()
	a.shutdown(context.Background())
	a.shutdown(context.Background())
}
