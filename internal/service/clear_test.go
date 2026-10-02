package service

import (
	"os"
	"path/filepath"
	"testing"
)

// 清空 removes what an import staged and never integrated: the rows, the
// per-file records, and the template copy waiting on disk for an export.
func TestClearStagingRemovesAnUnsavedImport(t *testing.T) {
	svc, dir := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import: %v", err)
	}

	sum, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if !sum.HasStaging || sum.RowKept == 0 {
		t.Fatalf("nothing staged after import: %+v", sum)
	}
	tplDir := filepath.Join(dir, "templates", sum.WeekCode)
	if entries, err := os.ReadDir(tplDir); err != nil || len(entries) == 0 {
		t.Fatalf("template copy missing at %s: %v", tplDir, err)
	}

	rows, err := svc.ClearStaging()
	if err != nil {
		t.Fatalf("clear staging: %v", err)
	}
	if rows != sum.RowKept {
		t.Errorf("cleared rows = %d, want %d", rows, sum.RowKept)
	}

	after, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging after clear: %v", err)
	}
	if after.HasStaging {
		t.Errorf("staging survived: %+v", after)
	}
	if _, err := os.Stat(tplDir); !os.IsNotExist(err) {
		t.Errorf("template folder survived the clear: %v", err)
	}

	// Pressing the button again must not fail or report rows that are gone.
	if rows, err := svc.ClearStaging(); err != nil || rows != 0 {
		t.Errorf("second clear = (%d, %v), want (0, nil)", rows, err)
	}
}
