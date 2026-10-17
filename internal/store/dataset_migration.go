package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// addDatasetModel moves a v4 database from "one table per week and dataset" to
// the dataset model: every archived week becomes up to three ready datasets
// (merged, global, local) plus one integration row, with the rows copied into
// dataset_row. The old archive, batch and staging tables are then dropped.
//
// Un-integrated staging rows are not carried over: under the new model a draft
// is discarded on startup, so an upgrade drops them the same way.
func addDatasetModel(q sqlQueryer) error {
	type archived struct {
		week       string
		kind       string
		weekStart  string
		weekCodes  []string
		indexNames []string
		rowCount   int
		fileCount  int
		committed  string
		table      string
		params     string
	}

	var archivedRows []archived
	rows, err := q.Query(`SELECT week_code, dataset, week_start, week_codes, index_names,
	                             row_count, file_count, committed_at, table_name,
	                             COALESCE(param_snapshot,'')
	                      FROM archive ORDER BY week_code, dataset`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var a archived
		var weekCodes, indexNames string
		if err := rows.Scan(&a.week, &a.kind, &a.weekStart, &weekCodes, &indexNames,
			&a.rowCount, &a.fileCount, &a.committed, &a.table, &a.params); err != nil {
			rows.Close()
			return err
		}
		_ = json.Unmarshal([]byte(weekCodes), &a.weekCodes)
		_ = json.Unmarshal([]byte(indexNames), &a.indexNames)
		archivedRows = append(archivedRows, a)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	// week -> kind -> dataset id/import metadata, for the integration rows.
	type weekInfo struct {
		ids        map[string]int64
		importedAt string
		params     string
	}
	byWeek := map[string]*weekInfo{}
	var weeks []string
	for _, a := range archivedRows {
		id, err := copyArchivedDataset(q, a.week, a.weekStart, a.kind, a.weekCodes,
			a.indexNames, a.committed, a.params, a.table, a.fileCount)
		if err != nil {
			return fmt.Errorf("迁移 %s/%s 失败: %w", a.week, a.kind, err)
		}
		info := byWeek[a.week]
		if info == nil {
			info = &weekInfo{ids: map[string]int64{}}
			byWeek[a.week] = info
			weeks = append(weeks, a.week)
		}
		info.ids[a.kind] = id
		if a.kind == DatasetMerged {
			info.importedAt = a.committed
			info.params = a.params
		}
		if _, err := q.Exec(`DROP TABLE IF EXISTS ` + quoteIdent(a.table)); err != nil {
			return err
		}
	}

	// One integration per migrated week. A week integrated before local files
	// existed has only the merged dataset; its global/local references stay NULL.
	for _, week := range weeks {
		info := byWeek[week]
		merged, ok := info.ids[DatasetMerged]
		if !ok {
			continue
		}
		var global, local any
		if id, ok := info.ids[DatasetGlobal]; ok {
			global = id
		}
		if id, ok := info.ids[DatasetLocal]; ok {
			local = id
		}
		if _, err := q.Exec(
			`INSERT INTO integration(week_code, global_id, local_id, merged_id, integrated_at, params)
			 VALUES(?,?,?,?,?,?)`, week, global, local, merged, info.importedAt, info.params); err != nil {
			return err
		}
	}

	// The old tables are no longer read by this release.
	for _, table := range []string{"archive", "batch", "stg_row", "batch_file"} {
		if _, err := q.Exec(`DROP TABLE IF EXISTS ` + quoteIdent(table)); err != nil {
			return err
		}
	}
	// Any per-week table still lying around (for example one whose archive row
	// was lost) is stale after the copy.
	tables, err := dataTables(q)
	if err != nil {
		return err
	}
	for _, table := range tables {
		if _, err := q.Exec(`DROP TABLE IF EXISTS ` + quoteIdent(table)); err != nil {
			return err
		}
	}
	_, err = q.Exec(`DELETE FROM app_meta WHERE key='files_cleared_upto'`)
	return err
}

// copyArchivedDataset writes one archive row's data as a ready dataset and
// returns its id.
func copyArchivedDataset(q sqlQueryer, week, weekStart, kind string, weekCodes, indexNames []string,
	importedAt, params, table string, fileCount int) (int64, error) {
	weekCodesJSON, _ := json.Marshal(weekCodes)
	indexNamesJSON, _ := json.Marshal(indexNames)

	res, err := q.Exec(
		`INSERT INTO dataset(week_code, week_start, kind, version, status, row_count,
		                     file_count, failed_count, week_codes, index_names, imported_at, params)
		 VALUES(?,?,?,1,'ready',0,?,0,?,?,?,?)`,
		week, weekStart, kind, fileCount, string(weekCodesJSON), string(indexNamesJSON), importedAt, params)
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}

	// Read the whole table before writing: the transaction runs on one
	// connection, so a cursor cannot stay open while another statement runs.
	cols := []string{"seq", "file_name", "src_row", "style_ids", "cf_id", "comments", "cf_colors"}
	for i := 1; i <= 15; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
	}
	for _, wc := range weekCodes {
		cols = append(cols, WeekColumnName(wc))
	}
	selectSQL := fmt.Sprintf(`SELECT %s FROM %s ORDER BY seq`,
		strings.Join(cols, ","), quoteIdent(table))
	dataRows, err := q.Query(selectSQL)
	if err != nil {
		return 0, err
	}
	type oldRow struct {
		seq       int
		fileName  sql.NullString
		srcRow    sql.NullInt64
		styleJSON sql.NullString
		cfID      sql.NullInt64
		comments  sql.NullString
		cfColors  sql.NullString
		index     [15]sql.NullString
		weeks     []string
	}
	var all []oldRow
	for dataRows.Next() {
		var r oldRow
		scan := []any{&r.seq, &r.fileName, &r.srcRow, &r.styleJSON, &r.cfID, &r.comments, &r.cfColors}
		for i := range r.index {
			scan = append(scan, &r.index[i])
		}
		weekVals := make([]any, len(weekCodes))
		for i := range weekVals {
			scan = append(scan, &weekVals[i])
		}
		if err := dataRows.Scan(scan...); err != nil {
			dataRows.Close()
			return 0, err
		}
		r.weeks = make([]string, len(weekVals))
		for i, v := range weekVals {
			r.weeks[i] = cellText(v)
		}
		all = append(all, r)
	}
	if err := dataRows.Err(); err != nil {
		dataRows.Close()
		return 0, err
	}
	dataRows.Close()

	insertCols := []string{"dataset_id", "seq", "file_name", "src_row"}
	for i := 1; i <= 15; i++ {
		insertCols = append(insertCols, fmt.Sprintf("c%d", i))
	}
	insertCols = append(insertCols, "weeks", "style_ids", "cf_id", "comments", "cf_colors")
	ph := make([]string, len(insertCols))
	for i := range ph {
		ph[i] = "?"
	}
	stmt, err := q.Prepare(fmt.Sprintf(`INSERT INTO dataset_row(%s) VALUES(%s)`,
		strings.Join(insertCols, ","), strings.Join(ph, ",")))
	if err != nil {
		return 0, err
	}
	defer stmt.Close()

	for _, r := range all {
		weeksJSON, _ := json.Marshal(r.weeks)
		styleJSON := "[]"
		if r.styleJSON.Valid && r.styleJSON.String != "" {
			styleJSON = r.styleJSON.String
		}
		var comments, cfColors any
		if r.comments.Valid {
			comments = r.comments.String
		}
		if r.cfColors.Valid {
			cfColors = r.cfColors.String
		}
		args := []any{id, r.seq, nullString(r.fileName), nullInt(r.srcRow)}
		for i := range r.index {
			args = append(args, nullString(r.index[i]))
		}
		args = append(args, string(weeksJSON), styleJSON, nullInt(r.cfID), comments, cfColors)
		if _, err := stmt.Exec(args...); err != nil {
			return 0, err
		}
	}
	if _, err := q.Exec(`UPDATE dataset SET row_count=? WHERE id=?`, len(all), id); err != nil {
		return 0, err
	}
	return id, nil
}

// cellText turns a scanned SQLite cell into the text the reader would have
// produced: numbers keep their shortest exact form, text stays as stored.
func cellText(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case []byte:
		return string(x)
	case int64:
		return strconv.FormatInt(x, 10)
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		return fmt.Sprint(x)
	}
}
