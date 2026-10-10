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

	if _, err := svc.AddFiles([]string{extraPath}, nil); err != nil {
		t.Fatalf("add file: %v", err)
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

// 再次添加同名文件：直接覆盖原有内容——不询问，也不累加。
//
// The fixture writes every workbook into its own folder, so this is the case that
// matters in practice: the same report saved to a second folder. Matching on the
// path would treat it as a new file and merge its week values a second time.
func TestAddFilesReplacesAFileWithTheSameName(t *testing.T) {
	svc, _ := newTestService(t)
	if _, err := svc.ImportFolder(writeSourceFolder(t), nil); err != nil {
		t.Fatalf("import folder: %v", err)
	}
	before, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if before.FileOK != 1 || before.RowKept != 1 {
		t.Fatalf("staged after import = %d files / %d rows, want 1 / 1", before.FileOK, before.RowKept)
	}

	other := writeSourceFolder(t)
	res, err := svc.AddFiles([]string{filepath.Join(other, "source.xlsm")}, nil)
	if err != nil {
		t.Fatalf("add same-named file: %v", err)
	}
	if len(res.Replaced) != 1 || res.Replaced[0] != "source.xlsm" {
		t.Fatalf("replaced = %v, want [source.xlsm]", res.Replaced)
	}

	after, err := svc.db.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if after.FileOK != 1 || after.RowKept != 1 {
		t.Fatalf("staged after replacing = %d files / %d rows, want 1 / 1: 同名文件必须整体覆盖，不能累加",
			after.FileOK, after.RowKept)
	}
	files, err := svc.db.StagingFiles()
	if err != nil {
		t.Fatalf("staging files: %v", err)
	}
	if len(files) != 1 || files[0].Name != "source.xlsm" {
		t.Fatalf("staging files = %+v, want a single source.xlsm", files)
	}
}
