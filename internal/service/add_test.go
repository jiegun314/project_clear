package service

import (
	"os"
	"path/filepath"
	"testing"

	"project_clear/internal/store"
)

// 添加 grows the integration list: the file that is added joins the one that
// was imported before it instead of replacing it, and the whole list is still
// one week waiting to be integrated.
func TestAddFilesMergesIntoTheStagedList(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import folder: %v", err)
	}

	second := writeSourceFolder(t)
	secondPath := filepath.Join(second, "source.xlsm")
	extraPath := filepath.Join(second, "extra.xlsm")
	if err := os.Rename(secondPath, extraPath); err != nil {
		t.Fatalf("rename second workbook: %v", err)
	}

	res, err := svc.AddFiles([]string{extraPath}, nil, false)
	if err != nil {
		t.Fatalf("add file: %v", err)
	}
	if res.NeedsConfirm {
		t.Fatalf("a new file must not ask for confirmation: %+v", res)
	}

	sum, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if sum.FileOK != 2 || sum.RowKept != 2 {
		t.Fatalf("staged list = %d files / %d rows, want 2 / 2", sum.FileOK, sum.RowKept)
	}
	files, err := svc.db.StagingFiles()
	if err != nil {
		t.Fatalf("staging files: %v", err)
	}
	if len(files) != 2 || files[0].Name != "source.xlsm" || files[1].Name != "extra.xlsm" {
		t.Fatalf("staging files = %+v, want source.xlsm then extra.xlsm", files)
	}

	grid, err := svc.Query(store.Query{Source: "", Page: 1, PageSize: 10, SortField: "seq"})
	if err != nil {
		t.Fatalf("query staging: %v", err)
	}
	if grid.Total != 2 {
		t.Fatalf("staged rows = %d, want 2", grid.Total)
	}
	if grid.Rows[0].FileName != "source.xlsm" || grid.Rows[1].FileName != "extra.xlsm" {
		t.Fatalf("row order = %q, %q; want the imported file first", grid.Rows[0].FileName, grid.Rows[1].FileName)
	}
}

// The same workbook may not be added twice: the second attempt stops and asks
// for confirmation, and only the confirmed retry replaces its rows.
func TestAddFilesRefusesADuplicateUntilConfirmed(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import folder: %v", err)
	}
	dir := writeSourceFolder(t)
	path := filepath.Join(dir, "source.xlsm")

	// First add: the file is new, so it just joins the list.
	if res, err := svc.AddFiles([]string{path}, nil, false); err != nil || res.NeedsConfirm {
		t.Fatalf("first add = (%+v, %v), want it to go through", res, err)
	}
	before, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}

	// Second add of the same path: no write happens, the UI gets the file list
	// to show in the overwrite prompt.
	res, err := svc.AddFiles([]string{path}, nil, false)
	if err != nil {
		t.Fatalf("duplicate add: %v", err)
	}
	if !res.NeedsConfirm {
		t.Fatalf("duplicate add did not ask for confirmation: %+v", res)
	}
	if len(res.DuplicateFiles) != 1 || res.DuplicateFiles[0] != "source.xlsm" {
		t.Fatalf("duplicate files = %v, want [source.xlsm]", res.DuplicateFiles)
	}
	if len(res.PendingPaths) != 1 || res.PendingPaths[0] != path {
		t.Fatalf("pending paths = %v, want [%s]", res.PendingPaths, path)
	}
	after, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if after.FileOK != before.FileOK || after.RowKept != before.RowKept {
		t.Fatalf("staging changed before the user confirmed: %+v -> %+v", before, after)
	}

	// Confirmed retry: the rows are replaced, the file count does not grow.
	if res, err := svc.AddFiles([]string{path}, nil, true); err != nil {
		t.Fatalf("confirmed add: %v", err)
	} else if res.NeedsConfirm {
		t.Fatalf("confirmed add still asks: %+v", res)
	}
	final, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if final.FileOK != 2 || final.RowKept != 2 {
		t.Fatalf("staged list after overwrite = %d files / %d rows, want 2 / 2", final.FileOK, final.RowKept)
	}
}
