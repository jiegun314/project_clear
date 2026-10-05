package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Save stamps the current format version. A caller that posts a stale or empty
// marker must not be able to make the file look like a pre-versioning one, or
// the next launch re-runs the version-2 migration and silently reverts a
// deliberate 「原文件格式」 choice to 「纯数据」.
func TestSaveStampsTheCurrentConfigVersion(t *testing.T) {
	dir := t.TempDir()

	store, _, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	// A client that echoes an empty marker, as an older webview would.
	if _, err := store.Save(Config{
		ConfigVersion: 0, ReadColumns: 20, PageSize: 200,
		HeaderDisplay: HeaderTwoRow, ExportMode: ExportTemplate, LOCFilter: "WH_CNB",
	}); err != nil {
		t.Fatalf("save: %v", err)
	}

	raw, err := os.ReadFile(filepath.Join(dir, "clear.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(raw), "configVersion: 2") {
		t.Errorf("the marker was not stamped: %s", raw)
	}

	// The deliberate choice must survive the reload.
	again, _, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got := again.Get().ExportMode; got != ExportTemplate {
		t.Errorf("a saved 「原文件格式」 choice was reverted to %q", got)
	}
	if got := again.Get().ConfigVersion; got != currentConfigVersion {
		t.Errorf("reloaded version = %d, want %d", got, currentConfigVersion)
	}
}

// An out-of-range value is clamped in memory on load. It also has to be written
// back, otherwise the same correction is reported on every single launch while
// the file keeps the bad value.
func TestLoadPersistsValidationCorrections(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clear.yaml")
	if err := os.WriteFile(path, []byte(
		"configVersion: 2\nreadColumns: 999\npageSize: 1\nlocFilter: \"\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, notes, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("the corrections were not reported")
	}
	if got := store.Get().ReadColumns; got != 20 {
		t.Errorf("readColumns = %d, want the clamped 20", got)
	}
	if got := store.Get().PageSize; got != 200 {
		t.Errorf("pageSize = %d, want the clamped 200", got)
	}

	// The file now carries the corrected values, so a second start has nothing
	// left to fix and reports nothing.
	if _, notes, err := NewStoreAt(dir); err != nil {
		t.Fatalf("reload: %v", err)
	} else if len(notes) != 0 {
		t.Errorf("the correction was reported again on reload: %v", notes)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"readColumns: 20", "pageSize: 200", "locFilter: WH_CNB"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("corrected file is missing %q:\n%s", want, raw)
		}
	}
}

// Save persists the corrections it reports, so the caller sees the same values
// that were written.
func TestSavePersistsTheCorrectionsItReports(t *testing.T) {
	dir := t.TempDir()
	store, _, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	notes, err := store.Save(Config{
		ReadColumns: 999, PageSize: 1, HeaderDisplay: "nonsense",
		ExportMode: "nonsense", LOCFilter: "  ",
	})
	if err != nil {
		t.Fatalf("save: %v", err)
	}
	if len(notes) == 0 {
		t.Fatal("no corrections were reported")
	}
	if got := store.Get().ReadColumns; got != 20 {
		t.Errorf("active readColumns = %d, want 20", got)
	}

	again, notes, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(notes) != 0 {
		t.Errorf("the file still needs correcting after Save: %v", notes)
	}
	if got := again.Get().ExportMode; got != ExportClean {
		t.Errorf("reloaded exportMode = %q, want %q", got, ExportClean)
	}
}

// A malformed file falls back to defaults but is left on disk: overwriting it
// would destroy whatever the user was writing.
func TestLoadLeavesAMalformedFileAlone(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clear.yaml")
	broken := "readColumns: [this is not a number\n"
	if err := os.WriteFile(path, []byte(broken), 0o644); err != nil {
		t.Fatal(err)
	}

	store, notes, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !mentions(notes, "解析失败") {
		t.Errorf("the parse failure was not reported: %v", notes)
	}
	if got := store.Get().ReadColumns; got != Default().ReadColumns {
		t.Errorf("readColumns = %d, want the default %d", got, Default().ReadColumns)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != broken {
		t.Errorf("the malformed file was overwritten:\n got %q\nwant %q", raw, broken)
	}
}

// Advancing the version marker on a file that already carries the current
// default has nothing to report. It used to append an empty string, which the
// settings dialog rendered as a blank bullet above the form.
func TestVersionMigrationWithoutAChangeReportsNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "clear.yaml")
	// A file written before the marker existed, already holding the new default.
	if err := os.WriteFile(path, []byte("readColumns: 20\nexportMode: clean\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	store, notes, err := NewStoreAt(dir)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if got := store.Get().ConfigVersion; got != currentConfigVersion {
		t.Errorf("version = %d, want the marker advanced to %d", got, currentConfigVersion)
	}
	for i, n := range notes {
		if strings.TrimSpace(n) == "" {
			t.Errorf("note %d is empty; the UI would show a blank line (%q)", i, notes)
		}
	}
	if len(notes) != 0 {
		t.Errorf("nothing needed correcting, but notes = %v", notes)
	}
}
