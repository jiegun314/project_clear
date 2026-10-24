package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"project_clear/internal/mps"
)

// Dataset kinds and statuses.
const (
	DatasetGlobal = "global"
	DatasetLocal  = "local"
	DatasetMerged = "merged"

	DatasetDraft = "draft"
	DatasetReady = "ready"
)

// Dataset is one complete import (or merged result) for a week. A draft is the
// Global import still being assembled; it becomes ready when the user finishes
// it, at which point it gets a version.
type Dataset struct {
	ID          int64
	WeekCode    string
	WeekStart   string
	Kind        string
	Version     int
	Status      string
	RowCount    int
	FileCount   int
	FailedCount int
	WeekCodes   []string
	IndexNames  []string
	ImportedAt  string
	Params      string
	BaseID      sql.NullInt64
}

// Ready reports whether the dataset is a finished import.
func (d *Dataset) Ready() bool { return d.Status == DatasetReady }

// StagedFile records the outcome of reading one source workbook. The name is
// historical: the same shape describes a file of a draft or of a ready dataset.
type StagedFile struct {
	Path      string
	Name      string
	Size      int64
	Status    string // ok | failed
	RowsTotal int
	RowsKept  int
	WeekCode  string
	Err       string
}

// StagedRow is one row waiting to be written to a dataset.
type StagedRow struct {
	FileName string
	// FilePath identifies the workbook the row came from; callers that only set
	// FileName still work because the id is then resolved by name.
	FilePath  string
	SourceRow int
	Index     [mps.IndexCols]string
	Weeks     []string
	StyleIDs  []int
	CF        mps.CFRow
	Comments  mps.CommentMap
	// CFColors is the colour the conditional formats paint on each week cell,
	// aligned to Weeks. Empty strings mean "no rule matched".
	CFColors []string
}

// StagedFileDetail is one row of the per-file report.
type StagedFileDetail struct {
	Name      string `json:"name"`
	Path      string `json:"path"`
	Size      int64  `json:"size"`
	Status    string `json:"status"`
	RowsTotal int    `json:"rowsTotal"`
	RowsKept  int    `json:"rowsKept"`
	WeekCode  string `json:"weekCode"`
	Err       string `json:"err,omitempty"`
}

const datasetCols = `id, week_code, week_start, kind, version, status, row_count,
	file_count, failed_count, week_codes, index_names, imported_at,
	COALESCE(params,''), base_id`

func scanDataset(row interface{ Scan(...any) error }) (*Dataset, error) {
	var d Dataset
	var weekCodes, indexNames string
	if err := row.Scan(&d.ID, &d.WeekCode, &d.WeekStart, &d.Kind, &d.Version, &d.Status,
		&d.RowCount, &d.FileCount, &d.FailedCount, &weekCodes, &indexNames,
		&d.ImportedAt, &d.Params, &d.BaseID); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(weekCodes), &d.WeekCodes)
	_ = json.Unmarshal([]byte(indexNames), &d.IndexNames)
	return &d, nil
}

// Dataset loads one dataset by id.
func (s *Store) Dataset(id int64) (*Dataset, error) {
	return scanDataset(s.db.QueryRow(`SELECT `+datasetCols+` FROM dataset WHERE id=?`, id))
}

func loadDataset(q sqlQueryer, id int64) (*Dataset, error) {
	d, err := scanDataset(q.QueryRow(`SELECT `+datasetCols+` FROM dataset WHERE id=?`, id))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// Current returns the newest ready dataset of one kind for a week.
func (s *Store) Current(week, kind string) (*Dataset, error) {
	d, err := scanDataset(s.db.QueryRow(
		`SELECT `+datasetCols+` FROM dataset
		 WHERE week_code=? AND kind=? AND status='ready'
		 ORDER BY version DESC LIMIT 1`, week, kind))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// Draft returns the open Global import, or nil.
func (s *Store) Draft() (*Dataset, error) {
	d, err := scanDataset(s.db.QueryRow(
		`SELECT ` + datasetCols + ` FROM dataset WHERE status='draft' ORDER BY id DESC LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func currentTx(q sqlQueryer, week, kind string) (*Dataset, error) {
	d, err := scanDataset(q.QueryRow(
		`SELECT `+datasetCols+` FROM dataset
		 WHERE week_code=? AND kind=? AND status='ready'
		 ORDER BY version DESC LIMIT 1`, week, kind))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

// DatasetsOfWeek lists the ready datasets of a week, newest version first.
func (s *Store) DatasetsOfWeek(week string) ([]*Dataset, error) {
	rows, err := s.db.Query(
		`SELECT `+datasetCols+` FROM dataset
		 WHERE week_code=? AND status='ready' ORDER BY kind, version DESC`, week)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Dataset
	for rows.Next() {
		d, err := scanDataset(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// WeeksWithReadyData lists every week that has at least one completed dataset,
// whether or not the week has been integrated. History is built from this list:
// finishing a Global import or importing a Local file is already a completed
// dataset, so it must appear there before 整合.
func (s *Store) WeeksWithReadyData() ([]string, error) {
	rows, err := s.db.Query(
		`SELECT DISTINCT week_code FROM dataset WHERE status='ready' ORDER BY week_code DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var week string
		if err := rows.Scan(&week); err != nil {
			return nil, err
		}
		out = append(out, week)
	}
	return out, rows.Err()
}

// LatestWeek is the week of the most recently touched ready dataset or draft.
func (s *Store) LatestWeek() (string, error) {
	var week string
	err := s.db.QueryRow(
		`SELECT week_code FROM dataset ORDER BY imported_at DESC, id DESC LIMIT 1`).Scan(&week)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return week, err
}

// BeginGlobalImport opens a Global draft, replacing any draft that is already
// open. A baseID copies an existing ready Global dataset first (copy-on-write),
// which is how "add a missed file after finishing" starts a new revision.
func (s *Store) BeginGlobalImport(week, weekStart string, weekCodes, indexNames []string,
	params string, baseID int64) (*Dataset, error) {
	if _, err := mustWeek(week); err != nil {
		return nil, err
	}
	weekCodesJSON, _ := json.Marshal(weekCodes)
	indexNamesJSON, _ := json.Marshal(indexNames)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM dataset WHERE status='draft'`); err != nil {
		return nil, err
	}
	var base any
	if baseID > 0 {
		baseDataset, err := loadDataset(tx, baseID)
		if err != nil {
			return nil, err
		}
		if baseDataset == nil || baseDataset.WeekCode != week {
			return nil, fmt.Errorf("草稿来源与当前导入不是同一周")
		}
		base = baseID
	}
	res, err := tx.Exec(
		`INSERT INTO dataset(week_code, week_start, kind, version, status, week_codes,
		                     index_names, imported_at, params, base_id)
		 VALUES(?,?,'global',0,'draft',?,?,?,?,?)`,
		week, weekStart, string(weekCodesJSON), string(indexNamesJSON), Now(), params, base)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if baseID > 0 {
		if err := copyDatasetTx(tx, baseID, id); err != nil {
			return nil, err
		}
	}
	if err := refreshCountsTx(tx, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Dataset(id)
}

// copyDatasetTx copies the rows and files of one dataset into another.
func copyDatasetTx(q sqlQueryer, from, to int64) error {
	if _, err := q.Exec(
		`INSERT INTO dataset_row(dataset_id, seq, file_name, src_row,
		    c1,c2,c3,c4,c5,c6,c7,c8,c9,c10,c11,c12,c13,c14,c15,
		    weeks, style_ids, cf_id, comments, cf_colors)
		 SELECT ?, seq, file_name, src_row,
		        c1,c2,c3,c4,c5,c6,c7,c8,c9,c10,c11,c12,c13,c14,c15,
		        weeks, style_ids, cf_id, comments, cf_colors
		 FROM dataset_row WHERE dataset_id=?`, to, from); err != nil {
		return err
	}
	_, err := q.Exec(
		`INSERT INTO dataset_file(dataset_id, path, name, size, status, rows_total, rows_kept, week_code, err)
		 SELECT ?, path, name, size, status, rows_total, rows_kept, week_code, err
		 FROM dataset_file WHERE dataset_id=?`, to, from)
	return err
}

// AddDraftData appends (or replaces by file name) files and rows of an open draft.
func (s *Store) AddDraftData(id int64, files []StagedFile, rows []StagedRow, replace []string,
	dict *mps.StyleDict, dxf *mps.DxfDict) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	d, err := loadDataset(tx, id)
	if err != nil {
		return err
	}
	if d == nil || d.Status != DatasetDraft {
		return fmt.Errorf("没有正在进行的 Global 导入")
	}
	if len(replace) > 0 {
		ph := make([]string, len(replace))
		args := make([]any, 0, len(replace)+1)
		args = append(args, id)
		for i, name := range replace {
			ph[i] = "?"
			args = append(args, name)
		}
		list := strings.Join(ph, ",")
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM dataset_row WHERE dataset_id=? AND file_id IN
			 (SELECT id FROM dataset_file WHERE dataset_id=? AND name IN (%s))`, list),
			append([]any{id, id}, args[1:]...)...); err != nil {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM dataset_row WHERE dataset_id=? AND file_name IN (%s)`, list),
			append([]any{id}, args[1:]...)...); err != nil {
			return err
		}
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM dataset_file WHERE dataset_id=? AND name IN (%s)`, list),
			append([]any{id}, args[1:]...)...); err != nil {
			return err
		}
	}
	styleMap, err := resolveStyleIDsTx(tx, dict)
	if err != nil {
		return err
	}
	if _, err := resolveDxfIDsTx(tx, dxf); err != nil {
		return err
	}
	fileIDs, err := insertDatasetFilesTx(tx, id, files)
	if err != nil {
		return err
	}
	var base int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(seq),-1)+1 FROM dataset_row WHERE dataset_id=?`, id).Scan(&base); err != nil {
		return err
	}
	if err := writeDatasetRowsTx(tx, id, base, rows, styleMap, fileIDs); err != nil {
		return err
	}
	if err := refreshCountsTx(tx, id); err != nil {
		return err
	}
	return tx.Commit()
}

// FinishDraft freezes a Global draft into a ready dataset. A draft with failed
// files is only finished when the caller has confirmed that is acceptable.
func (s *Store) FinishDraft(id int64, confirmFailures bool) (*Dataset, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	d, err := loadDataset(tx, id)
	if err != nil {
		return nil, err
	}
	if d == nil || d.Status != DatasetDraft {
		return nil, fmt.Errorf("没有正在进行的 Global 导入")
	}
	if err := refreshCountsTx(tx, id); err != nil {
		return nil, err
	}
	d, err = loadDataset(tx, id)
	if err != nil {
		return nil, err
	}
	if d.FailedCount > 0 && !confirmFailures {
		return nil, fmt.Errorf("还有 %d 个文件读取失败，请确认后再完成导入", d.FailedCount)
	}
	version, err := nextVersionTx(tx, d.WeekCode, d.Kind)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(
		`UPDATE dataset SET status='ready', version=?, imported_at=?, base_id=NULL WHERE id=?`,
		version, Now(), id); err != nil {
		return nil, err
	}
	if err := cleanupSupersededTx(tx, d.WeekCode, d.Kind, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Dataset(id)
}

// CreateLocalDataset stores a complete Local import as a ready dataset.
func (s *Store) CreateLocalDataset(week, weekStart string, weekCodes, indexNames []string,
	params string, files []StagedFile, rows []StagedRow, dict *mps.StyleDict) (*Dataset, error) {
	if _, err := mustWeek(week); err != nil {
		return nil, err
	}
	weekCodesJSON, _ := json.Marshal(weekCodes)
	indexNamesJSON, _ := json.Marshal(indexNames)

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	version, err := nextVersionTx(tx, week, DatasetLocal)
	if err != nil {
		return nil, err
	}
	res, err := tx.Exec(
		`INSERT INTO dataset(week_code, week_start, kind, version, status, week_codes,
		                     index_names, imported_at, params)
		 VALUES(?,?,'local',?,'ready',?,?,?,?)`,
		week, weekStart, version, string(weekCodesJSON), string(indexNamesJSON), Now(), params)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	fileIDs, err := insertDatasetFilesTx(tx, id, files)
	if err != nil {
		return nil, err
	}
	styleMap, err := resolveStyleIDsTx(tx, dict)
	if err != nil {
		return nil, err
	}
	if err := writeDatasetRowsTx(tx, id, 0, rows, styleMap, fileIDs); err != nil {
		return nil, err
	}
	if err := refreshCountsTx(tx, id); err != nil {
		return nil, err
	}
	if err := cleanupSupersededTx(tx, week, DatasetLocal, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Dataset(id)
}

// DiscardDraft deletes the open Global import.
func (s *Store) DiscardDraft(id int64) error {
	_, err := s.db.Exec(`DELETE FROM dataset WHERE id=? AND status='draft'`, id)
	return err
}

// DiscardAllDrafts drops every draft, which is what startup does.
func (s *Store) DiscardAllDrafts() (int, error) {
	res, err := s.db.Exec(`DELETE FROM dataset WHERE status='draft'`)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

// nextVersionTx is the next ready version for a week and kind.
func nextVersionTx(q sqlQueryer, week, kind string) (int, error) {
	var v int
	err := q.QueryRow(
		`SELECT COALESCE(MAX(version),0)+1 FROM dataset
		 WHERE week_code=? AND kind=? AND status='ready'`, week, kind).Scan(&v)
	return v, err
}

// cleanupSupersededTx deletes older ready versions that no integration points
// at. A version an integration still references survives, which is what keeps
// an imported-but-not-yet-reintegrated week's history intact.
func cleanupSupersededTx(q sqlQueryer, week, kind string, keepID int64) error {
	rows, err := q.Query(
		`SELECT id FROM dataset
		 WHERE week_code=? AND kind=? AND status='ready' AND id<>?
		   AND id NOT IN (SELECT global_id FROM integration WHERE global_id IS NOT NULL
		                  UNION SELECT local_id FROM integration WHERE local_id IS NOT NULL
		                  UNION SELECT merged_id FROM integration)`, week, kind, keepID)
	if err != nil {
		return err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, id := range ids {
		if _, err := q.Exec(`DELETE FROM dataset WHERE id=?`, id); err != nil {
			return err
		}
	}
	return nil
}

func refreshCountsTx(q sqlQueryer, id int64) error {
	_, err := q.Exec(
		`UPDATE dataset SET
		    row_count=(SELECT COUNT(1) FROM dataset_row WHERE dataset_id=?),
		    file_count=(SELECT COUNT(1) FROM dataset_file WHERE dataset_id=? AND status='ok'),
		    failed_count=(SELECT COUNT(1) FROM dataset_file WHERE dataset_id=? AND status<>'ok')
		 WHERE id=?`, id, id, id, id)
	return err
}

// insertDatasetFilesTx writes the workbook records and returns path -> id.
func insertDatasetFilesTx(tx *sql.Tx, datasetID int64, files []StagedFile) (map[string]int64, error) {
	ids := make(map[string]int64, len(files))
	for _, f := range files {
		res, err := tx.Exec(
			`INSERT INTO dataset_file(dataset_id, path, name, size, status, rows_total, rows_kept, week_code, err)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			datasetID, f.Path, f.Name, f.Size, f.Status, f.RowsTotal, f.RowsKept, f.WeekCode, f.Err)
		if err != nil {
			return nil, err
		}
		if id, err := res.LastInsertId(); err == nil && f.Path != "" {
			ids[f.Path] = id
		}
	}
	return ids, nil
}

// writeDatasetRowsTx appends rows to a dataset, numbering them from base.
func writeDatasetRowsTx(tx *sql.Tx, datasetID int64, base int, rows []StagedRow,
	styleMap map[int]int, fileIDs map[string]int64) error {
	cols := []string{"dataset_id", "seq", "file_id", "file_name", "src_row"}
	for i := 1; i <= mps.IndexCols; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
	}
	cols = append(cols, "weeks", "style_ids", "cf_id", "comments", "cf_colors")
	ph := make([]string, len(cols))
	for i := range ph {
		ph[i] = "?"
	}
	stmt, err := tx.Prepare(fmt.Sprintf(`INSERT INTO dataset_row(%s) VALUES(%s)`,
		strings.Join(cols, ","), strings.Join(ph, ",")))
	if err != nil {
		return err
	}
	defer stmt.Close()

	byName := map[string]int64{}
	nameCount := map[string]int{}
	for path, id := range fileIDs {
		name := filepathBase(path)
		byName[name] = id
		nameCount[name]++
	}
	cfCache := map[string]sql.NullInt64{}
	for i, r := range rows {
		weeks := r.Weeks
		if weeks == nil {
			weeks = []string{}
		}
		weeksJSON, err := json.Marshal(weeks)
		if err != nil {
			return err
		}
		styleIDs := make([]int, len(r.StyleIDs))
		for j, id := range r.StyleIDs {
			mapped, ok := styleMap[id]
			if !ok {
				mapped = id
			}
			styleIDs[j] = mapped
		}
		styleJSON, err := json.Marshal(styleIDs)
		if err != nil {
			return err
		}
		var cfID sql.NullInt64
		if !r.CF.Empty() {
			payload := mps.MarshalRow(r.CF)
			if cached, ok := cfCache[payload]; ok {
				cfID = cached
			} else {
				id, err := cfPatternIDTx(tx, payload)
				if err != nil {
					return err
				}
				cfCache[payload] = id
				cfID = id
			}
		}
		var comments any
		if len(r.Comments) > 0 {
			data, err := json.Marshal(r.Comments)
			if err != nil {
				return err
			}
			comments = string(data)
		}
		var cfColors any
		if hasAny(r.CFColors) {
			data, err := json.Marshal(r.CFColors)
			if err != nil {
				return err
			}
			cfColors = string(data)
		}
		var fileID any
		if id, ok := fileIDs[r.FilePath]; ok && r.FilePath != "" {
			fileID = id
		} else if id, ok := byName[r.FileName]; ok && nameCount[r.FileName] == 1 {
			fileID = id
		}
		args := []any{datasetID, base + i, fileID, r.FileName, r.SourceRow}
		for _, v := range r.Index {
			args = append(args, v)
		}
		args = append(args, string(weeksJSON), string(styleJSON), nullInt(cfID), comments, cfColors)
		if _, err := stmt.Exec(args...); err != nil {
			return err
		}
	}
	return nil
}

// DatasetFiles lists the workbooks of a dataset, in import order.
func (s *Store) DatasetFiles(datasetID int64) ([]StagedFileDetail, error) {
	rows, err := s.db.Query(
		`SELECT name, path, size, status, rows_total, rows_kept, week_code, err
		 FROM dataset_file WHERE dataset_id=? ORDER BY id`, datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StagedFileDetail{}
	for rows.Next() {
		var d StagedFileDetail
		if err := rows.Scan(&d.Name, &d.Path, &d.Size, &d.Status, &d.RowsTotal, &d.RowsKept, &d.WeekCode, &d.Err); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DatasetRows reads a dataset in the shape a merge or an export needs.
func (s *Store) DatasetRows(datasetID int64) ([]StagedRow, error) {
	cfPatterns, err := s.LoadCFPatterns()
	if err != nil {
		return nil, err
	}
	cols := []string{"seq", "file_name", "src_row"}
	for i := 1; i <= mps.IndexCols; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
	}
	cols = append(cols, "weeks", "style_ids", "cf_id", "comments", "cf_colors")
	rows, err := s.db.Query(fmt.Sprintf(`SELECT %s FROM dataset_row WHERE dataset_id=? ORDER BY seq`,
		strings.Join(cols, ",")), datasetID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []StagedRow{}
	for rows.Next() {
		var r StagedRow
		var seq int
		var weeksJSON, styleJSON string
		var cfID sql.NullInt64
		var commentsJSON, cfColorsJSON sql.NullString
		idx := make([]sql.NullString, mps.IndexCols)
		scan := []any{&seq, &r.FileName, &r.SourceRow}
		for i := range idx {
			scan = append(scan, &idx[i])
		}
		scan = append(scan, &weeksJSON, &styleJSON, &cfID, &commentsJSON, &cfColorsJSON)
		if err := rows.Scan(scan...); err != nil {
			return nil, err
		}
		for i := range idx {
			r.Index[i] = idx[i].String
		}
		s.decodeJSON("周数据", weeksJSON, &r.Weeks)
		s.decodeJSON("单元格样式", styleJSON, &r.StyleIDs)
		if commentsJSON.Valid && commentsJSON.String != "" {
			s.decodeJSON("批注", commentsJSON.String, &r.Comments)
		}
		if cfColorsJSON.Valid && cfColorsJSON.String != "" {
			s.decodeJSON("条件格式底色", cfColorsJSON.String, &r.CFColors)
		}
		if cfID.Valid {
			if payload, ok := cfPatterns[cfID.Int64]; ok {
				r.CF = mps.UnmarshalRow(payload)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Integration is a week's current combination of the three datasets.
type Integration struct {
	WeekCode     string
	GlobalID     sql.NullInt64
	LocalID      sql.NullInt64
	MergedID     int64
	IntegratedAt string
	Params       string
}

func scanIntegration(row interface{ Scan(...any) error }) (*Integration, error) {
	var in Integration
	var params sql.NullString
	if err := row.Scan(&in.WeekCode, &in.GlobalID, &in.LocalID, &in.MergedID,
		&in.IntegratedAt, &params); err != nil {
		return nil, err
	}
	in.Params = params.String
	return &in, nil
}

// Integration loads a week's integration row, or nil.
func (s *Store) Integration(week string) (*Integration, error) {
	in, err := scanIntegration(s.db.QueryRow(
		`SELECT week_code, global_id, local_id, merged_id, integrated_at, params
		 FROM integration WHERE week_code=?`, week))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return in, err
}

// ListIntegrations returns every integrated week, newest first.
func (s *Store) ListIntegrations() ([]*Integration, error) {
	rows, err := s.db.Query(
		`SELECT week_code, global_id, local_id, merged_id, integrated_at, params
		 FROM integration ORDER BY week_code DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*Integration{}
	for rows.Next() {
		in, err := scanIntegration(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, in)
	}
	return out, rows.Err()
}

// IntegrationResult is what Integrate produced.
type IntegrationResult struct {
	Integration *Integration
	Global      *Dataset
	Local       *Dataset
	Merged      *Dataset
	// RequestedWeeks is the selected cutoff (26 for AO, 61 for BX).
	RequestedWeeks int
	// MergedWeeks is the number of weeks that survived the cutoff and the
	// Global/Local week-code intersection.
	MergedWeeks int
	// Warnings explains a shortened range; the caller shows it in a dialog and
	// writes it to the log.
	Warnings []string
}

// Integrate combines the current Global and Local datasets of a week into a
// merged dataset and a new integration row. It never modifies the inputs.
func (s *Store) Integrate(week string, weekLimit int) (*IntegrationResult, error) {
	if _, err := mustWeek(week); err != nil {
		return nil, err
	}
	if weekLimit <= 0 || weekLimit > mps.MaxWeekColumns {
		weekLimit = mps.MaxWeekColumns
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	draft, err := scanDraftTx(tx)
	if err != nil {
		return nil, err
	}
	if draft != nil {
		return nil, fmt.Errorf("请先完成或丢弃当前 Global 导入")
	}
	global, err := currentTx(tx, week, DatasetGlobal)
	if err != nil {
		return nil, err
	}
	if global == nil {
		return nil, fmt.Errorf("周码 %s 还没有完成导入的 Global 数据", week)
	}
	local, err := currentTx(tx, week, DatasetLocal)
	if err != nil {
		return nil, err
	}
	if local == nil {
		return nil, fmt.Errorf("周码 %s 还没有导入 Local 数据", week)
	}
	globalRows, err := datasetRowsTx(tx, global.ID)
	if err != nil {
		return nil, err
	}
	localRows, err := datasetRowsTx(tx, local.ID)
	if err != nil {
		return nil, err
	}
	globalAvailable := len(global.WeekCodes)
	localAvailable := len(local.WeekCodes)
	globalCodes := global.WeekCodes
	if len(globalCodes) > weekLimit {
		globalCodes = globalCodes[:weekLimit]
	}
	localCodes := local.WeekCodes
	if len(localCodes) > weekLimit {
		localCodes = localCodes[:weekLimit]
	}
	globalRows = trimStagedRows(globalRows, weekLimit)
	localRows = trimStagedRows(localRows, weekLimit)
	header := mps.Header{IndexNames: global.IndexNames, Weeks: parseWeeks(globalCodes)}
	localHeader := mps.Header{IndexNames: local.IndexNames, Weeks: parseWeeks(localCodes)}
	localSheet := &mps.LocalSheet{
		Header:    localHeader,
		Rows:      toMPSRows(localRows),
		RowsTotal: len(localRows),
		RowsKept:  len(localRows),
	}
	merged, err := mps.MergeLocal(header, toMPSRows(globalRows), localSheet)
	if err != nil {
		return nil, err
	}
	warnings := []string{}
	if globalAvailable < weekLimit || localAvailable < weekLimit || len(merged.Header.Weeks) < weekLimit {
		warnings = append(warnings, fmt.Sprintf(
			"选择截止 %d 周：Global 提供 %d 周，Local 提供 %d 周，实际按交集整合 %d 周。",
			weekLimit, globalAvailable, localAvailable, len(merged.Header.Weeks)))
	}
	localName, err := datasetFirstNameTx(tx, local.ID)
	if err != nil {
		return nil, err
	}
	mergedRows := stagedRowsFromMerge(merged.Rows, merged.Sources, globalRows, localName)
	version, err := nextVersionTx(tx, week, DatasetMerged)
	if err != nil {
		return nil, err
	}
	mergedCodes := make([]string, len(merged.Header.Weeks))
	for i, w := range merged.Header.Weeks {
		mergedCodes[i] = w.Code
	}
	weekCodesJSON, _ := json.Marshal(mergedCodes)
	indexNamesJSON, _ := json.Marshal(global.IndexNames)
	// One timestamp for the merged dataset and the integration row: history
	// shows the dataset's own creation time, and it has to agree with the
	// integration it belongs to.
	integratedAt := Now()
	res, err := tx.Exec(
		`INSERT INTO dataset(week_code, week_start, kind, version, status, row_count,
		                     file_count, failed_count, week_codes, index_names, imported_at, params,
		                     merged_from_global, merged_from_local)
		 VALUES(?,?,'merged',?,'ready',?,?,0,?,?,?,?,?,?)`,
		week, global.WeekStart, version, len(mergedRows), global.FileCount+local.FileCount,
		string(weekCodesJSON), string(indexNamesJSON), integratedAt, global.Params, global.ID, local.ID)
	if err != nil {
		return nil, err
	}
	mergedID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := writeDatasetRowsTx(tx, mergedID, 0, mergedRows, nil, nil); err != nil {
		return nil, err
	}
	// The merged dataset has no workbooks of its own; its file count is the two
	// inputs' total and was set on insert, so refreshCounts (which counts
	// dataset_file rows) must not run here.

	var oldGlobal, oldLocal, oldMerged sql.NullInt64
	_ = tx.QueryRow(`SELECT global_id, local_id, merged_id FROM integration WHERE week_code=?`, week).
		Scan(&oldGlobal, &oldLocal, &oldMerged)
	if _, err := tx.Exec(
		`INSERT INTO integration(week_code, global_id, local_id, merged_id, integrated_at, params)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(week_code) DO UPDATE SET
		   global_id=excluded.global_id, local_id=excluded.local_id,
		   merged_id=excluded.merged_id, integrated_at=excluded.integrated_at,
		   params=excluded.params`,
		week, global.ID, local.ID, mergedID, integratedAt, global.Params); err != nil {
		return nil, err
	}
	// Old versions the integration no longer points at are unreachable history
	// now; drop them so the database does not grow a copy per re-integration.
	for _, old := range []sql.NullInt64{oldGlobal, oldLocal, oldMerged} {
		if !old.Valid {
			continue
		}
		if old.Int64 == global.ID || old.Int64 == local.ID || old.Int64 == mergedID {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM dataset WHERE id=?`, old.Int64); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	out := &IntegrationResult{Global: global, Local: local, Merged: &Dataset{
		ID: mergedID, WeekCode: week, WeekStart: global.WeekStart, Kind: DatasetMerged,
		Version: version, Status: DatasetReady, RowCount: len(mergedRows),
		FileCount: global.FileCount + local.FileCount, WeekCodes: mergedCodes,
		IndexNames: global.IndexNames, ImportedAt: integratedAt, Params: global.Params,
	}, RequestedWeeks: weekLimit, MergedWeeks: len(mergedCodes), Warnings: warnings}
	out.Integration, err = s.Integration(week)
	return out, err
}

// trimStagedRows applies the integration cutoff to one input. The rows keep
// only the first weekLimit week values, their aligned style/CF-colour entries,
// and comments that still point inside the kept columns.
func trimStagedRows(rows []StagedRow, weekLimit int) []StagedRow {
	for i := range rows {
		if len(rows[i].Weeks) > weekLimit {
			rows[i].Weeks = rows[i].Weeks[:weekLimit]
		}
		if len(rows[i].StyleIDs) > mps.IndexCols+weekLimit {
			rows[i].StyleIDs = rows[i].StyleIDs[:mps.IndexCols+weekLimit]
		}
		if len(rows[i].CFColors) > weekLimit {
			rows[i].CFColors = rows[i].CFColors[:weekLimit]
		}
		for col := range rows[i].Comments {
			if col > mps.IndexCols+weekLimit {
				delete(rows[i].Comments, col)
			}
		}
	}
	return rows
}

func scanDraftTx(q sqlQueryer) (*Dataset, error) {
	d, err := scanDataset(q.QueryRow(
		`SELECT ` + datasetCols + ` FROM dataset WHERE status='draft' ORDER BY id DESC LIMIT 1`))
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return d, err
}

func datasetRowsTx(q sqlQueryer, id int64) ([]StagedRow, error) {
	// Loaded before the cursor: the transaction has one connection, so a second
	// query cannot run while the rows below are open.
	cfPatterns := map[int64]string{}
	cfRows, err := q.Query(`SELECT id, payload FROM cf_pattern`)
	if err != nil {
		return nil, err
	}
	for cfRows.Next() {
		var cid int64
		var payload string
		if err := cfRows.Scan(&cid, &payload); err != nil {
			cfRows.Close()
			return nil, err
		}
		cfPatterns[cid] = payload
	}
	if err := cfRows.Err(); err != nil {
		cfRows.Close()
		return nil, err
	}
	cfRows.Close()

	cols := []string{"seq", "file_name", "src_row"}
	for i := 1; i <= mps.IndexCols; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
	}
	cols = append(cols, "weeks", "style_ids", "cf_id", "comments", "cf_colors")
	rows, err := q.Query(fmt.Sprintf(`SELECT %s FROM dataset_row WHERE dataset_id=? ORDER BY seq`,
		strings.Join(cols, ",")), id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StagedRow
	for rows.Next() {
		var r StagedRow
		var seq int
		var weeksJSON, styleJSON string
		var cfID sql.NullInt64
		var commentsJSON, cfColorsJSON sql.NullString
		idx := make([]sql.NullString, mps.IndexCols)
		scan := []any{&seq, &r.FileName, &r.SourceRow}
		for i := range idx {
			scan = append(scan, &idx[i])
		}
		scan = append(scan, &weeksJSON, &styleJSON, &cfID, &commentsJSON, &cfColorsJSON)
		if err := rows.Scan(scan...); err != nil {
			return nil, err
		}
		for i := range idx {
			r.Index[i] = idx[i].String
		}
		_ = json.Unmarshal([]byte(weeksJSON), &r.Weeks)
		_ = json.Unmarshal([]byte(styleJSON), &r.StyleIDs)
		if commentsJSON.Valid && commentsJSON.String != "" {
			_ = json.Unmarshal([]byte(commentsJSON.String), &r.Comments)
		}
		if cfColorsJSON.Valid && cfColorsJSON.String != "" {
			_ = json.Unmarshal([]byte(cfColorsJSON.String), &r.CFColors)
		}
		if cfID.Valid {
			if payload, ok := cfPatterns[cfID.Int64]; ok {
				r.CF = mps.UnmarshalRow(payload)
			}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// datasetFirstNameTx is the first successful workbook of a dataset, used to
// attribute the rows that came from it.
func datasetFirstNameTx(q sqlQueryer, id int64) (string, error) {
	var name string
	err := q.QueryRow(
		`SELECT name FROM dataset_file WHERE dataset_id=? AND status='ok' ORDER BY id LIMIT 1`, id).Scan(&name)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return name, err
}

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

func parseWeeks(codes []string) []mps.Week {
	out := make([]mps.Week, 0, len(codes))
	for _, c := range codes {
		if w, err := mps.ParseWeekCode(c); err == nil {
			out = append(out, w)
		}
	}
	return out
}

// DiscardWeekImports deletes a week's draft and any ready datasets that are not
// referenced by its integration. History (the integration and its datasets) is
// never touched.
func (s *Store) DiscardWeekImports(week string) (int, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var global, local, merged sql.NullInt64
	_ = tx.QueryRow(`SELECT global_id, local_id, merged_id FROM integration WHERE week_code=?`, week).
		Scan(&global, &local, &merged)
	keep := map[int64]bool{}
	for _, id := range []sql.NullInt64{global, local, merged} {
		if id.Valid {
			keep[id.Int64] = true
		}
	}
	rows, err := tx.Query(`SELECT id FROM dataset WHERE week_code=?`, week)
	if err != nil {
		return 0, err
	}
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	n := 0
	for _, id := range ids {
		if keep[id] {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM dataset WHERE id=?`, id); err != nil {
			return 0, err
		}
		n++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return n, nil
}

// ReferencedByIntegration reports whether an integration points at a dataset.
func (s *Store) ReferencedByIntegration(id int64) (bool, error) {
	var n int
	err := s.db.QueryRow(
		`SELECT COUNT(1) FROM integration
		 WHERE global_id=? OR local_id=? OR merged_id=?`, id, id, id).Scan(&n)
	return n > 0, err
}

// DeleteDataset removes a dataset and its rows, refusing one an integration uses.
func (s *Store) DeleteDataset(id int64) error {
	used, err := s.ReferencedByIntegration(id)
	if err != nil {
		return err
	}
	if used {
		return fmt.Errorf("该数据集已被整合记录引用，不能删除")
	}
	_, err = s.db.Exec(`DELETE FROM dataset WHERE id=?`, id)
	return err
}

// filepathBase is a tiny wrapper so this file does not need path/filepath just
// for the file-id lookup.
func filepathBase(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[i+1:]
		}
	}
	return p
}
