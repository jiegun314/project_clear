package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"project_clear/internal/mps"
)

// StagedFile records the outcome of reading one source workbook.
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

// StagedRow is one merged row waiting to be committed.
type StagedRow struct {
	FileName  string
	SourceRow int
	Index     [mps.IndexCols]string
	Weeks     []string
	StyleIDs  []int
	CF        mps.CFRow
	Comments  map[int]string
}

// StagingInput is everything one import or add action produces.
type StagingInput struct {
	Action      string
	WeekCode    string
	WeekStart   string
	WeekCodes   []string
	IndexNames  []string
	Files       []StagedFile
	Rows        []StagedRow
	Dict        *mps.StyleDict
	TemplateSrc string // path of the workbook to store as the export template
	TemplateNam string
	ParamSnap   string
}

// StagingResult reports what was written.
type StagingResult struct {
	BatchID    int64
	StyleMap   map[int]int
	TemplateID sql.NullInt64
}

// SaveStaging replaces the staging area with a new batch in one transaction,
// so an interrupted import can never leave a half-written batch behind.
func (s *Store) SaveStaging(in StagingInput) (*StagingResult, error) {
	if _, err := mustWeek(in.WeekCode); err != nil {
		return nil, err
	}
	weekCodes, err := json.Marshal(in.WeekCodes)
	if err != nil {
		return nil, err
	}
	indexNames, err := json.Marshal(in.IndexNames)
	if err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// A new import or add supersedes whatever was staged before, but never
	// touches committed weeks.
	if _, err := tx.Exec(`DELETE FROM batch WHERE status='staging'`); err != nil {
		return nil, err
	}

	ok, failed := 0, 0
	kept := 0
	for _, f := range in.Files {
		if f.Status == "ok" {
			ok++
			kept += f.RowsKept
		} else {
			failed++
		}
	}
	res, err := tx.Exec(
		`INSERT INTO batch(created_at, action, week_code, week_start, week_codes, index_names,
		                   status, file_total, file_ok, file_fail, row_total, row_kept, param_snapshot)
		 VALUES(?,?,?,?,?,?, 'staging', ?,?,?,?,?,?)`,
		Now(), in.Action, in.WeekCode, in.WeekStart, string(weekCodes), string(indexNames),
		len(in.Files), ok, failed, totalRows(in.Files), kept, in.ParamSnap)
	if err != nil {
		return nil, err
	}
	batchID, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}

	// The style dictionary is process-wide, so map its local ids onto
	// database ids inside the same transaction.
	styleMap, err := resolveStyleIDsTx(tx, in.Dict)
	if err != nil {
		return nil, err
	}

	for _, f := range in.Files {
		if _, err := tx.Exec(
			`INSERT INTO batch_file(batch_id, path, name, size, status, rows_total, rows_kept, week_code, err)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			batchID, f.Path, f.Name, f.Size, f.Status, f.RowsTotal, f.RowsKept, f.WeekCode, f.Err); err != nil {
			return nil, err
		}
	}

	cfCache := map[string]sql.NullInt64{}
	// The column list and the placeholder list are built together so the two
	// can never drift apart.
	stgCols := []string{"batch_id", "seq", "file_id", "file_name", "src_row"}
	for i := 1; i <= mps.IndexCols; i++ {
		stgCols = append(stgCols, fmt.Sprintf("c%d", i))
	}
	stgCols = append(stgCols, "weeks", "style_ids", "cf_id", "comments")
	ph := make([]string, len(stgCols))
	for i := range ph {
		ph[i] = "?"
	}
	rowStmt, err := tx.Prepare(fmt.Sprintf(
		`INSERT INTO stg_row(%s) VALUES (%s)`, strings.Join(stgCols, ","), strings.Join(ph, ",")))
	if err != nil {
		return nil, err
	}
	defer rowStmt.Close()

	for i, r := range in.Rows {
		weeks := r.Weeks
		if weeks == nil {
			weeks = []string{}
		}
		weeksJSON, err := json.Marshal(weeks)
		if err != nil {
			return nil, err
		}
		styleIDs := make([]int, len(r.StyleIDs))
		for j, id := range r.StyleIDs {
			mapped, ok := styleMap[id]
			if !ok {
				mapped = 0
			}
			styleIDs[j] = mapped
		}
		styleJSON, err := json.Marshal(styleIDs)
		if err != nil {
			return nil, err
		}

		var cfID sql.NullInt64
		if !r.CF.Empty() {
			payload := mps.MarshalRow(r.CF)
			if cached, ok := cfCache[payload]; ok {
				cfID = cached
			} else {
				id, err := cfPatternIDTx(tx, payload)
				if err != nil {
					return nil, err
				}
				cfCache[payload] = id
				cfID = id
			}
		}

		var comments any
		if len(r.Comments) > 0 {
			b, err := json.Marshal(r.Comments)
			if err != nil {
				return nil, err
			}
			comments = string(b)
		}

		rowArgs := make([]any, 0, len(stgCols))
		rowArgs = append(rowArgs, batchID, i, nil, r.FileName, r.SourceRow)
		for _, v := range r.Index {
			rowArgs = append(rowArgs, v)
		}
		rowArgs = append(rowArgs, string(weeksJSON), string(styleJSON), cfID, comments)
		if _, err := rowStmt.Exec(rowArgs...); err != nil {
			return nil, err
		}
	}

	// Store the export template, if this batch produced one.
	var templateID sql.NullInt64
	if in.TemplateSrc != "" {
		week := in.WeekCode
		if _, err := tx.Exec(`DELETE FROM template WHERE week_code=?`, week); err != nil {
			return nil, err
		}
		r, err := tx.Exec(
			`INSERT INTO template(week_code, src_name, stored_path, created_at) VALUES(?,?,?,?)`,
			week, in.TemplateNam, in.TemplateSrc, Now())
		if err != nil {
			return nil, err
		}
		if id, err := r.LastInsertId(); err == nil {
			templateID = sql.NullInt64{Int64: id, Valid: true}
			if _, err := tx.Exec(`UPDATE batch SET template_id=? WHERE id=?`, id, batchID); err != nil {
				return nil, err
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &StagingResult{BatchID: batchID, StyleMap: styleMap, TemplateID: templateID}, nil
}

// hashOf is the short signature key used to intern styles and format programs.
func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

// indexArgs flattens the fixed A..O index block into bind arguments.
func indexArgs(idx [mps.IndexCols]string) []any {
	args := make([]any, mps.IndexCols)
	for i, v := range idx {
		args[i] = v
	}
	return args
}

func totalRows(files []StagedFile) int {
	n := 0
	for _, f := range files {
		n += f.RowsTotal
	}
	return n
}

func resolveStyleIDsTx(tx *sql.Tx, dict *mps.StyleDict) (map[int]int, error) {
	mapping := make(map[int]int, dict.Len())
	for id := 0; id < dict.Len(); id++ {
		sig, ok := dict.Style(id)
		if !ok {
			continue
		}
		dbID, err := internStyleTx(tx, sig)
		if err != nil {
			return nil, err
		}
		mapping[id] = dbID
	}
	return mapping, nil
}

func internStyleTx(tx *sql.Tx, sig json.RawMessage) (int, error) {
	hash := hashOf(sig)
	var id int
	err := tx.QueryRow(`SELECT id FROM cell_style WHERE sig_hash=?`, hash).Scan(&id)
	if err == nil {
		return id, nil
	}
	if err != sql.ErrNoRows {
		return 0, err
	}
	res, err := tx.Exec(`INSERT INTO cell_style(sig_hash, sig) VALUES(?,?)`, hash, string(sig))
	if err != nil {
		return 0, err
	}
	n, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	return int(n), nil
}

func cfPatternIDTx(tx *sql.Tx, payload string) (sql.NullInt64, error) {
	if payload == "" || payload == "null" {
		return sql.NullInt64{}, nil
	}
	hash := hashOf(json.RawMessage(payload))
	var id int64
	err := tx.QueryRow(`SELECT id FROM cf_pattern WHERE sig_hash=?`, hash).Scan(&id)
	if err == nil {
		return sql.NullInt64{Int64: id, Valid: true}, nil
	}
	if err != sql.ErrNoRows {
		return sql.NullInt64{}, err
	}
	res, err := tx.Exec(`INSERT INTO cf_pattern(sig_hash, payload) VALUES(?,?)`, hash, payload)
	if err != nil {
		return sql.NullInt64{}, err
	}
	n, err := res.LastInsertId()
	if err != nil {
		return sql.NullInt64{}, err
	}
	return sql.NullInt64{Int64: n, Valid: true}, nil
}

// StagingSummary describes what is currently staged.
type StagingSummary struct {
	BatchID    int64
	WeekCode   string
	WeekStart  string
	WeekCodes  []string
	IndexNames []string
	FileTotal  int
	FileOK     int
	FileFail   int
	RowTotal   int
	RowKept    int
	TemplateID sql.NullInt64
	HasStaging bool
}

// LoadStaging returns the active staging batch, or HasStaging=false.
func (s *Store) LoadStaging() (*StagingSummary, error) {
	out := &StagingSummary{}
	var weekCodes, indexNames string
	err := s.db.QueryRow(
		`SELECT id, week_code, week_start, week_codes, index_names,
		        file_total, file_ok, file_fail, row_total, row_kept, template_id
		 FROM batch WHERE status='staging' ORDER BY id DESC LIMIT 1`).
		Scan(&out.BatchID, &out.WeekCode, &out.WeekStart, &weekCodes, &indexNames,
			&out.FileTotal, &out.FileOK, &out.FileFail, &out.RowTotal, &out.RowKept, &out.TemplateID)
	if err == sql.ErrNoRows {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.HasStaging = true
	_ = json.Unmarshal([]byte(weekCodes), &out.WeekCodes)
	SetStagingWeekOrder(out.WeekCodes)
	_ = json.Unmarshal([]byte(indexNames), &out.IndexNames)
	return out, nil
}

// ClearStaging drops the staging batch, used when an import fails outright.
func (s *Store) ClearStaging() error {
	_, err := s.db.Exec(`DELETE FROM batch WHERE status='staging'`)
	return err
}

// StagedFileDetail is one row of the per-file report shown after an import.
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

// StagedFiles lists the files of the active staging batch.
func (s *Store) StagedFiles(batchID int64) ([]StagedFileDetail, error) {
	rows, err := s.db.Query(
		`SELECT name, path, size, status, rows_total, rows_kept, week_code, err
		 FROM batch_file WHERE batch_id=? ORDER BY id`, batchID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StagedFileDetail
	for rows.Next() {
		var d StagedFileDetail
		if err := rows.Scan(&d.Name, &d.Path, &d.Size, &d.Status, &d.RowsTotal, &d.RowsKept, &d.WeekCode, &d.Err); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

var _ = strings.TrimSpace
var _ = fmt.Sprintf
