package store

import (
	"fmt"
	"testing"

	"project_clear/internal/mps"
)

// 查询界面（历史数据）只认已整合的周：临时数据无论有多少，都不该出现在
// 归档列表、年份或周列表里。
func TestArchiveListsKeepStagingOut(t *testing.T) {
	st := openTestStore(t)

	staged := stagedBatch(nil)
	staged.WeekCode = "2640"
	staged.WeekStart = "2026-09-28"
	staged.WeekCodes = []string{"2640"}
	staged.Files[0].WeekCode = "2640"
	staged.Rows[0].Weeks = []string{"130"}
	if _, err := st.SaveStaging(staged); err != nil {
		t.Fatalf("save staging: %v", err)
	}

	list, err := st.ListArchive()
	if err != nil {
		t.Fatalf("list archive: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("archive = %+v, 未整合的临时数据不该出现在查询界面", list)
	}
	years, err := st.AvailableYears()
	if err != nil {
		t.Fatalf("available years: %v", err)
	}
	if len(years) != 0 {
		t.Fatalf("years = %v, want none", years)
	}
	weeks, err := st.WeeksOfYear(2026)
	if err != nil {
		t.Fatalf("weeks of year: %v", err)
	}
	if len(weeks) != 0 {
		t.Fatalf("weeks = %v, want none", weeks)
	}
}

// 同一个周码再次整合：数据整体覆盖，归档里始终只有一行，整合时间换成新的。
func TestReCommitReplacesTheWeekAndRefreshesTheTime(t *testing.T) {
	st := openTestStore(t)

	first := stagedBatch(nil)
	if _, err := st.SaveStaging(first); err != nil {
		t.Fatalf("save first staging: %v", err)
	}
	firstEntry, err := st.Commit("2639")
	if err != nil {
		t.Fatalf("first commit: %v", err)
	}

	// 把第一次的时间推回很久以前，这样“时间被刷新”是确定可断言的，不依赖
	// 两次调用之间是否跨秒。
	const old = "2020-01-01 00:00:00"
	if _, err := st.db.Exec(`UPDATE archive SET committed_at=? WHERE week_code='2639'`, old); err != nil {
		t.Fatalf("age first commit: %v", err)
	}

	// 第二次导入同一周，行数与第一次不同：覆盖后应当只剩新数据。
	second := stagedBatch(nil)
	extra := second.Rows[0]
	extra.SourceRow = mps.FirstDataRow + 1
	extra.Index[0] = "P5_EP_ZZ"
	extra.Weeks = []string{"150"}
	second.Rows = append(second.Rows, extra)
	second.Files[0].RowsTotal = 2
	second.Files[0].RowsKept = 2
	if _, err := st.SaveStaging(second); err != nil {
		t.Fatalf("save second staging: %v", err)
	}
	secondEntry, err := st.Commit("2639")
	if err != nil {
		t.Fatalf("second commit: %v", err)
	}
	if secondEntry.RowCount != 2 {
		t.Fatalf("second commit rows = %d, want 2", secondEntry.RowCount)
	}

	// 归档里 2639 只有一行，且时间已经是这次整合的时间。
	list, err := st.ListArchive()
	if err != nil {
		t.Fatalf("list archive: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("archive rows = %d (%+v), want exactly one 2639", len(list), list)
	}
	if list[0].WeekCode != "2639" {
		t.Fatalf("archive week = %s, want 2639", list[0].WeekCode)
	}
	if list[0].CommittedAt == old {
		t.Errorf("committed_at 仍是旧值 %s，再次整合后应当刷新", list[0].CommittedAt)
	}
	if list[0].CommittedAt != secondEntry.CommittedAt {
		t.Errorf("archive committed_at = %s, want %s", list[0].CommittedAt, secondEntry.CommittedAt)
	}
	if firstEntry.CommittedAt == "" {
		t.Errorf("first commit returned an empty time")
	}
	if list[0].RowCount != 2 {
		t.Errorf("archive row_count = %d, want 2 (新旧数据不得累加)", list[0].RowCount)
	}

	// 周数据表被整体替换：只剩第二次导入的两行。
	var rows int
	if err := st.db.QueryRow(`SELECT COUNT(1) FROM data_2639`).Scan(&rows); err != nil {
		t.Fatalf("count data_2639: %v", err)
	}
	if rows != 2 {
		t.Errorf("data_2639 rows = %d, want 2", rows)
	}

	// 年份/周筛选同样只看到一份 2639。
	weeks, err := st.WeeksOfYear(2026)
	if err != nil {
		t.Fatalf("weeks of year: %v", err)
	}
	if len(weeks) != 1 || weeks[0] != 39 {
		t.Errorf("weeks of 2026 = %v, want [39]", weeks)
	}
}

// 整合返回的时间必须就是写进归档的那个时间。两次各读一次时钟时，两者只在
// 恰好跨秒的时候才会不一致——真实运行几乎不会，Windows 的 CI 跑出来了一次。
// 这里让每次读取都往前走一秒，把"是否同一次读数"变成确定的断言。
func TestCommitReturnsTheTimeItStored(t *testing.T) {
	st := openTestStore(t)

	var readings int
	real := Now
	Now = func() string {
		readings++
		return fmt.Sprintf("2026-01-01 00:00:%02d", readings)
	}
	t.Cleanup(func() { Now = real })

	if _, err := st.SaveStaging(stagedBatch(nil)); err != nil {
		t.Fatalf("save staging: %v", err)
	}
	entry, err := st.Commit("2639")
	if err != nil {
		t.Fatalf("commit: %v", err)
	}

	list, err := st.ListArchive()
	if err != nil {
		t.Fatalf("list archive: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("archive rows = %d, want 1", len(list))
	}
	if list[0].CommittedAt != entry.CommittedAt {
		t.Errorf("归档时间 %q 与返回的 %q 不是同一次读数（共读了 %d 次时钟）",
			list[0].CommittedAt, entry.CommittedAt, readings)
	}
}
