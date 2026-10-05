package service

import (
	"os"
	"path/filepath"
	"testing"

	"project_clear/internal/store"
)

// The template copy is taken after the staging write, so a write that fails
// leaves nothing behind. It used to be copied first, which meant a failed import
// could leave a folder on disk with no database row pointing at it.
func TestImportDoesNotStoreATemplateWhenTheWriteFails(t *testing.T) {
	svc, dataDir := newTestService(t)
	folder := writeSourceFolder(t)

	// A closed database cannot accept the staging write. Reading the workbooks
	// does not touch it, so the import gets as far as the write.
	if err := svc.db.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}
	if _, err := svc.ImportFolder(folder, nil); err == nil {
		t.Fatal("the import reported success with a closed database")
	}

	entries, err := os.ReadDir(filepath.Join(dataDir, "templates"))
	if os.IsNotExist(err) {
		return // nothing was created at all, which is the point
	}
	if err != nil {
		t.Fatalf("read templates dir: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("a failed write left template folders behind: %v", entries)
	}
}

// A successful import still stores the template, row and copy alike: the copy
// moved later, it did not go away.
func TestImportStoresTheTemplateRowAndCopy(t *testing.T) {
	svc, dataDir := newTestService(t)
	folder := writeSourceFolder(t)

	res, err := svc.ImportFolder(folder, nil)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if res.OK == 0 || res.WeekCode == "" {
		t.Fatalf("nothing was imported: %+v", res)
	}

	tpl, err := svc.db.TemplateFor(res.WeekCode)
	if err != nil {
		t.Fatalf("template row: %v", err)
	}
	if tpl == nil {
		t.Fatalf("no template row for week %s", res.WeekCode)
	}
	entries, err := os.ReadDir(filepath.Join(dataDir, "templates", res.WeekCode))
	if err != nil {
		t.Fatalf("template folder: %v", err)
	}
	if len(entries) == 0 {
		t.Error("the template folder is empty")
	}
	if entries[0].Name() != tpl.SrcName {
		t.Errorf("stored copy = %q, row says %q", entries[0].Name(), tpl.SrcName)
	}
}

// A database transaction cannot delete files. The service removes the folder of
// every week whose template row the write dropped, so the two stay in step.
func TestDropOrphanedTemplatesRemovesTheFolder(t *testing.T) {
	svc, dataDir := newTestService(t)

	dir := filepath.Join(dataDir, "templates", "2639")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "a.xlsm"), []byte("workbook"), 0o644); err != nil {
		t.Fatal(err)
	}

	svc.dropOrphanedTemplates(&store.StagingResult{OrphanedTemplates: []string{"2639"}})

	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("the superseded template folder survived (stat err %v)", err)
	}
}

// Nothing to drop must not panic or touch anything.
func TestDropOrphanedTemplatesHandlesNothingToDo(t *testing.T) {
	svc, dataDir := newTestService(t)

	keep := filepath.Join(dataDir, "templates", "2640")
	if err := os.MkdirAll(keep, 0o755); err != nil {
		t.Fatal(err)
	}

	svc.dropOrphanedTemplates(nil)
	svc.dropOrphanedTemplates(&store.StagingResult{})

	if _, err := os.Stat(keep); err != nil {
		t.Errorf("an unrelated template folder was removed: %v", err)
	}
}
