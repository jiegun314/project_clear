package main

import (
	"os"
	"path/filepath"
	"testing"

	"project_clear/internal/config"
)

// The folder a chooser opens must exist. A remembered folder that has been
// deleted or moved used to be handed straight to the system dialog, which can only
// answer "folder does not exist" — leaving the user with no way to pick a file.
func TestStartDirDropsAFolderThatIsGone(t *testing.T) {
	present := map[string]bool{"/still/there": true, "/program": true, "/home/Documents": true}
	exists := func(p string) bool { return present[p] }

	cases := []struct {
		name       string
		remembered string
		want       string
	}{
		{"a folder that is still there is reused", "/still/there", "/still/there"},
		{"a deleted folder falls back to the program folder", "/gone", "/program"},
		{"an empty setting opens the program folder", "", "/program"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := startDir(c.remembered, exists, "/program", "/home/Documents")
			if got != c.want {
				t.Errorf("startDir(%q) = %q, want %q", c.remembered, got, c.want)
			}
		})
	}
}

// With neither the remembered folder nor the program folder available, the home
// folder is still better than an empty string, which would open the dialog
// wherever the system last left it.
func TestStartDirFallsBackToHome(t *testing.T) {
	exists := func(p string) bool { return p == "/home/Documents" }
	if got := startDir("/gone", exists, "", "/home/Documents"); got != "/home/Documents" {
		t.Errorf("startDir = %q, want the home folder", got)
	}
	if got := startDir("/gone", exists, "", ""); got != "" {
		t.Errorf("startDir with nothing available = %q, want empty", got)
	}
}

// The real one, against the filesystem: a config pointing at a folder that no
// longer exists must not reach a dialog.
func TestLastDirIgnoresADeletedRememberedFolder(t *testing.T) {
	dir := t.TempDir()
	cfg, _, err := config.NewStoreAt(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatalf("config store: %v", err)
	}
	gone := filepath.Join(dir, "was-here-yesterday")
	if err := os.MkdirAll(gone, 0o755); err != nil {
		t.Fatal(err)
	}
	c := cfg.Get()
	c.ExportDir = gone
	if _, err := cfg.Save(c); err != nil {
		t.Fatalf("save config: %v", err)
	}

	a := NewApp()
	a.cfg = cfg
	if got := a.lastDir(); got != gone {
		t.Fatalf("lastDir with the folder present = %q, want %q", got, gone)
	}

	// The user moves or deletes it, then clicks 导入文件夹 again.
	if err := os.RemoveAll(gone); err != nil {
		t.Fatal(err)
	}
	got := a.lastDir()
	if got == gone {
		t.Fatalf("lastDir still points at the deleted folder %q", got)
	}
	if !isDir(got) {
		t.Fatalf("lastDir = %q, which is not an existing folder", got)
	}
}
