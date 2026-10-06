package config

import (
	"os"
	"path/filepath"
	"testing"
)

// A macOS bundle must keep its state beside the .app the user launched, not
// inside the package where nobody looks.
func TestProgramDirFrom(t *testing.T) {
	// The paths are written POSIX-style and converted below: programDirFrom goes
	// through filepath, so on Windows the answer comes back with backslashes and a
	// POSIX-only expectation would be testing the other operating system's rules.
	cases := []struct{ exe, want string }{
		{"/Applications/CLEAR.app/Contents/MacOS/CLEAR", "/Applications"},
		{"/Users/z/Downloads/CLEAR.app/Contents/MacOS/CLEAR", "/Users/z/Downloads"},
		{"/Users/z/bin/CLEAR", "/Users/z/bin"},
		{"/opt/clear/clear", "/opt/clear"},
		// A folder that merely happens to be called MacOS is not a bundle.
		{"/srv/MacOS/CLEAR", "/srv/MacOS"},
	}
	for _, c := range cases {
		exe, want := filepath.FromSlash(c.exe), filepath.FromSlash(c.want)
		if got := programDirFrom(exe); got != want {
			t.Errorf("programDirFrom(%q) = %q, want %q", exe, got, want)
		}
	}
}

// go run and wails dev build the binary into a throwaway folder, so anchoring
// state there would hand every run an empty database.
func TestIsTempDirIn(t *testing.T) {
	root := t.TempDir()
	inside := filepath.Join(root, "go-build123", "b001", "exe", "CLEAR")
	if !isTempDirIn(inside, root) {
		t.Errorf("%q should count as temporary under %q", inside, root)
	}
	if isTempDirIn("/Users/z/bin/CLEAR", root) {
		t.Errorf("a normal install directory must not count as temporary")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Moving the folders next to the app must not look like the user's database
// vanished: the first run copies the old one over, and never overwrites a
// database the new location already has.
func TestAdoptDirCopiesLegacyOnce(t *testing.T) {
	root := t.TempDir()
	legacyBase := filepath.Join(root, "legacy")
	current := filepath.Join(root, "current")
	legacy := filepath.Join(legacyBase, "data")
	writeFile(t, filepath.Join(legacy, "clear.db"), "old database")
	writeFile(t, filepath.Join(legacy, "templates", "2639", "tpl.xlsm"), "template")
	writeFile(t, filepath.Join(legacy, "logs", "clear-2026-10-02.log"), "log")

	moved, err := AdoptDir("data", "clear.db", current, []string{legacyBase})
	if err != nil {
		t.Fatalf("adopt: %v", err)
	}
	if moved != legacy {
		t.Errorf("moved = %q, want %q", moved, legacy)
	}
	for _, rel := range []string{"clear.db", filepath.Join("templates", "2639", "tpl.xlsm"), filepath.Join("logs", "clear-2026-10-02.log")} {
		if _, err := os.Stat(filepath.Join(current, rel)); err != nil {
			t.Errorf("%s was not copied: %v", rel, err)
		}
	}
	// The original stays put.
	if _, err := os.Stat(filepath.Join(legacy, "clear.db")); err != nil {
		t.Errorf("legacy folder should be left alone: %v", err)
	}

	// A second run has nothing to do.
	if again, err := AdoptDir("data", "clear.db", current, []string{legacyBase}); err != nil || again != "" {
		t.Errorf("second adopt = %q %v, want a no-op", again, err)
	}
}

func TestAdoptDirKeepsExistingCurrentDatabase(t *testing.T) {
	root := t.TempDir()
	legacyBase := filepath.Join(root, "legacy")
	current := filepath.Join(root, "current")
	writeFile(t, filepath.Join(legacyBase, "data", "clear.db"), "old database")
	writeFile(t, filepath.Join(current, "clear.db"), "current database")

	moved, err := AdoptDir("data", "clear.db", current, []string{legacyBase})
	if err != nil || moved != "" {
		t.Fatalf("adopt = %q %v, want a no-op", moved, err)
	}
	body, err := os.ReadFile(filepath.Join(current, "clear.db"))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "current database" {
		t.Errorf("current database was overwritten: %q", body)
	}
}

// With no legacy folder in sight there is nothing to adopt and no error.
func TestAdoptDirWithoutLegacy(t *testing.T) {
	root := t.TempDir()
	moved, err := AdoptDir("data", "clear.db", filepath.Join(root, "current"), []string{filepath.Join(root, "nope")})
	if err != nil || moved != "" {
		t.Fatalf("adopt = %q %v, want a no-op", moved, err)
	}
	if _, err := os.Stat(filepath.Join(root, "current")); !os.IsNotExist(err) {
		t.Errorf("an empty adoption must not create the target folder")
	}
}
