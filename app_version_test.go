package main

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The version is written by hand in five places, and nothing tied them together:
// a missed bump shipped a package whose own file name, About box and lock file
// disagreed, and the release notes in the README pointed at a file that was not
// produced. This pins all of them to AppVersion.
func TestVersionAgreesEverywhere(t *testing.T) {
	var wailsJSON struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	readJSONFile(t, "wails.json", &wailsJSON)
	if got := wailsJSON.Info.ProductVersion; got != AppVersion {
		t.Errorf("wails.json info.productVersion = %q, AppVersion = %q", got, AppVersion)
	}

	var pkgJSON struct {
		Version string `json:"version"`
	}
	readJSONFile(t, filepath.Join("frontend", "package.json"), &pkgJSON)
	if got := pkgJSON.Version; got != AppVersion {
		t.Errorf("frontend/package.json version = %q, AppVersion = %q", got, AppVersion)
	}

	// The lock file carries the version twice: at the top level and for the root
	// package entry. npm keeps both in step with package.json.
	var lockJSON struct {
		Version  string `json:"version"`
		Packages map[string]struct {
			Version string `json:"version"`
		} `json:"packages"`
	}
	readJSONFile(t, filepath.Join("frontend", "package-lock.json"), &lockJSON)
	if got := lockJSON.Version; got != AppVersion {
		t.Errorf("frontend/package-lock.json version = %q, AppVersion = %q", got, AppVersion)
	}
	if root, ok := lockJSON.Packages[""]; ok && root.Version != AppVersion {
		t.Errorf("frontend/package-lock.json packages[\"\"].version = %q, AppVersion = %q",
			root.Version, AppVersion)
	}

	// The README names the archive the packaging script produces.
	raw, err := os.ReadFile("README.md")
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	want := "CLEAR-" + AppVersion + "-darwin-arm64.zip"
	if !strings.Contains(string(raw), want) {
		t.Errorf("README.md does not name the archive %q this version produces", want)
	}

	// Wails stores md5(package.json) to decide whether the frontend dependencies
	// need reinstalling. Bump package.json without refreshing it and the next
	// build reinstalls against a stale expectation.
	pkgRaw, err := os.ReadFile(filepath.Join("frontend", "package.json"))
	if err != nil {
		t.Fatalf("read frontend/package.json: %v", err)
	}
	stampRaw, err := os.ReadFile(filepath.Join("frontend", "package.json.md5"))
	if err != nil {
		t.Fatalf("read frontend/package.json.md5: %v", err)
	}
	wantStamp := fmt.Sprintf("%x", md5.Sum(pkgRaw))
	if got := strings.TrimSpace(string(stampRaw)); got != wantStamp {
		t.Errorf("frontend/package.json.md5 = %q, want md5(package.json) = %q\n"+
			"(the stamp is the hash of the file as it sits on disk, so a checkout that "+
			"rewrote its line endings cannot match — see .gitattributes)", got, wantStamp)
	}
}

func readJSONFile(t *testing.T, path string, dst any) {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	if err := json.Unmarshal(raw, dst); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
}
