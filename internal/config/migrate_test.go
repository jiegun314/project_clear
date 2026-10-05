package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A settings file written before 纯数据 became the default must follow the new
// default once, and say so — but a file the user has already saved with the
// current version must be left alone, choice included.
func TestExportModeDefaultMigratesOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clear.yaml")
	// The 1.0 file: no version marker, and the old default.
	if err := os.WriteFile(path, []byte("readColumns: 20\nexportMode: template\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, notes, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := store.Get().ExportMode; got != ExportClean {
		t.Errorf("export mode = %q, want %q", got, ExportClean)
	}
	if !mentions(notes, "纯数据") {
		t.Errorf("the change should be reported, notes = %v", notes)
	}
	// The upgraded file is persisted, so the next start is a no-op.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "configVersion: 2") || !strings.Contains(string(raw), "exportMode: clean") {
		t.Errorf("upgraded file = %s", raw)
	}

	// A deliberate choice of 原文件格式 survives a reload.
	if _, _, err := NewStoreAt(dir); err != nil {
		t.Fatalf("reload: %v", err)
	}
	if _, err := store.Save(Config{ConfigVersion: currentConfigVersion, ReadColumns: 20, PageSize: 200,
		HeaderDisplay: HeaderTwoRow, ExportMode: ExportTemplate, LOCFilter: "WH_CNB"}); err != nil {
		t.Fatal(err)
	}
	again, _, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("reload after choosing template: %v", err)
	}
	if got := again.Get().ExportMode; got != ExportTemplate {
		t.Errorf("a saved choice was reverted: %q", got)
	}
}

func mentions(notes []string, want string) bool {
	for _, n := range notes {
		if strings.Contains(n, want) {
			return true
		}
	}
	return false
}
