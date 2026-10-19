package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"project_clear/internal/config"
	"project_clear/internal/logging"
	"project_clear/internal/mps"
	"project_clear/internal/store"

	"github.com/xuri/excelize/v2"
)

var testIndexNames = []string{
	"P5", "P4", "P3", "P2", "P1", "MFG GROUP", "MFG CLASS CODE", "MFG DESCR",
	"ITEM", "US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC",
}

func newTestService(t *testing.T) *Service {
	t.Helper()
	dir := t.TempDir()
	cfg, _, err := config.NewStoreAt(filepath.Join(dir, "config"))
	if err != nil {
		t.Fatal(err)
	}
	log := logging.New(filepath.Join(dir, "logs"))
	db, err := store.Open(filepath.Join(dir, "clear.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return New(cfg, log, db, dir)
}

func testRow(item, value string) store.StagedRow {
	var idx [mps.IndexCols]string
	copy(idx[:], testIndexNames)
	idx[8] = item
	return store.StagedRow{
		FileName: "a.xlsm", FilePath: "/in/a.xlsm", SourceRow: 4,
		Index: idx, Weeks: []string{value},
	}
}

func addGlobal(t *testing.T, s *Service, rows ...store.StagedRow) *store.Dataset {
	t.Helper()
	d, err := s.db.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	files := []store.StagedFile{{Path: "/in/a.xlsm", Name: "a.xlsm", Status: "ok", RowsTotal: len(rows), RowsKept: len(rows), WeekCode: "2639"}}
	if err := s.db.AddDraftData(d.ID, files, rows, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	ready, err := s.db.FinishDraft(d.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	s.sessionActive = true
	s.activeWeek = "2639"
	return ready
}

func addGlobalFrom(t *testing.T, s *Service, baseID int64, rows ...store.StagedRow) *store.Dataset {
	t.Helper()
	d, err := s.db.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", baseID)
	if err != nil {
		t.Fatal(err)
	}
	files := []store.StagedFile{{Path: "/in/b.xlsm", Name: "b.xlsm", Status: "ok", RowsTotal: len(rows), RowsKept: len(rows), WeekCode: "2639"}}
	if err := s.db.AddDraftData(d.ID, files, rows, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	ready, err := s.db.FinishDraft(d.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	s.sessionActive = true
	s.activeWeek = "2639"
	return ready
}

func addLocal(t *testing.T, s *Service, rows ...store.StagedRow) *store.Dataset {
	t.Helper()
	files := []store.StagedFile{{Path: "/in/local.xlsx", Name: "local.xlsx", Status: "ok", RowsTotal: len(rows), RowsKept: len(rows), WeekCode: "2639"}}
	d, err := s.db.CreateLocalDataset("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", files, rows, nil)
	if err != nil {
		t.Fatal(err)
	}
	s.sessionActive = true
	s.activeWeek = "2639"
	return d
}

func TestServiceFinishIntegrateAndExpiry(t *testing.T) {
	s := newTestService(t)
	d, err := s.db.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.AddDraftData(d.ID, []store.StagedFile{{Path: "/in/a.xlsm", Name: "a.xlsm", Status: "ok", RowsKept: 2, WeekCode: "2639"}},
		[]store.StagedRow{testRow("A", "100"), testRow("B", "200")}, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	fin, err := s.FinishGlobal(false)
	if err != nil {
		t.Fatal(err)
	}
	if fin.Version != 1 || fin.RowCount != 2 || fin.Expired {
		t.Fatalf("finish = %+v", fin)
	}
	addLocal(t, s, testRow("B", "250"), testRow("C", "300"))
	// The file list is Global-only; Local is one workbook and is reported in the
	// status strip, not in this list.
	files, err := s.Files()
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Dataset != "Global" || files[0].Name != "a.xlsm" {
		t.Fatalf("files = %+v, want only the Global workbook", files)
	}
	res, had, wasExpired, err := s.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if had || wasExpired {
		t.Fatalf("had/wasExpired = %v/%v", had, wasExpired)
	}
	if res.Merged.RowCount != 3 {
		t.Fatalf("merged rows = %d", res.Merged.RowCount)
	}
	archive, err := s.Archive()
	if err != nil || len(archive) != 3 {
		t.Fatalf("archive = %+v %v", archive, err)
	}
	for _, e := range archive {
		if e.Expired {
			t.Fatalf("fresh integration marked expired: %+v", e)
		}
	}

	// A new Global version is a completed import the integration does not point
	// at: the workspace and the archive both have to say expired.
	v2 := addGlobalFrom(t, s, res.Global.ID, testRow("D", "400"))
	ws, err := s.Workspace()
	if err != nil {
		t.Fatal(err)
	}
	if !ws.Expired || !ws.Unintegrated {
		t.Fatalf("workspace = %+v", ws)
	}
	archive, _ = s.Archive()
	byKind := map[string]ArchiveEntry{}
	for _, e := range archive {
		byKind[e.Dataset] = e
	}
	if !byKind[store.DatasetMerged].Expired {
		t.Fatalf("integration should be expired: %+v", byKind[store.DatasetMerged])
	}
	if byKind[store.DatasetGlobal].Expired || byKind[store.DatasetLocal].Expired {
		t.Fatalf("inputs must not be expired: %+v", archive)
	}
	if byKind[store.DatasetGlobal].DatasetID != v2.ID || byKind[store.DatasetGlobal].Version != v2.Version {
		t.Fatalf("global row = %+v, want current v%d", byKind[store.DatasetGlobal], v2.Version)
	}
	if byKind[store.DatasetLocal].DatasetID != res.Local.ID {
		t.Fatalf("local row = %+v, want unchanged %d", byKind[store.DatasetLocal], res.Local.ID)
	}
	if src, _ := s.GridSource(v2.ID); src.Expired {
		t.Fatalf("current Global grid source marked expired: %+v", src)
	}
	if src, _ := s.GridSource(res.Integration.MergedID); !src.Expired {
		t.Fatalf("expired integration grid source not marked: %+v", src)
	}

	// Re-integrating folds the new Global in and clears the expiry.
	res, had, wasExpired, err = s.Commit()
	if err != nil {
		t.Fatal(err)
	}
	if !had || !wasExpired || res.Merged.RowCount != 4 {
		t.Fatalf("re-integrate = %+v had=%v expired=%v", res, had, wasExpired)
	}
	if ws, _ := s.Workspace(); ws.Expired {
		t.Fatalf("workspace still expired: %+v", ws)
	}
}

func TestLocalCanStartANewWeekBeforeItsGlobal(t *testing.T) {
	s := newTestService(t)
	addGlobal(t, s, testRow("A", "100"))
	addLocal(t, s, testRow("B", "200"))
	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	// Week 2640's Local arrives before its Global; the integrated 2639 must not
	// stand in the way, and the Local becomes the active week.
	local, err := s.db.CreateLocalDataset("2640", "2026-09-28", []string{"2640"}, testIndexNames, "",
		[]store.StagedFile{{Path: "/in/local.xlsx", Name: "local.xlsx", Status: "ok", RowsKept: 2, WeekCode: "2640"}},
		[]store.StagedRow{testRow("B", "250"), testRow("C", "300")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	// ImportLocal would switch the workbench to the new week; this test writes
	// the Local dataset directly, so do the same here.
	s.sessionActive = true
	s.activeWeek = "2640"
	ws, err := s.Workspace()
	if err != nil {
		t.Fatal(err)
	}
	if ws.Week != "2640" || ws.Global != nil || ws.Local == nil || ws.Local.ID != local.ID || !ws.Unintegrated {
		t.Fatalf("workspace = %+v", ws)
	}
	// The older integration is intact and still current for its own week.
	in, _ := s.db.Integration("2639")
	if in == nil || in.MergedID == 0 {
		t.Fatalf("2639 integration = %+v", in)
	}
	if g, _ := s.db.Current("2639", store.DatasetGlobal); g == nil {
		t.Fatal("2639 global disappeared")
	}
}

// Completing a Global import or importing a Local file is a finished dataset and
// must appear in history immediately, before the week is integrated.
func TestArchiveShowsCompletedImportsBeforeIntegration(t *testing.T) {
	s := newTestService(t)
	g := addGlobal(t, s, testRow("A", "100"))
	list, err := s.Archive()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Dataset != store.DatasetGlobal || list[0].DatasetID != g.ID {
		t.Fatalf("history after Global finish = %+v", list)
	}
	if list[0].Expired {
		t.Fatalf("unintegrated Global marked expired: %+v", list[0])
	}
	weeks, err := s.db.WeeksWithReadyData()
	if err != nil || len(weeks) != 1 || weeks[0] != "2639" {
		t.Fatalf("ready weeks = %v %v", weeks, err)
	}
	if ys, _ := s.Years(); len(ys) != 1 || ys[0] != 2026 {
		t.Fatalf("years = %v", ys)
	}
	if ws, _ := s.Weeks(2026); len(ws) != 1 || ws[0] != 39 {
		t.Fatalf("weeks = %v", ws)
	}

	l := addLocal(t, s, testRow("B", "200"))
	list, err = s.Archive()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("history after Local import = %+v", list)
	}
	seen := map[string]ArchiveEntry{}
	for _, e := range list {
		seen[e.Dataset] = e
	}
	if seen[store.DatasetGlobal].DatasetID != g.ID || seen[store.DatasetLocal].DatasetID != l.ID {
		t.Fatalf("history entries = %+v", seen)
	}
	for _, e := range list {
		if e.Expired {
			t.Fatalf("pre-integration entry expired: %+v", e)
		}
	}

	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	list, err = s.Archive()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 3 || list[0].Dataset != store.DatasetMerged {
		t.Fatalf("history after integration = %+v", list)
	}
}

// A fresh launch must not reopen last session's imports, but history stays.
func TestStartupWorkbenchIsBlankAndHistoryRemains(t *testing.T) {
	s := newTestService(t)
	addGlobal(t, s, testRow("A", "100"))
	addLocal(t, s, testRow("B", "200"))
	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	if ws, _ := s.Workspace(); ws.Week != "2639" {
		t.Fatalf("active session = %+v", ws)
	}

	fresh := New(s.cfg, s.log, s.db, s.dataDir)
	ws, err := fresh.Workspace()
	if err != nil {
		t.Fatal(err)
	}
	if ws.Week != "" || ws.Global != nil || ws.Local != nil || ws.Integration != nil || ws.Draft != nil {
		t.Fatalf("fresh workbench = %+v, want blank", ws)
	}
	files, err := fresh.Files()
	if err != nil || len(files) != 0 {
		t.Fatalf("fresh files = %+v %v", files, err)
	}
	if got := fresh.ExportWeek(""); got != "" {
		t.Fatalf("fresh export week = %q, want empty", got)
	}
	archive, err := fresh.Archive()
	if err != nil || len(archive) != 3 {
		t.Fatalf("history = %+v %v", archive, err)
	}
}

// The history's Global and Local rows are the current datasets; only the merged
// row is the integration and can be expired.
func TestArchiveUpdatesOnlyTheReimportedInput(t *testing.T) {
	s := newTestService(t)
	g := addGlobal(t, s, testRow("A", "100"))
	l := addLocal(t, s, testRow("B", "200"))
	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}

	g2 := addGlobalFrom(t, s, g.ID, testRow("C", "300"))
	byKind := func() map[string]ArchiveEntry {
		list, err := s.Archive()
		if err != nil {
			t.Fatal(err)
		}
		out := map[string]ArchiveEntry{}
		for _, e := range list {
			out[e.Dataset] = e
		}
		return out
	}
	got := byKind()
	if !got[store.DatasetMerged].Expired {
		t.Fatalf("integration not expired after a Global re-import: %+v", got)
	}
	if got[store.DatasetGlobal].Expired || got[store.DatasetGlobal].DatasetID != g2.ID {
		t.Fatalf("global row = %+v, want current %d and not expired", got[store.DatasetGlobal], g2.ID)
	}
	if got[store.DatasetLocal].Expired || got[store.DatasetLocal].DatasetID != l.ID {
		t.Fatalf("local row = %+v, want unchanged %d and not expired", got[store.DatasetLocal], l.ID)
	}

	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	l2 := addLocal(t, s, testRow("D", "400"))
	got = byKind()
	if !got[store.DatasetMerged].Expired {
		t.Fatalf("integration not expired after a Local re-import: %+v", got)
	}
	if got[store.DatasetGlobal].Expired || got[store.DatasetGlobal].DatasetID != g2.ID {
		t.Fatalf("global row = %+v, want %d and not expired", got[store.DatasetGlobal], g2.ID)
	}
	if got[store.DatasetLocal].Expired || got[store.DatasetLocal].DatasetID != l2.ID {
		t.Fatalf("local row = %+v, want current %d and not expired", got[store.DatasetLocal], l2.ID)
	}
}

func TestDiscardWeekImportsKeepsHistory(t *testing.T) {
	s := newTestService(t)
	addGlobal(t, s, testRow("A", "100"))
	addLocal(t, s, testRow("B", "200"))
	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	addGlobal(t, s, testRow("C", "300"))
	if _, err := s.DiscardWeekImports("2639"); err != nil {
		t.Fatal(err)
	}
	ws, err := s.Workspace()
	if err != nil {
		t.Fatal(err)
	}
	if ws.Expired || ws.Unintegrated || ws.Integration == nil {
		t.Fatalf("workspace = %+v", ws)
	}
	archive, _ := s.Archive()
	if len(archive) != 3 {
		t.Fatalf("history = %+v", archive)
	}
}

func TestExportBundleWritesThreeSheets(t *testing.T) {
	s := newTestService(t)
	addGlobal(t, s, testRow("A", "100"), testRow("B", "200"))
	addLocal(t, s, testRow("B", "250"), testRow("C", "300"))
	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "bundle.xlsx")
	stats, err := s.ExportBundle(BundleExportOptions{WeekCode: "2639", DestPath: dest}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Sheets != 3 || stats.Expired {
		t.Fatalf("stats = %+v", stats)
	}
	f, err := excelize.OpenFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if got := f.GetSheetList(); len(got) != 3 || got[0] != "MPS" || got[1] != "Local" || got[2] != "Global" {
		t.Fatalf("sheets = %v", got)
	}
}

func TestExportSingleDataset(t *testing.T) {
	s := newTestService(t)
	g := addGlobal(t, s, testRow("A", "100"), testRow("B", "200"))
	tplPath := filepath.Join(t.TempDir(), "template.xlsx")
	tpl := excelize.NewFile()
	_ = tpl.SetSheetName(tpl.GetSheetList()[0], "MPS")
	if err := tpl.SaveAs(tplPath); err != nil {
		t.Fatal(err)
	}
	if err := s.storeTemplate("2639", tplPath); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(t.TempDir(), "global.xlsx")
	stats, err := s.Export(ExportOptions{DatasetID: g.ID, DestPath: dest, Mode: config.ExportClean}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Rows != 2 || !fileExists(dest) {
		t.Fatalf("stats = %+v", stats)
	}
}

func TestExportBundlePreservesColorsAndComments(t *testing.T) {
	s := newTestService(t)
	dict := mps.NewStyleDict()
	styleID, err := dict.InternStyle(&excelize.Style{
		Font: &excelize.Font{Size: 9, Bold: true},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"00FF00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	row := testRow("A", "100")
	row.StyleIDs = make([]int, mps.IndexCols+1)
	for i := range row.StyleIDs {
		row.StyleIDs[i] = styleID
	}
	row.Comments = mps.CommentMap{16: {Text: "备注", Author: "me"}}
	row.CFColors = []string{"#FF0000"}

	d, err := s.db.BeginGlobalImport("2639", "2026-09-21", []string{"2639"}, testIndexNames, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.db.AddDraftData(d.ID, []store.StagedFile{{Path: "/in/a.xlsm", Name: "a.xlsm", Status: "ok", RowsKept: 1, WeekCode: "2639"}},
		[]store.StagedRow{row}, nil, dict, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.FinishGlobal(false); err != nil {
		t.Fatal(err)
	}
	addLocal(t, s, testRow("B", "200"))
	if _, _, _, err := s.Commit(); err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(t.TempDir(), "bundle.xlsx")
	if _, err := s.ExportBundle(BundleExportOptions{WeekCode: "2639", DestPath: dest}, nil); err != nil {
		t.Fatal(err)
	}
	f, err := excelize.OpenFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	for _, sheet := range []string{"MPS", "Global"} {
		style, err := f.GetCellStyle(sheet, "P4")
		if err != nil {
			t.Fatal(err)
		}
		st, err := f.GetStyle(style)
		if err != nil {
			t.Fatal(err)
		}
		if len(st.Fill.Color) == 0 || !strings.Contains(strings.ToUpper(st.Fill.Color[0]), "FF0000") {
			t.Fatalf("%s P4 fill = %+v, want the conditional red", sheet, st.Fill)
		}
		static, err := f.GetCellStyle(sheet, "A4")
		if err != nil {
			t.Fatal(err)
		}
		sst, err := f.GetStyle(static)
		if err != nil {
			t.Fatal(err)
		}
		if len(sst.Fill.Color) == 0 || !strings.Contains(strings.ToUpper(sst.Fill.Color[0]), "00FF00") {
			t.Fatalf("%s A4 fill = %+v, want the source green", sheet, sst.Fill)
		}
	}
	comments, err := f.GetComments("MPS")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range comments {
		if c.Cell == "P4" && c.Text == "备注" {
			found = true
		}
	}
	if !found {
		t.Fatalf("MPS comments = %+v, want P4 备注", comments)
	}
}

func TestExportLocalOnlyUsesBlankTemplate(t *testing.T) {
	s := newTestService(t)
	l := addLocal(t, s, testRow("A", "100"), testRow("B", "200"))
	for _, mode := range []config.ExportMode{config.ExportClean, config.ExportTemplate} {
		dest := filepath.Join(t.TempDir(), string(mode)+".xlsx")
		stats, err := s.Export(ExportOptions{DatasetID: l.ID, DestPath: dest, Mode: mode}, nil)
		if err != nil {
			t.Fatalf("%s export: %v", mode, err)
		}
		if stats.Rows != 2 || !fileExists(dest) {
			t.Fatalf("%s stats = %+v", mode, stats)
		}
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
