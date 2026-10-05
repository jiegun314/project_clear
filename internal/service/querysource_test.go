package service

import (
	"testing"
	"time"
)

// The application releases the database on shutdown, so it has to be able to
// wait until the operation in progress has finished: closing the handle under a
// running export would fail it halfway.
func TestWaitIdleWaitsForTheOperationInProgress(t *testing.T) {
	svc, _ := newTestService(t)

	// Stand in for a long-running operation holding the lock.
	svc.mu.Lock()
	done := make(chan struct{})
	go func() {
		svc.WaitIdle()
		close(done)
	}()

	select {
	case <-done:
		t.Fatal("WaitIdle returned while an operation still held the lock")
	case <-time.After(100 * time.Millisecond):
	}

	svc.mu.Unlock()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("WaitIdle did not return once the lock was released")
	}
}

// The grid must be able to tell "nothing imported yet" from "failed to load".
func TestGridSourceResolvesStagingAndCommittedWeeks(t *testing.T) {
	svc, _ := newTestService(t)

	empty, err := svc.GridSource("")
	if err != nil {
		t.Fatalf("empty source: %v", err)
	}
	if empty.HasStaging {
		t.Errorf("nothing is imported, but staging was reported present: %+v", empty)
	}

	folder := writeSourceFolder(t)
	if _, err := svc.ImportFolder(folder, nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	staged, err := svc.GridSource("")
	if err != nil {
		t.Fatalf("staging source: %v", err)
	}
	if !staged.HasStaging || staged.WeekCode == "" {
		t.Fatalf("staging source = %+v", staged)
	}
	if len(staged.WeekCodes) == 0 || len(staged.IndexNames) == 0 {
		t.Fatalf("staging source lost its columns: %+v", staged)
	}
	week := staged.WeekCode

	if _, _, err := svc.Commit(); err != nil {
		t.Fatalf("commit: %v", err)
	}

	committed, err := svc.GridSource(week)
	if err != nil {
		t.Fatalf("committed source: %v", err)
	}
	if committed.HasStaging {
		t.Errorf("a committed week was reported as staging: %+v", committed)
	}
	if committed.WeekCode != week || committed.WeekStart != staged.WeekStart {
		t.Errorf("committed source = %+v, want week %s starting %s", committed, week, staged.WeekStart)
	}
	if len(committed.WeekCodes) != len(staged.WeekCodes) {
		t.Errorf("committed columns = %v, want %v", committed.WeekCodes, staged.WeekCodes)
	}

	// A week code that is valid but has never been integrated is empty, not an
	// error: the grid shows its placeholder.
	missing, err := svc.GridSource("9999")
	if err != nil {
		t.Fatalf("unknown week: %v", err)
	}
	if missing.WeekCode != "" || len(missing.WeekCodes) != 0 || missing.HasStaging {
		t.Errorf("unknown week source = %+v, want empty", missing)
	}
}

// The overwrite flag tells the user whether 整合 replaced an existing week. It
// used to be read before the lock was taken, so a concurrent import could make
// it describe a different batch.
func TestCommitReportsWhetherItReplacedAWeek(t *testing.T) {
	svc, _ := newTestService(t)
	folder := writeSourceFolder(t)

	if _, err := svc.ImportFolder(folder, nil); err != nil {
		t.Fatalf("import: %v", err)
	}
	entry, overwrote, err := svc.Commit()
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}
	if overwrote {
		t.Error("the first commit reported replacing an existing week")
	}

	if _, err := svc.ImportFolder(folder, nil); err != nil {
		t.Fatalf("re-import: %v", err)
	}
	again, overwroteAgain, err := svc.Commit()
	if err != nil {
		t.Fatalf("second commit: %v", err)
	}
	if !overwroteAgain {
		t.Error("re-integrating the same week was not reported as a replacement")
	}
	if again.WeekCode != entry.WeekCode {
		t.Errorf("second commit week = %s, want %s", again.WeekCode, entry.WeekCode)
	}
}
