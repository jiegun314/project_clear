package store

import (
	"database/sql"
	"encoding/json"
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
}

// Commit promotes the staging rows into a permanent per-week table. Re-running
// it for the same week replaces the previous table, as specified.
func (s *Store) Commit(weekCode string) (*ArchiveEntry, error) {
	code, err := mustWeek(weekCode)
	if err != nil {
		return nil, err
	}
	sum, err := s.LoadStaging()
	if err != nil {
		return nil, err
	}
	if !sum.HasStaging {
		return nil, fmt.Errorf("没有待整合的临时数据，请先导入或添加文件")
	}
	if sum.WeekCode != code {
		return nil, fmt.Errorf("临时数据属于周码 %s，无法整合为 %s", sum.WeekCode, code)
	}
	table := DataTableName(code)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Overwrite any previous copy of this week.
	if _, err := tx.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s`, quoteIdent(table))); err != nil {
		return nil, err
	}
	var b strings.Builder
	fmt.Fprintf(&b, `CREATE TABLE %s (seq INTEGER PRIMARY KEY`, quoteIdent(table))
	for i := 1; i <= mps.IndexCols; i++ {
		fmt.Fprintf(&b, `, c%d TEXT`, i)
	}
	for _, wc := range sum.WeekCodes {
		// Declared REAL for correct numeric ordering, but SQLite's dynamic
		// typing still keeps a stray text value intact rather than coercing it.
		fmt.Fprintf(&b, `, %s REAL`, quoteIdent(WeekColumnName(wc)))
	}
	b.WriteString(`, cf_id INTEGER, file_name TEXT, src_row INTEGER, style_ids TEXT, comments TEXT, cf_colors TEXT)`)
	if _, err := tx.Exec(b.String()); err != nil {
		return nil, err
	}

	placeholders := make([]string, 0, 4+mps.IndexCols+len(sum.WeekCodes))
	cols := make([]string, 0, 4+mps.IndexCols+len(sum.WeekCodes))
	cols = append(cols, "seq")
	placeholders = append(placeholders, "?")
	for i := 1; i <= mps.IndexCols; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
		placeholders = append(placeholders, "?")
	}
	for _, wc := range sum.WeekCodes {
		cols = append(cols, WeekColumnName(wc))
		placeholders = append(placeholders, "?")
	}
	cols = append(cols, "cf_id", "file_name", "src_row", "style_ids", "comments", "cf_colors")
	placeholders = append(placeholders, "?", "?", "?", "?", "?", "?")

	stmt, err := tx.Prepare(fmt.Sprintf(`INSERT INTO %s (%s) VALUES (%s)`,
		quoteIdent(table), strings.Join(cols, ","), strings.Join(placeholders, ",")))
	if err != nil {
		return nil, err
	}
	defer stmt.Close()

	sel2, err := tx.Query(
		`SELECT seq, c1,c2,c3,c4,c5,c6,c7,c8,c9,c10,c11,c12,c13,c14,c15,
		        weeks, style_ids, cf_id, file_name, src_row, comments, cf_colors
		 FROM stg_row WHERE batch_id=? ORDER BY seq`, sum.BatchID)
	if err != nil {
		return nil, err
	}
	defer sel2.Close()

	count := 0
	for sel2.Next() {
		var seq int
		var weeksJSON, styleJSON string
		var commentsJSON sql.NullString
		var cfColorsJSON sql.NullString
		var cfID sql.NullInt64
		var fileName string
		var srcRow int
		var idx [mps.IndexCols]string
		idxCells := make([]sql.NullString, mps.IndexCols)
		scan := make([]any, 0, 21)
		scan = append(scan, &seq)
		for i := range idxCells {
			scan = append(scan, &idxCells[i])
		}
		scan = append(scan, &weeksJSON, &styleJSON, &cfID, &fileName, &srcRow, &commentsJSON, &cfColorsJSON)
		if err := sel2.Scan(scan...); err != nil {
			return nil, err
		}
		for i := range idxCells {
			idx[i] = idxCells[i].String
		}
		var weeks []string
		s.decodeJSON("周数据", weeksJSON, &weeks)

		args := make([]any, 0, len(placeholders))
		args = append(args, seq)
		for i := 0; i < mps.IndexCols; i++ {
			args = append(args, idx[i])
		}
		for i := range sum.WeekCodes {
			var raw string
			if i < len(weeks) {
				raw = weeks[i]
			}
			args = append(args, weekValue(raw))
		}
		args = append(args, nullInt(cfID), fileName, srcRow, styleJSON, nullString(commentsJSON), nullString(cfColorsJSON))
		if _, err := stmt.Exec(args...); err != nil {
			return nil, err
		}
		count++
	}
	if err := sel2.Err(); err != nil {
		return nil, err
	}

	// The staged rows now live in the week table. Keeping the staging copy as
	// well would leave a second, full copy of every integrated week in the
	// database and grow it by roughly double each week. Nothing reads them once
	// the batch is committed: the grid reads data_<week>, and the 已导入文件
	// list reads batch_file.
	if _, err := tx.Exec(`DELETE FROM stg_row WHERE batch_id=?`, sum.BatchID); err != nil {
		return nil, err
	}

	files := 0
	if err := tx.QueryRow(`SELECT file_ok FROM batch WHERE id=?`, sum.BatchID).Scan(&files); err != nil && err != sql.ErrNoRows {
		return nil, err
	}

	// Read the clock once. The row is what the next read returns, so the value
	// handed back here has to be the same string — reading the clock twice let the
	// two land either side of a second boundary and disagree (the Windows runner
	// showed an archive time one second behind the time the caller was told).
	committedAt := Now()

	weekCodes, _ := json.Marshal(sum.WeekCodes)
	indexNames, _ := json.Marshal(sum.IndexNames)
	var paramSnap sql.NullString
	_ = tx.QueryRow(`SELECT param_snapshot FROM batch WHERE id=?`, sum.BatchID).Scan(&paramSnap)
	var templateID sql.NullInt64
	_ = tx.QueryRow(`SELECT template_id FROM batch WHERE id=?`, sum.BatchID).Scan(&templateID)

	if _, err := tx.Exec(
		`INSERT INTO archive(week_code, week_start, week_codes, index_names, row_count, file_count,
		                     batch_id, committed_at, template_id, param_snapshot, table_name)
		 VALUES(?,?,?,?,?,?,?,?,?,?,?)
		 ON CONFLICT(week_code) DO UPDATE SET
		   week_start=excluded.week_start, week_codes=excluded.week_codes,
		   index_names=excluded.index_names, row_count=excluded.row_count,
		   file_count=excluded.file_count, batch_id=excluded.batch_id,
		   committed_at=excluded.committed_at, template_id=excluded.template_id,
		   param_snapshot=excluded.param_snapshot, table_name=excluded.table_name`,
		code, sum.WeekStart, string(weekCodes), string(indexNames), count, files,
		sum.BatchID, committedAt, nullInt(templateID), paramSnap, table); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE batch SET status='committed' WHERE id=?`, sum.BatchID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}

	w, _ := mps.ParseWeekCode(code)
	return &ArchiveEntry{
		WeekCode:    code,
		WeekStart:   sum.WeekStart,
		Year:        w.FullYear(),
		WeekNo:      w.WeekNo,
		WeekCodes:   sum.WeekCodes,
		IndexNames:  sum.IndexNames,
		RowCount:    count,
		FileCount:   files,
		BatchID:     sum.BatchID,
		CommittedAt: committedAt,
		TableName:   table,
	}, nil
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
		        batch_id, committed_at, table_name
		 FROM archive ORDER BY week_code DESC`)
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
			&e.RowCount, &e.FileCount, &batchID, &e.CommittedAt, &e.TableName); err != nil {
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
func (s *Store) ArchiveEntryFor(weekCode string) (*ArchiveEntry, error) {
	code, err := mustWeek(weekCode)
	if err != nil {
		return nil, err
	}
	var e ArchiveEntry
	var weekCodes, indexNames string
	var batchID sql.NullInt64
	err = s.db.QueryRow(
		`SELECT week_code, week_start, week_codes, index_names, row_count, file_count,
		        batch_id, committed_at, table_name
		 FROM archive WHERE week_code=?`, code).
		Scan(&e.WeekCode, &e.WeekStart, &weekCodes, &indexNames, &e.RowCount, &e.FileCount,
			&batchID, &e.CommittedAt, &e.TableName)
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

// WeeksOfYear lists the ISO week numbers committed for a year, descending.
func (s *Store) WeeksOfYear(year int) ([]int, error) {
	list, err := s.ListArchive()
	if err != nil {
		return nil, err
	}
	out := []int{}
	for _, e := range list {
		if e.Year == year {
			out = append(out, e.WeekNo)
		}
	}
	sort.Sort(sort.Reverse(sort.IntSlice(out)))
	return out, nil
}
