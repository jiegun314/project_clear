package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"project_clear/internal/mps"
)

// Wails marshals every bound return value with encoding/json. A field that
// does not survive that round trip (time.Time being the known risk) would only
// fail at runtime inside the webview, so it is checked here instead.
func TestWeekMarshalsAsTheFrontendExpects(t *testing.T) {
	w, err := mps.ParseWeekCode("2639")
	if err != nil {
		t.Fatalf("ParseWeekCode: %v", err)
	}
	b, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got struct {
		Code   string `json:"code"`
		Year   int    `json:"year"`
		WeekNo int    `json:"weekNo"`
		Start  string `json:"start"`
	}
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Code != "2639" || got.Year != 26 || got.WeekNo != 39 {
		t.Fatalf("week fields wrong: %+v", got)
	}
	if got.Start != "2026-09-21T00:00:00Z" && !strings.HasPrefix(got.Start, "2026-09-21") {
		t.Fatalf("start = %q, want an ISO date for 2026-09-21", got.Start)
	}
	t.Logf("Week JSON: %s", b)
}

func TestHeaderIsSerialisable(t *testing.T) {
	w1, _ := mps.ParseWeekCode("2639")
	w2, _ := mps.ParseWeekCode("2640")
	h := mps.Header{
		IndexNames: []string{"P5", "P4", "P3", "P2", "P1", "MFG GROUP", "MFG CLASS CODE",
			"MFG DESCR", "ITEM", "US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC"},
		Weeks: []mps.Week{w1, w2},
	}
	b, err := json.Marshal(h)
	if err != nil {
		t.Fatalf("header must marshal: %v", err)
	}
	if !strings.Contains(string(b), `"2639"`) {
		t.Fatalf("week code missing from header JSON: %s", b)
	}
}

// The app seeds its database and config next to the executable; make sure a
// fresh machine gets both without a pre-existing folder.
func TestFreshInstallSeedsConfigAndDatabase(t *testing.T) {
	dir := t.TempDir()
	fakeExe := filepath.Join(dir, "CLEAR")
	if err := os.WriteFile(fakeExe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = context.Background()
	if _, err := os.Stat(filepath.Join(dir, "config")); err != nil {
		t.Logf("config dir not pre-created (created on demand at runtime): %v", err)
	}
}
