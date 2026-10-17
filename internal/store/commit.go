package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"project_clear/internal/mps"
)

// DatasetTableName is the permanent table of one dataset of a week. The merged
// result keeps the week's own name (2639 -> data_2639); the two inputs are
// suffixed so each can be queried and replaced on its own.
func DatasetTableName(code, dataset string) string {
	switch dataset {
	case DatasetGlobal:
		return DataTableName(code) + "_global"
	case DatasetLocal:
		return DataTableName(code) + "_local"
	default:
		return DataTableName(code)
	}
}

// Commit promotes the staging rows into permanent per-week tables and returns
// the merged result's entry. It writes the merged result plus each input the
// workbench holds; a batch from before the local sheet existed holds only global
// rows, which are then the merged result too.
func (s *Store) Commit(weekCode string) (*ArchiveEntry, error) {
	entries, err := s.CommitAll(weekCode)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Dataset == DatasetMerged {
			return e, nil
		}
	}
	return nil, fmt.Errorf("整合没有生成整合结果")
}

// CommitAll computes the merged result from the staged global and local rows and
// writes the three tables in one transaction: the merge happens here, at 整合,
// and never earlier. A workbench holding only global rows (a batch written before
// local files existed) still produces just the merged table, which is what every
// earlier release wrote.
//
// Re-running it for the same week replaces the previous tables, as specified.
func (s *Store) CommitAll(weekCode string) ([]*ArchiveEntry, error) {
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
	weeks, err := parseStoredWeeks(sum.WeekCodes)
	if err != nil {
		return nil, err
	}

	globalRows, err := s.LoadStagingDataset(sum.BatchID, DatasetGlobal)
	if err != nil {
		return nil, err
	}
	if len(globalRows) == 0 {
		return nil, fmt.Errorf("没有 Global 数据，无法整合")
	}
	localRows, err := s.LoadStagingDataset(sum.BatchID, DatasetLocal)
	if err != nil {
		return nil, err
	}
	hasLocal := len(localRows) > 0

	mergedRows := globalRows
	if hasLocal {
		localName, err := s.DatasetFileName(sum.BatchID, DatasetLocal)
		if err != nil {
			return nil, err
		}
		header := mps.Header{IndexNames: sum.IndexNames, Weeks: weeks}
		localSheet := &mps.LocalSheet{
			Header:    header,
			Rows:      toMPSRows(localRows),
			RowsTotal: len(localRows),
			RowsKept:  len(localRows),
		}
		merged, err := mps.MergeLocal(header, toMPSRows(globalRows), localSheet)
		if err != nil {
			return nil, err
		}
		aligned := merged.AlignedTo(weeks)
		mergedRows = stagedRowsFromMerge(aligned, merged.Sources, globalRows, localName)
	}

	// Which table gets which rows. The merged table is named after the week, so
	// a week integrated before the local sheet existed keeps working unchanged.
	type target struct {
		dataset string
		table   string
		rows    []StagedRow
	}
	targets := []target{{DatasetMerged, DatasetTableName(code, DatasetMerged), mergedRows}}
	if hasLocal {
		targets = append(targets,
			target{DatasetGlobal, DatasetTableName(code, DatasetGlobal), globalRows},
			target{DatasetLocal, DatasetTableName(code, DatasetLocal), localRows},
		)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// Read the clock once. The row is what the next read returns, so the value
	// handed back here has to be the same string — reading the clock twice let the
	// two land either side of a second boundary and disagree (the Windows runner
	// showed an archive time one second behind the time the caller was told).
	committedAt := Now()

	weekCodesJSON, _ := json.Marshal(sum.WeekCodes)
	indexNamesJSON, _ := json.Marshal(sum.IndexNames)
	var paramSnap sql.NullString
	_ = tx.QueryRow(`SELECT param_snapshot FROM batch WHERE id=?`, sum.BatchID).Scan(&paramSnap)
	var templateID sql.NullInt64
	_ = tx.QueryRow(`SELECT template_id FROM batch WHERE id=?`, sum.BatchID).Scan(&templateID)

	// The per-file record knows which side each workbook came from, so the
	// archive can say how many files each dataset is made of.
	filesByDataset := map[string]int{}
	frows, err := tx.Query(
		`SELECT dataset, COUNT(1) FROM batch_file WHERE batch_id=? AND status='ok' GROUP BY dataset`,
		sum.BatchID)
	if err != nil {
		return nil, err
	}
	for frows.Next() {
		var name string
		var n int
		if err := frows.Scan(&name, &n); err != nil {
			frows.Close()
			return nil, err
		}
		filesByDataset[name] = n
	}
	if err := frows.Err(); err != nil {
		frows.Close()
		return nil, err
	}
	frows.Close()

	st, _ := mps.ParseWeekCode(code)
	entries := make([]*ArchiveEntry, 0, len(targets))
	cfCache := map[string]sql.NullInt64{}
	for _, t := range targets {
		if err := createWeekTableTx(tx, t.table, sum.WeekCodes); err != nil {
			return nil, err
		}
		count, err := insertRowsTx(tx, t.table, sum.WeekCodes, t.rows, cfCache)
		if err != nil {
			return nil, err
		}
		files := filesByDataset[t.dataset]
		if t.dataset == DatasetMerged {
			// The merged result is built from both sides, so all their workbooks
			// count towards it.
			files = filesByDataset[DatasetGlobal] + filesByDataset[DatasetLocal]
		}
		if _, err := tx.Exec(
			`INSERT INTO archive(week_code, dataset, week_start, week_codes, index_names, row_count,
			                     file_count, batch_id, committed_at, template_id, param_snapshot, table_name)
			 VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
			 ON CONFLICT(week_code, dataset) DO UPDATE SET
			   week_start=excluded.week_start, week_codes=excluded.week_codes,
			   index_names=excluded.index_names, row_count=excluded.row_count,
			   file_count=excluded.file_count, batch_id=excluded.batch_id,
			   committed_at=excluded.committed_at, template_id=excluded.template_id,
			   param_snapshot=excluded.param_snapshot, table_name=excluded.table_name`,
			code, t.dataset, sum.WeekStart, string(weekCodesJSON), string(indexNamesJSON), count,
			files, sum.BatchID, committedAt, nullInt(templateID), paramSnap, t.table); err != nil {
			return nil, err
		}
		entries = append(entries, &ArchiveEntry{
			WeekCode:    code,
			WeekStart:   sum.WeekStart,
			Year:        st.FullYear(),
			WeekNo:      st.WeekNo,
			WeekCodes:   sum.WeekCodes,
			IndexNames:  sum.IndexNames,
			RowCount:    count,
			FileCount:   files,
			BatchID:     sum.BatchID,
			CommittedAt: committedAt,
			TableName:   t.table,
			Dataset:     t.dataset,
		})
	}

	// The staged rows now live in the week tables. Keeping the staging copy as
	// well would leave a second, full copy of every integrated week in the
	// database and grow it by roughly double each week. Nothing reads them once
	// the batch is committed: the grid reads data_<week>, and the 已导入文件
	// list reads batch_file.
	if _, err := tx.Exec(`DELETE FROM stg_row WHERE batch_id=?`, sum.BatchID); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE batch SET status='committed' WHERE id=?`, sum.BatchID); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return entries, nil
}

// createWeekTableTx rebuilds one week table with the batch's columns.
func createWeekTableTx(tx *sql.Tx, table string, weekCodes []string) error {
	if _, err := tx.Exec(fmt.Sprintf(`DROP TABLE IF EXISTS %s`, quoteIdent(table))); err != nil {
		return err
	}
	var b strings.Builder
	fmt.Fprintf(&b, `CREATE TABLE %s (seq INTEGER PRIMARY KEY`, quoteIdent(table))
	for i := 1; i <= mps.IndexCols; i++ {
		fmt.Fprintf(&b, `, c%d TEXT`, i)
	}
	for _, wc := range weekCodes {
		// Declared REAL for correct numeric ordering, but SQLite's dynamic
		// typing still keeps a stray text value intact rather than coercing it.
		fmt.Fprintf(&b, `, %s REAL`, quoteIdent(WeekColumnName(wc)))
	}
	b.WriteString(`, cf_id INTEGER, file_name TEXT, src_row INTEGER, style_ids TEXT, comments TEXT, cf_colors TEXT)`)
	_, err := tx.Exec(b.String())
	return err
}

// insertRowsTx writes rows into a week table and returns how many there were.
func insertRowsTx(tx *sql.Tx, table string, weekCodes []string, rows []StagedRow, cfCache map[string]sql.NullInt64) (int, error) {
	var b strings.Builder
	fmt.Fprintf(&b, `INSERT INTO %s (seq`, quoteIdent(table))
	for i := 1; i <= mps.IndexCols; i++ {
		fmt.Fprintf(&b, `, c%d`, i)
	}
	for _, wc := range weekCodes {
		fmt.Fprintf(&b, `, %s`, quoteIdent(WeekColumnName(wc)))
	}
	b.WriteString(`, cf_id, file_name, src_row, style_ids, comments, cf_colors) VALUES (?`)
	for i := 0; i < mps.IndexCols+len(weekCodes)+6; i++ {
		b.WriteString(",?")
	}
	b.WriteString(")")
	stmt, err := tx.Prepare(b.String())
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for i, r := range rows {
		var styleJSON []byte
		if r.StyleIDs != nil {
			styleJSON, err = json.Marshal(r.StyleIDs)
			if err != nil {
				return 0, err
			}
		} else {
			styleJSON = []byte("[]")
		}
		var cfID sql.NullInt64
		if !r.CF.Empty() {
			payload := mps.MarshalRow(r.CF)
			if cached, ok := cfCache[payload]; ok {
				cfID = cached
			} else {
				id, err := cfPatternIDTx(tx, payload)
				if err != nil {
					return 0, err
				}
				cfCache[payload] = id
				cfID = id
			}
		}
		var comments any
		if len(r.Comments) > 0 {
			data, err := json.Marshal(r.Comments)
			if err != nil {
				return 0, err
			}
			comments = string(data)
		}
		var cfColors any
		if hasAny(r.CFColors) {
			data, err := json.Marshal(r.CFColors)
			if err != nil {
				return 0, err
			}
			cfColors = string(data)
		}

		args := make([]any, 0, 1+mps.IndexCols+len(weekCodes)+5)
		args = append(args, i)
		for _, v := range r.Index {
			args = append(args, v)
		}
		for w := range weekCodes {
			var raw string
			if w < len(r.Weeks) {
				raw = r.Weeks[w]
			}
			args = append(args, weekValue(raw))
		}
		args = append(args, nullInt(cfID), r.FileName, r.SourceRow, string(styleJSON),
			comments, cfColors)
		if _, err := stmt.Exec(args...); err != nil {
			return 0, err
		}
	}
	return len(rows), nil
}

// toMPSRows converts stored rows into the reader's shape for the merge.
func toMPSRows(rows []StagedRow) []*mps.DataRow {
	out := make([]*mps.DataRow, 0, len(rows))
	for _, r := range rows {
		out = append(out, &mps.DataRow{
			Index: r.Index, Weeks: r.Weeks, StyleIDs: r.StyleIDs,
			CF: r.CF, Comments: r.Comments, CFColors: r.CFColors, SourceRow: r.SourceRow,
		})
	}
	return out
}

// stagedRowsFromMerge turns merged rows back into staged rows, keeping the file
// and worksheet row each one came from: a global row keeps its workbook, a row
// only the local file has keeps the local file's name.
func stagedRowsFromMerge(rows []*mps.DataRow, sources []mps.RowSource, global []StagedRow, localName string) []StagedRow {
	out := make([]StagedRow, 0, len(rows))
	for i, r := range rows {
		row := StagedRow{
			Index: r.Index, Weeks: r.Weeks, StyleIDs: r.StyleIDs,
			CF: r.CF, Comments: r.Comments, CFColors: r.CFColors, SourceRow: r.SourceRow,
		}
		if i < len(sources) {
			src := sources[i]
			if src.Local {
				row.FileName = localName
			} else if src.Index < len(global) {
				row.FileName = global[src.Index].FileName
				row.SourceRow = global[src.Index].SourceRow
			}
		}
		out = append(out, row)
	}
	return out
}

// parseStoredWeeks turns the stored week codes back into the week type.
func parseStoredWeeks(codes []string) ([]mps.Week, error) {
	out := make([]mps.Week, 0, len(codes))
	for _, c := range codes {
		w, err := mps.ParseWeekCode(c)
		if err != nil {
			return nil, fmt.Errorf("周码 %q 无法识别: %w", c, err)
		}
		out = append(out, w)
	}
	return out, nil
}
