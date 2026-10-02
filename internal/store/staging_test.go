package store

import (
	"testing"

	"project_clear/internal/mps"
)

// countStagedRows counts the rows a table holds for the active staging batch.
// Committed batches keep their own copies of the records they were built from,
// so a plain table count would also see that older, already-integrated data.
func countStagedRows(t *testing.T, st *Store, table string) int {
	t.Helper()
	var n int
	q := `SELECT COUNT(1) FROM ` + table +
		` WHERE batch_id IN (SELECT id FROM batch WHERE status='staging')`
	if err := st.db.QueryRow(q).Scan(&n); err != nil {
		t.Fatalf("count %s: %v", table, err)
	}
	return n
}

// 清空 drops the imported-but-unsaved data and the export template that came
// with it, but must never touch a week that was already integrated — that
// week's table and the template its export rebuilds from both have to survive.
func TestClearStagingKeepsCommittedWeeks(t *testing.T) {
	st := openTestStore(t)

	// A week the user integrated, together with its stored template copy.
	first := stagedBatch(mps.CommentMap{})
	first.TemplateSrc = "/src/2639/source.xlsm"
	first.TemplateNam = "source.xlsm"
	if _, err := st.SaveStaging(first); err != nil {
		t.Fatalf("save first staging: %v", err)
	}
	if _, err := st.Commit("2639"); err != nil {
		t.Fatalf("commit 2639: %v", err)
	}

	// A later import the user never integrated.
	second := stagedBatch(nil)
	second.WeekCode = "2640"
	second.WeekStart = "2026-09-28"
	second.WeekCodes = []string{"2640"}
	second.Files[0].WeekCode = "2640"
	second.Rows[0].Weeks = []string{"130"}
	second.TemplateSrc = "/src/2640/source.xlsm"
	second.TemplateNam = "source.xlsm"
	if _, err := st.SaveStaging(second); err != nil {
		t.Fatalf("save second staging: %v", err)
	}

	tpl, err := st.TemplateFor("2640")
	if err != nil || tpl == nil {
		t.Fatalf("template for the staged week missing: %+v %v", tpl, err)
	}

	orphaned, err := st.ClearStaging()
	if err != nil {
		t.Fatalf("clear staging: %v", err)
	}
	if len(orphaned) != 1 || orphaned[0] != "2640" {
		t.Fatalf("orphaned = %v, want [2640]", orphaned)
	}

	sum, err := st.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if sum.HasStaging {
		t.Fatalf("staging survived the clear: %+v", sum)
	}
	if n := countStagedRows(t, st, "stg_row"); n != 0 {
		t.Errorf("stg_row rows = %d, want 0", n)
	}
	if n := countStagedRows(t, st, "batch_file"); n != 0 {
		t.Errorf("batch_file rows = %d, want 0", n)
	}
	if tpl, err := st.TemplateFor("2640"); err != nil || tpl != nil {
		t.Errorf("uncommitted template survived: %+v %v", tpl, err)
	}

	// The integrated week is whole: archive entry, weekly table and template.
	entry, err := st.ArchiveEntryFor("2639")
	if err != nil || entry == nil {
		t.Fatalf("committed week lost: %+v %v", entry, err)
	}
	_, rows, err := st.FetchExportRows("2639")
	if err != nil {
		t.Fatalf("fetch committed rows: %v", err)
	}
	if len(rows) != 1 {
		t.Errorf("committed rows = %d, want 1", len(rows))
	}
	if tpl, err := st.TemplateFor("2639"); err != nil || tpl == nil {
		t.Errorf("committed template lost: %+v %v", tpl, err)
	}
}

// Clearing an empty staging area is a no-op, not an error: the button stays
// harmless once the data is already gone.
func TestClearStagingWithoutDataIsANoOp(t *testing.T) {
	st := openTestStore(t)
	orphaned, err := st.ClearStaging()
	if err != nil {
		t.Fatalf("clear empty staging: %v", err)
	}
	if len(orphaned) != 0 {
		t.Fatalf("orphaned = %v, want none", orphaned)
	}
}

// 添加 appends one workbook to the list that is already staged: the previous
// files keep their rows, the new file is added after them, and the batch still
// reports the whole list.
func TestMergeStagingAppendsToTheExistingList(t *testing.T) {
	st := openTestStore(t)

	first := stagedBatch(nil)
	if _, err := st.SaveStaging(first); err != nil {
		t.Fatalf("save first staging: %v", err)
	}

	second := stagedBatch(nil)
	second.Action = "add"
	second.Files[0] = StagedFile{
		Path: "/src/b.xlsm", Name: "b.xlsm", Size: 200, Status: "ok",
		RowsTotal: 2, RowsKept: 2, WeekCode: "2639",
	}
	second.Rows[0].FileName = "b.xlsm"
	second.Rows[0].FilePath = "/src/b.xlsm"
	second.Rows[0].Index[0] = "P5_EP_B"
	extra := second.Rows[0]
	extra.SourceRow = mps.FirstDataRow + 1
	extra.Index[0] = "P5_EP_B2"
	second.Rows = append(second.Rows, extra)
	if _, err := st.MergeStaging(second, nil); err != nil {
		t.Fatalf("merge staging: %v", err)
	}

	files, err := st.StagingFiles()
	if err != nil {
		t.Fatalf("staging files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("staging files = %d (%+v), want 2", len(files), files)
	}
	sum, err := st.LoadStaging()
	if err != nil {
		t.Fatalf("load staging: %v", err)
	}
	if sum.FileOK != 2 || sum.RowKept != 3 {
		t.Errorf("batch summary = %d files / %d rows, want 2 / 3", sum.FileOK, sum.RowKept)
	}

	grid, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 20, SortField: "seq"})
	if err != nil {
		t.Fatalf("query staging: %v", err)
	}
	if grid.Total != 3 {
		t.Fatalf("staged rows = %d, want 3", grid.Total)
	}
	// The first file keeps sequence 0; the added rows continue behind it.
	if got := grid.Rows[0].FileName; got != "a.xlsm" {
		t.Errorf("first row belongs to %q, want a.xlsm", got)
	}
	if got := grid.Rows[2].Index[0]; got != "P5_EP_B2" {
		t.Errorf("last row = %q, want P5_EP_B2", got)
	}
}

// Re-adding a workbook that is already in the list replaces exactly its own
// rows: the file count does not grow and the other files stay intact.
func TestMergeStagingReplacesAReAddedFile(t *testing.T) {
	st := openTestStore(t)

	first := stagedBatch(nil)
	first.Rows[0].FilePath = "/src/a.xlsm"
	if _, err := st.SaveStaging(first); err != nil {
		t.Fatalf("save first staging: %v", err)
	}

	second := stagedBatch(nil)
	second.Action = "add"
	second.Files[0] = StagedFile{
		Path: "/src/b.xlsm", Name: "b.xlsm", Size: 200, Status: "ok",
		RowsTotal: 1, RowsKept: 1, WeekCode: "2639",
	}
	second.Rows[0].FileName = "b.xlsm"
	second.Rows[0].FilePath = "/src/b.xlsm"
	second.Rows[0].Index[0] = "P5_EP_B"
	if _, err := st.MergeStaging(second, nil); err != nil {
		t.Fatalf("merge second file: %v", err)
	}

	replacement := stagedBatch(nil)
	replacement.Action = "add"
	replacement.Rows[0].FilePath = "/src/a.xlsm"
	replacement.Rows[0].Index[0] = "P5_EP_A2"
	replacement.Rows[0].Weeks = []string{"999"}
	if _, err := st.MergeStaging(replacement, []string{"/src/a.xlsm"}); err != nil {
		t.Fatalf("replace a.xlsm: %v", err)
	}

	files, err := st.StagingFiles()
	if err != nil {
		t.Fatalf("staging files: %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("staging files = %d (%+v), want 2 (同一个文件不得重复添加)", len(files), files)
	}
	grid, err := st.QueryRows(Query{Source: "", Page: 1, PageSize: 20, SortField: "seq"})
	if err != nil {
		t.Fatalf("query staging: %v", err)
	}
	if grid.Total != 2 {
		t.Fatalf("staged rows = %d, want 2 (旧数据必须被覆盖而不是累加)", grid.Total)
	}
	got := map[string]string{}
	for _, r := range grid.Rows {
		got[r.FileName] = r.Index[0]
	}
	if got["a.xlsm"] != "P5_EP_A2" {
		t.Errorf("a.xlsm = %q, want the re-added P5_EP_A2", got["a.xlsm"])
	}
	if got["b.xlsm"] != "P5_EP_B" {
		t.Errorf("b.xlsm = %q, want it untouched (P5_EP_B)", got["b.xlsm"])
	}

	// 清空 empties the list the button shows.
	if _, err := st.ClearStaging(); err != nil {
		t.Fatalf("clear staging: %v", err)
	}
	files, err = st.StagingFiles()
	if err != nil {
		t.Fatalf("staging files after clear: %v", err)
	}
	if len(files) != 0 {
		t.Errorf("staging files after clear = %+v, want empty", files)
	}
}

// 已导入文件列表在整合之后仍保留为这一批的记录，点清空才清空；清空不会动
// 已整合入库的数据。
func TestCurrentBatchFilesKeepsTheRecordAfterCommit(t *testing.T) {
	st := openTestStore(t)

	if files, _, _, err := st.CurrentBatchFiles(); err != nil || len(files) != 0 {
		t.Fatalf("空库的文件列表 = %+v (%v), want empty", files, err)
	}

	if _, err := st.SaveStaging(stagedBatch(nil)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	files, code, state, err := st.CurrentBatchFiles()
	if err != nil {
		t.Fatalf("current batch files: %v", err)
	}
	if len(files) != 1 || code != "2639" || state != "staging" {
		t.Fatalf("导入后列表 = %d 个文件 / 周码 %s / %s，want 1 / 2639 / staging", len(files), code, state)
	}

	// 整合之后这一批不再是 staging，但它仍然是界面上的记录。
	if _, err := st.Commit("2639"); err != nil {
		t.Fatalf("commit: %v", err)
	}
	files, code, state, err = st.CurrentBatchFiles()
	if err != nil {
		t.Fatalf("current batch files after commit: %v", err)
	}
	if len(files) != 1 || state != "committed" {
		t.Fatalf("整合后列表 = %d 个文件 / %s，want 1 / committed（这就是「显示为零」的回归点）", len(files), state)
	}
	if files[0].RowsKept == 0 {
		t.Errorf("记录里的行数丢了: %+v", files[0])
	}

	// 清空只结束列表，不动已经入库的周数据。
	if _, err := st.ClearStaging(); err != nil {
		t.Fatalf("clear: %v", err)
	}
	files, _, state, err = st.CurrentBatchFiles()
	if err != nil {
		t.Fatalf("current batch files after clear: %v", err)
	}
	if len(files) != 0 || state != "" {
		t.Fatalf("清空后列表 = %+v / %s, want empty", files, state)
	}
	entry, err := st.ArchiveEntryFor("2639")
	if err != nil || entry == nil {
		t.Fatalf("清空后已整合的周丢了: %+v %v", entry, err)
	}

	// 紧接着的下一次导入重新开始一份列表。
	next := stagedBatch(nil)
	next.WeekCode = "2640"
	next.WeekStart = "2026-09-28"
	next.WeekCodes = []string{"2640"}
	next.Files[0].WeekCode = "2640"
	next.Rows[0].Weeks = []string{"130"}
	if _, err := st.SaveStaging(next); err != nil {
		t.Fatalf("save next staging: %v", err)
	}
	files, code, state, err = st.CurrentBatchFiles()
	if err != nil {
		t.Fatalf("current batch files after next import: %v", err)
	}
	if len(files) != 1 || code != "2640" || state != "staging" {
		t.Fatalf("下一次导入后列表 = %d 个文件 / %s / %s, want 1 / 2640 / staging", len(files), code, state)
	}
}
