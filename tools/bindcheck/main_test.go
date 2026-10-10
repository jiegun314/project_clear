package main

import (
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"project_clear/internal/config"
	"project_clear/internal/mps"
	"project_clear/internal/store"
	"project_clear/internal/view"
)

// Wails marshals every bound return value with encoding/json. A field that does
// not survive that round trip (time.Time being the known risk) would only fail
// at runtime inside the webview, so it is checked here instead.
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

func jsonKeys(t *testing.T, v any) []string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %T: %v", v, err)
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// The frontend reads these payloads by field name. Renaming a Go field or its
// tag changes the wire format, and the page then reads undefined with nothing to
// show for it — so the key set of every bound payload is pinned here.
func TestBoundPayloadFieldNamesArePinned(t *testing.T) {
	cases := []struct {
		name string
		v    any
		keys []string
	}{
		{"AppInfo", view.AppInfo{}, []string{
			"configPath", "dataDir", "database", "fullName", "goVersion", "name", "platform", "version"}},
		{"Status", view.Status{}, []string{
			"archivedRows", "archivedWeeks", "database", "exportMode", "hasStaging", "headerDisplay",
			"lastAction", "locFilter", "pageSize", "readColumns", "stagedRows", "weekCode", "weekStart"}},
		{"CommitResult", view.CommitResult{}, []string{
			"committedAt", "fileCount", "overwrote", "rowCount", "tableName", "weekCode", "weekStart"}},
		{"StagingFilesView", view.NewStagingFilesView(), []string{
			"batchState", "failedCount", "fileCount", "files", "hasStaging", "rowCount"}},
		{"ClearStagingResult", view.ClearStagingResult{}, []string{"files", "rows", "weekCode"}},
		{"ExportResult", view.ExportResult{}, []string{
			"cfRows", "cols", "comments", "destPath", "dropped", "durationMs", "mode",
			"preservedVba", "rows", "sizeBytes", "stylesUsed"}},
		{"GridQuery", view.GridQuery{}, []string{
			"filters", "page", "pageSize", "search", "sortDesc", "sortField", "source"}},
		{"GridHeader", view.NewGridHeader(""), []string{
			"hasStaging", "indexNames", "source", "total", "weekCode", "weekStart", "weeks"}},
		{"ConfigView", view.ConfigView{}, []string{"config", "path"}},
	}

	for _, c := range cases {
		got := jsonKeys(t, c.v)
		if strings.Join(got, ",") != strings.Join(c.keys, ",") {
			t.Errorf("%s JSON keys =\n  %v\nwant\n  %v", c.name, got, c.keys)
		}
	}
}

// A nil slice becomes null, and the page walks these arrays on its first render:
// that is what produced the blank window. The payloads the grid is built from
// must therefore carry empty lists, never nil ones.
func TestGridPayloadsNeverMarshalListsAsNull(t *testing.T) {
	cases := []struct {
		name string
		v    any
	}{
		{"GridHeader", view.NewGridHeader("")},
		{"GridHeader after data", view.NewGridHeader("2639").NonNil()},
		{"StagingFilesView", view.NewStagingFilesView()},
		{"StagingFilesView after data", (&view.StagingFilesView{}).NonNil()},
	}

	for _, c := range cases {
		b, err := json.Marshal(c.v)
		if err != nil {
			t.Fatalf("marshal %s: %v", c.name, err)
		}
		if strings.Contains(string(b), "null") {
			t.Errorf("%s marshals a list as null: %s", c.name, b)
		}
	}
}

// The constructor is only worth having if it is what the grid actually gets.
func TestNewGridHeaderSeedsItsLists(t *testing.T) {
	h := view.NewGridHeader("")
	if h.IndexNames == nil || h.Weeks == nil {
		t.Fatalf("constructor left a nil list: %+v", h)
	}
	if h.Source != "" {
		t.Errorf("source = %q, want the empty staging source", h.Source)
	}
	if got := string(mustJSON(t, h)); !strings.Contains(got, `"indexNames":[]`) || !strings.Contains(got, `"weeks":[]`) {
		t.Errorf("seeded lists did not marshal as empty arrays: %s", got)
	}
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// A fresh machine has neither folder. Opening the database has to create the
// schema, or the first render asks for tables that do not exist yet.
func TestFreshDatabaseIsUsableImmediately(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "clear.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	defer st.Close()

	if _, err := st.ListArchive(); err != nil {
		t.Errorf("archive table missing on a fresh database: %v", err)
	}
	if _, err := st.LoadStaging(); err != nil {
		t.Errorf("staging table missing on a fresh database: %v", err)
	}
	if _, err := st.AvailableYears(); err != nil {
		t.Errorf("archive not queryable on a fresh database: %v", err)
	}
}

// The parameters have to be readable before anything has been saved.
func TestFreshConfigIsUsableImmediately(t *testing.T) {
	cfg, notes, err := config.NewStoreAt(filepath.Join(t.TempDir(), "config"))
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	c := cfg.Get()
	if c.ReadColumns <= 0 || c.PageSize <= 0 || c.LOCFilter == "" || c.ExportMode == "" {
		t.Errorf("defaults are not usable: %+v", c)
	}
	// Generating the file is reported, not an error.
	if len(notes) == 0 {
		t.Log("no notes on first run (acceptable if the file already existed)")
	}
}
