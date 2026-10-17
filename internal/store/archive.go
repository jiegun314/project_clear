package store

import (
	"database/sql"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"project_clear/internal/mps"
)

// ArchiveEntry describes one committed week.
type ArchiveEntry struct {
	WeekCode    string   `json:"weekCode"`
	WeekStart   string   `json:"weekStart"`
	Year        int      `json:"year"`
	WeekNo      int      `json:"weekNo"`
	WeekCodes   []string `json:"weekCodes"`
	IndexNames  []string `json:"indexNames"`
	RowCount    int      `json:"rowCount"`
	FileCount   int      `json:"fileCount"`
	BatchID     int64    `json:"batchId"`
	CommittedAt string   `json:"committedAt"`
	TableName   string   `json:"tableName"`
	// Dataset is which of the week's three datasets this entry is: global, local
	// or merged.
	Dataset string `json:"dataset"`
}

// A committed week holds three datasets: the global MPS rows, the local MPS rows,
// and the merged result the business hands on.
const (
	DatasetGlobal = "global"
	DatasetLocal  = "local"
	DatasetMerged = "merged"
)

// normaliseDataset maps an empty name to the merged result — which is what every
// caller meant before the three datasets existed — and rejects anything else.
func normaliseDataset(dataset string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(dataset)) {
	case "", DatasetMerged:
		return DatasetMerged, nil
	case DatasetGlobal:
		return DatasetGlobal, nil
	case DatasetLocal:
		return DatasetLocal, nil
	default:
		return "", fmt.Errorf("未知的数据集 %q", dataset)
	}
}

// weekValue converts a staged cell into a bindable value: a number when it is
// one, the original text when it is not, and NULL when the cell was empty.
func weekValue(raw string) any {
	s := strings.TrimSpace(raw)
	if s == "" {
		return nil
	}
	if n, err := strconv.ParseFloat(s, 64); err == nil {
		return n
	}
	return raw
}

func nullInt(v sql.NullInt64) any {
	if !v.Valid {
		return nil
	}
	return v.Int64
}

// nullString keeps a nullable text column NULL rather than turning an absent
// value into an empty string that later reads as "no annotations on this row".
func nullString(v sql.NullString) any {
	if !v.Valid {
		return nil
	}
	return v.String
}

// ListArchive returns committed weeks, newest first.
func (s *Store) ListArchive() ([]ArchiveEntry, error) {
	rows, err := s.db.Query(
		`SELECT week_code, week_start, week_codes, index_names, row_count, file_count,
		        batch_id, committed_at, table_name, dataset
		 FROM archive ORDER BY week_code DESC,
		   CASE dataset WHEN 'merged' THEN 0 WHEN 'global' THEN 1 ELSE 2 END`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	// A nil slice marshals to JSON null, which the frontend would then have to
	// guard everywhere. Empty lists are always emitted as [].
	out := []ArchiveEntry{}
	for rows.Next() {
		var e ArchiveEntry
		var weekCodes, indexNames string
		var batchID sql.NullInt64
		if err := rows.Scan(&e.WeekCode, &e.WeekStart, &weekCodes, &indexNames,
			&e.RowCount, &e.FileCount, &batchID, &e.CommittedAt, &e.TableName, &e.Dataset); err != nil {
			return nil, err
		}
		e.BatchID = batchID.Int64
		s.decodeJSON("周码列表", weekCodes, &e.WeekCodes)
		s.decodeJSON("索引表头", indexNames, &e.IndexNames)
		if w, err := mps.ParseWeekCode(e.WeekCode); err == nil {
			e.Year = w.FullYear()
			e.WeekNo = w.WeekNo
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ArchiveEntryFor returns the registry row of one week.
func (s *Store) ArchiveEntryFor(weekCode, dataset string) (*ArchiveEntry, error) {
	code, err := mustWeek(weekCode)
	if err != nil {
		return nil, err
	}
	set, err := normaliseDataset(dataset)
	if err != nil {
		return nil, err
	}
	var e ArchiveEntry
	var weekCodes, indexNames string
	var batchID sql.NullInt64
	err = s.db.QueryRow(
		`SELECT week_code, week_start, week_codes, index_names, row_count, file_count,
		        batch_id, committed_at, table_name, dataset
		 FROM archive WHERE week_code=? AND dataset=?`, code, set).
		Scan(&e.WeekCode, &e.WeekStart, &weekCodes, &indexNames, &e.RowCount, &e.FileCount,
			&batchID, &e.CommittedAt, &e.TableName, &e.Dataset)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	e.BatchID = batchID.Int64
	s.decodeJSON("周码列表", weekCodes, &e.WeekCodes)
	s.decodeJSON("索引表头", indexNames, &e.IndexNames)
	if w, err := mps.ParseWeekCode(e.WeekCode); err == nil {
		e.Year = w.FullYear()
		e.WeekNo = w.WeekNo
	}
	return &e, nil
}

// AvailableYears lists the years that have committed weeks, newest first.
func (s *Store) AvailableYears() ([]int, error) {
	list, err := s.ListArchive()
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	out := []int{}
	for _, e := range list {
		if e.Year > 0 && !seen[e.Year] {
			seen[e.Year] = true
			out = append(out, e.Year)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out, nil
}

// WeeksOfYear lists the ISO week numbers committed for a year, descending. A
// week holds three datasets, so the registry has three rows for it; this list is
// the weeks, not the rows.
func (s *Store) WeeksOfYear(year int) ([]int, error) {
	list, err := s.ListArchive()
	if err != nil {
		return nil, err
	}
	seen := map[int]bool{}
	out := []int{}
	for _, e := range list {
		if e.Year == year && !seen[e.WeekNo] {
			seen[e.WeekNo] = true
			out = append(out, e.WeekNo)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out, nil
}
