package store

import "testing"

// Committing copies the staged rows into the week table. Keeping the staging
// copy as well put a second, full copy of every integrated week in the database
// and grew it by roughly double each week, for rows nothing ever read again.
func TestCommitDropsTheRedundantStagingRows(t *testing.T) {
	st := openTestStore(t)
	if _, err := st.SaveStaging(stagedRowWithStyle(t)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	if _, err := st.Commit("2639"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	// The week table is the surviving copy and still reads back.
	gr, err := st.QueryRows(Query{Source: "2639", Page: 1, PageSize: 10, SortField: "seq"})
	if err != nil {
		t.Fatalf("query committed week: %v", err)
	}
	assertWeekCells(t, gr.Rows)

	var staged int
	if err := st.db.QueryRow(
		`SELECT COUNT(1) FROM stg_row WHERE batch_id=(SELECT id FROM batch WHERE status='committed')`).
		Scan(&staged); err != nil {
		t.Fatalf("count staging rows: %v", err)
	}
	if staged != 0 {
		t.Errorf("a committed batch kept %d staging rows", staged)
	}

	// The per-file record is what the 已导入文件 window reads after 整合, so it
	// has to survive the purge.
	files, weekCode, state, err := st.CurrentBatchFiles()
	if err != nil {
		t.Fatalf("current batch files: %v", err)
	}
	if state != "committed" || weekCode != "2639" {
		t.Errorf("batch state = %q / %q, want committed / 2639", state, weekCode)
	}
	if len(files) != 1 || files[0].Name != "a.xlsm" {
		t.Errorf("the file list did not survive the commit: %+v", files)
	}
}

// A staging area that is superseded by the next import takes its export template
// with it. A template row has no foreign key to batch, so the delete cascade
// never reached it and the week's copy stayed behind for good.
func TestSaveStagingDropsTheSupersededWeeksTemplate(t *testing.T) {
	st := openTestStore(t)

	first := stagedBatch(nil)
	first.TemplateSrc = "/src/first.xlsm"
	first.TemplateNam = "first.xlsm"
	if _, err := st.SaveStaging(first); err != nil {
		t.Fatalf("first staging: %v", err)
	}

	second := stagedBatch(nil)
	second.WeekCode = "2640"
	second.WeekCodes = []string{"2640"}
	second.TemplateSrc = "/src/second.xlsm"
	second.TemplateNam = "second.xlsm"
	res, err := st.SaveStaging(second)
	if err != nil {
		t.Fatalf("second staging: %v", err)
	}

	if len(res.OrphanedTemplates) != 1 || res.OrphanedTemplates[0] != "2639" {
		t.Errorf("orphaned templates = %v, want [2639]", res.OrphanedTemplates)
	}
	if tpl, err := st.TemplateFor("2639"); err != nil || tpl != nil {
		t.Errorf("the superseded template row survived: %+v (err %v)", tpl, err)
	}
	if tpl, err := st.TemplateFor("2640"); err != nil || tpl == nil {
		t.Errorf("the incoming template row is missing: %+v (err %v)", tpl, err)
	}
}

// Re-importing the same week must not delete its own template.
func TestSaveStagingKeepsTheTemplateOfTheWeekItWrites(t *testing.T) {
	st := openTestStore(t)

	first := stagedBatch(nil)
	first.TemplateSrc = "/src/first.xlsm"
	first.TemplateNam = "first.xlsm"
	if _, err := st.SaveStaging(first); err != nil {
		t.Fatalf("first staging: %v", err)
	}

	second := stagedBatch(nil)
	second.TemplateSrc = "/src/first.xlsm"
	second.TemplateNam = "first.xlsm"
	res, err := st.SaveStaging(second)
	if err != nil {
		t.Fatalf("second staging: %v", err)
	}
	if len(res.OrphanedTemplates) != 0 {
		t.Errorf("re-importing a week orphaned its own template: %v", res.OrphanedTemplates)
	}
	if tpl, err := st.TemplateFor("2639"); err != nil || tpl == nil {
		t.Errorf("the week's template is missing: %+v (err %v)", tpl, err)
	}
}

// An integrated week still needs its original workbook to export from, so the
// template has to outlive the staging area that produced it.
func TestSupersedingKeepsACommittedWeeksTemplate(t *testing.T) {
	st := openTestStore(t)

	first := stagedBatch(nil)
	first.TemplateSrc = "/src/first.xlsm"
	first.TemplateNam = "first.xlsm"
	if _, err := st.SaveStaging(first); err != nil {
		t.Fatalf("first staging: %v", err)
	}
	if _, err := st.Commit("2639"); err != nil {
		t.Fatalf("commit: %v", err)
	}

	second := stagedBatch(nil)
	second.WeekCode = "2640"
	second.WeekCodes = []string{"2640"}
	second.TemplateSrc = "/src/second.xlsm"
	second.TemplateNam = "second.xlsm"
	res, err := st.SaveStaging(second)
	if err != nil {
		t.Fatalf("second staging: %v", err)
	}

	if len(res.OrphanedTemplates) != 0 {
		t.Errorf("a committed week's template was orphaned: %v", res.OrphanedTemplates)
	}
	if tpl, err := st.TemplateFor("2639"); err != nil || tpl == nil {
		t.Errorf("the committed week lost its template: %+v (err %v)", tpl, err)
	}
}
