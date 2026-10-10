package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
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
	FileName string
	// FilePath identifies the workbook the row came from. 添加 uses it to
	// replace exactly the rows of a re-added file; callers that only set
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
	Dxf         *mps.DxfDict
	TemplateSrc string // path of the workbook to store as the export template
	TemplateNam string
	ParamSnap   string
}

// StagingResult reports what was written.
type StagingResult struct {
	BatchID    int64
	StyleMap   map[int]int
	TemplateID sql.NullInt64
	// OrphanedTemplates lists the week codes whose stored template row this
	// write removed because their staging area was superseded and they were
	// never integrated. The caller deletes the matching file copies, which a
	// database transaction cannot do.
	OrphanedTemplates []string
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
	// touches committed weeks. The superseded batch's export template goes with
	// it: a template row carries no foreign key to batch, so the cascade does not
	// reach it and it would otherwise stay behind for good.
	superseded, err := stagingWeekCodesTx(tx)
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`DELETE FROM batch WHERE status='staging'`); err != nil {
		return nil, err
	}

	batchID, err := insertBatchTx(tx, in, weekCodes, indexNames)
	if err != nil {
		return nil, err
	}

	// The style dictionary is process-wide, so map its local ids onto
	// database ids inside the same transaction.
	styleMap, err := resolveStyleIDsTx(tx, in.Dict)
	if err != nil {
		return nil, err
	}
	if _, err := resolveDxfIDsTx(tx, in.Dxf); err != nil {
		return nil, err
	}

	fileIDs, err := insertBatchFilesTx(tx, batchID, in.Files)
	if err != nil {
		return nil, err
	}
	if err := writeStagingRowsTx(tx, batchID, 0, in.Rows, styleMap, fileIDs); err != nil {
		return nil, err
	}

	templateID, err := storeTemplateTx(tx, batchID, in)
	if err != nil {
		return nil, err
	}
	orphaned, err := dropUnreachableTemplatesTx(tx, superseded, in.WeekCode)
	if err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &StagingResult{
		BatchID:           batchID,
		StyleMap:          styleMap,
		TemplateID:        templateID,
		OrphanedTemplates: orphaned,
	}, nil
}

// stagingWeekCodesTx lists the week codes of the staging batches a new import is
// about to supersede, so their export templates can be removed with them.
func stagingWeekCodesTx(tx *sql.Tx) ([]string, error) {
	rows, err := tx.Query(`SELECT DISTINCT week_code FROM batch WHERE status='staging'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var weeks []string
	for rows.Next() {
		var wc string
		if err := rows.Scan(&wc); err != nil {
			return nil, err
		}
		weeks = append(weeks, wc)
	}
	return weeks, rows.Err()
}

// dropUnreachableTemplatesTx removes the stored template of every superseded
// week that was never integrated, and returns those week codes so the caller can
// delete the file copies too. A committed week keeps its template: exporting it
// still needs the original workbook, and the archive row is what keeps it
// reachable.
//
// keep is the week being written now. Its template row is managed by
// storeTemplateTx and must not be touched here.
func dropUnreachableTemplatesTx(tx *sql.Tx, weeks []string, keep string) ([]string, error) {
	orphaned := []string{}
	for _, wc := range weeks {
		if wc == keep {
			continue
		}
		var committed int
		if err := tx.QueryRow(`SELECT COUNT(1) FROM archive WHERE week_code=?`, wc).Scan(&committed); err != nil {
			return nil, err
		}
		if committed > 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM template WHERE week_code=?`, wc); err != nil {
			return nil, err
		}
		orphaned = append(orphaned, wc)
	}
	return orphaned, nil
}

// fileCounts summarises what one add/import action produced.
func fileCounts(files []StagedFile) (ok, failed, kept int) {
	for _, f := range files {
		if f.Status == "ok" {
			ok++
			kept += f.RowsKept
		} else {
			failed++
		}
	}
	return ok, failed, kept
}

// insertBatchTx creates the staging batch row itself.
func insertBatchTx(tx *sql.Tx, in StagingInput, weekCodes, indexNames []byte) (int64, error) {
	ok, failed, kept := fileCounts(in.Files)
	res, err := tx.Exec(
		`INSERT INTO batch(created_at, action, week_code, week_start, week_codes, index_names,
		                   status, file_total, file_ok, file_fail, row_total, row_kept, param_snapshot)
		 VALUES(?,?,?,?,?,?, 'staging', ?,?,?,?,?,?)`,
		Now(), in.Action, in.WeekCode, in.WeekStart, string(weekCodes), string(indexNames),
		len(in.Files), ok, failed, totalRows(in.Files), kept, in.ParamSnap)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// insertBatchFilesTx writes the per-file records and returns path -> file id.
// The id is how a staging row is tied back to the workbook it came from, which
// is what makes "添加同一个文件 = 覆盖它原来的数据" possible.
func insertBatchFilesTx(tx *sql.Tx, batchID int64, files []StagedFile) (map[string]int64, error) {
	ids := make(map[string]int64, len(files))
	for _, f := range files {
		res, err := tx.Exec(
			`INSERT INTO batch_file(batch_id, path, name, size, status, rows_total, rows_kept, week_code, err)
			 VALUES(?,?,?,?,?,?,?,?,?)`,
			batchID, f.Path, f.Name, f.Size, f.Status, f.RowsTotal, f.RowsKept, f.WeekCode, f.Err)
		if err != nil {
			return nil, err
		}
		if id, err := res.LastInsertId(); err == nil && f.Path != "" {
			ids[f.Path] = id
		}
	}
	return ids, nil
}

// writeStagingRowsTx appends rows to a batch, numbering them from base so a
// merge continues the existing sequence instead of restarting it.
func writeStagingRowsTx(tx *sql.Tx, batchID int64, base int, rows []StagedRow, styleMap map[int]int, fileIDs map[string]int64) error {
	// The column list and the placeholder list are built together so the two
	// can never drift apart.
	stgCols := []string{"batch_id", "seq", "file_id", "file_name", "src_row"}
	for i := 1; i <= mps.IndexCols; i++ {
		stgCols = append(stgCols, fmt.Sprintf("c%d", i))
	}
	stgCols = append(stgCols, "weeks", "style_ids", "cf_id", "comments", "cf_colors")
	ph := make([]string, len(stgCols))
	for i := range ph {
		ph[i] = "?"
	}
	rowStmt, err := tx.Prepare(fmt.Sprintf(
		`INSERT INTO stg_row(%s) VALUES (%s)`, strings.Join(stgCols, ","), strings.Join(ph, ",")))
	if err != nil {
		return err
	}
	defer rowStmt.Close()

	// Older rows only carry a file name; resolve those when the name is
	// unambiguous inside the batch.
	byName := map[string]int64{}
	nameCount := map[string]int{}
	for path, id := range fileIDs {
		name := filepath.Base(path)
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
				mapped = 0
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
			b, err := json.Marshal(r.Comments)
			if err != nil {
				return err
			}
			comments = string(b)
		}

		var cfColors any
		if hasAny(r.CFColors) {
			b, err := json.Marshal(r.CFColors)
			if err != nil {
				return err
			}
			cfColors = string(b)
		}

		var fileID any
		if id, ok := fileIDs[r.FilePath]; ok && r.FilePath != "" {
			fileID = id
		} else if id, ok := byName[r.FileName]; ok && nameCount[r.FileName] == 1 {
			fileID = id
		}

		rowArgs := make([]any, 0, len(stgCols))
		rowArgs = append(rowArgs, batchID, base+i, fileID, r.FileName, r.SourceRow)
		for _, v := range r.Index {
			rowArgs = append(rowArgs, v)
		}
		rowArgs = append(rowArgs, string(weeksJSON), string(styleJSON), cfID, comments, cfColors)
		if _, err := rowStmt.Exec(rowArgs...); err != nil {
			return err
		}
	}
	return nil
}

// storeTemplateTx records the export template of a batch. Returns an invalid
// id when the batch produced none.
func storeTemplateTx(tx *sql.Tx, batchID int64, in StagingInput) (sql.NullInt64, error) {
	var templateID sql.NullInt64
	if in.TemplateSrc == "" {
		return templateID, nil
	}
	week := in.WeekCode
	if _, err := tx.Exec(`DELETE FROM template WHERE week_code=?`, week); err != nil {
		return templateID, err
	}
	r, err := tx.Exec(
		`INSERT INTO template(week_code, src_name, stored_path, created_at) VALUES(?,?,?,?)`,
		week, in.TemplateNam, in.TemplateSrc, Now())
	if err != nil {
		return templateID, err
	}
	id, err := r.LastInsertId()
	if err != nil {
		return templateID, err
	}
	templateID = sql.NullInt64{Int64: id, Valid: true}
	if _, err := tx.Exec(`UPDATE batch SET template_id=? WHERE id=?`, id, batchID); err != nil {
		return templateID, err
	}
	return templateID, nil
}

// refreshBatchTotalsTx recomputes the aggregate columns from batch_file, so a
// merged batch reports the whole list rather than only the latest add.
func refreshBatchTotalsTx(tx *sql.Tx, batchID int64) error {
	_, err := tx.Exec(
		`UPDATE batch SET
		   file_total=(SELECT COUNT(1) FROM batch_file WHERE batch_id=?),`+
			`  file_ok=(SELECT COUNT(1) FROM batch_file WHERE batch_id=? AND status='ok'),`+
			`  file_fail=(SELECT COUNT(1) FROM batch_file WHERE batch_id=? AND status<>'ok'),`+
			`  row_total=(SELECT COALESCE(SUM(rows_total),0) FROM batch_file WHERE batch_id=?),`+
			`  row_kept=(SELECT COALESCE(SUM(rows_kept),0) FROM batch_file WHERE batch_id=?) `+
			`WHERE id=?`,
		batchID, batchID, batchID, batchID, batchID, batchID)
	return err
}

// MergeStaging adds one 添加 action to the batch that is already staged.
//
// Files that are already in the list stay untouched unless their path is named
// in replace: those are removed first, so re-adding the same workbook
// overwrites its rows instead of duplicating them. Everything remains one
// batch, which keeps the staging list, the row count and 整合 working on the
// merged set. With nothing staged this is exactly SaveStaging.
//
// `replace` holds file *names*: re-importing a workbook replaces the entry with
// the same name no matter which folder this copy came from. Matching on the path
// instead would let the same report staged from two folders be merged twice,
// counting its week values twice.
func (s *Store) MergeStaging(in StagingInput, replace []string) (*StagingResult, error) {
	sum, err := s.LoadStaging()
	if err != nil {
		return nil, err
	}
	if !sum.HasStaging || len(in.Files) == 0 {
		return s.SaveStaging(in)
	}
	if _, err := mustWeek(in.WeekCode); err != nil {
		return nil, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	if len(replace) > 0 {
		ph := make([]string, len(replace))
		replaced := make([]any, 0, len(replace))
		for i, name := range replace {
			ph[i] = "?"
			replaced = append(replaced, name)
		}
		list := strings.Join(ph, ",")
		args := append([]any{sum.BatchID, sum.BatchID}, replaced...)
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM stg_row WHERE batch_id=? AND file_id IN
			   (SELECT id FROM batch_file WHERE batch_id=? AND name IN (%s))`, list), args...); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM stg_row WHERE batch_id=? AND file_id IS NULL AND file_name IN (%s)`, list),
			append([]any{sum.BatchID}, replaced...)...); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(fmt.Sprintf(
			`DELETE FROM batch_file WHERE batch_id=? AND name IN (%s)`, list),
			append([]any{sum.BatchID}, replaced...)...); err != nil {
			return nil, err
		}
	}

	styleMap, err := resolveStyleIDsTx(tx, in.Dict)
	if err != nil {
		return nil, err
	}
	if _, err := resolveDxfIDsTx(tx, in.Dxf); err != nil {
		return nil, err
	}
	fileIDs, err := insertBatchFilesTx(tx, sum.BatchID, in.Files)
	if err != nil {
		return nil, err
	}
	var base int
	if err := tx.QueryRow(
		`SELECT COALESCE(MAX(seq),-1)+1 FROM stg_row WHERE batch_id=?`, sum.BatchID).Scan(&base); err != nil {
		return nil, err
	}
	if err := writeStagingRowsTx(tx, sum.BatchID, base, in.Rows, styleMap, fileIDs); err != nil {
		return nil, err
	}
	if err := refreshBatchTotalsTx(tx, sum.BatchID); err != nil {
		return nil, err
	}

	// The first file of the list owns the export template; later adds reuse it.
	if !sum.TemplateID.Valid {
		tplIn := in
		tplIn.WeekCode = sum.WeekCode
		if _, err := storeTemplateTx(tx, sum.BatchID, tplIn); err != nil {
			return nil, err
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &StagingResult{BatchID: sum.BatchID, StyleMap: styleMap, TemplateID: sum.TemplateID}, nil
}

// hashOf is the short signature key used to intern styles and format programs.
func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:8])
}

func totalRows(files []StagedFile) int {
	n := 0
	for _, f := range files {
		n += f.RowsTotal
	}
	return n
}

// hasAny reports whether at least one entry of the slice is non-empty.
func hasAny(values []string) bool {
	for _, v := range values {
		if v != "" {
			return true
		}
	}
	return false
}

func resolveStyleIDsTx(tx *sql.Tx, dict *mps.StyleDict) (map[int]int, error) {
	// Same guard as resolveDxfIDsTx below: SaveStaging and MergeStaging are
	// public, and a caller that supplies no dictionary must get an empty mapping
	// rather than a nil-pointer panic.
	if dict == nil {
		return map[int]int{}, nil
	}
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

// resolveDxfIDsTx stores the batch's differential formats and returns the id
// each dictionary entry was given.
func resolveDxfIDsTx(tx *sql.Tx, dict *mps.DxfDict) (map[int]int, error) {
	if dict == nil {
		return map[int]int{}, nil
	}
	mapping := make(map[int]int, dict.Len())
	for id := 0; id < dict.Len(); id++ {
		dxf, ok := dict.At(id)
		if !ok {
			continue
		}
		hash := hashOf([]byte(dxf))
		var stored int
		err := tx.QueryRow(`SELECT id FROM dxf_style WHERE sig_hash=?`, hash).Scan(&stored)
		if err == sql.ErrNoRows {
			res, err := tx.Exec(`INSERT INTO dxf_style(sig_hash, payload) VALUES(?,?)`, hash, dxf)
			if err != nil {
				return nil, err
			}
			n, err := res.LastInsertId()
			if err != nil {
				return nil, err
			}
			stored = int(n)
		} else if err != nil {
			return nil, err
		}
		mapping[id] = stored
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
	s.decodeJSON("周码列表", weekCodes, &out.WeekCodes)
	s.decodeJSON("索引表头", indexNames, &out.IndexNames)
	return out, nil
}

// ClearStaging drops the staging batch, used by 清空 and when an import fails
// outright. Rows and per-file records go with it through ON DELETE CASCADE.
//
// The stored export template of a week travels with its data: a template whose
// week was never integrated is temporary too, so it is dropped as well and its
// week code is returned for the caller to delete the file copy. A committed
// week keeps its template, because exporting that week still needs the original
// workbook as the template.
func (s *Store) ClearStaging() ([]string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// The marker has to be read before the batch disappears, otherwise an older
	// committed batch would take its place in the 已导入文件 window.
	var newest int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(id),0) FROM batch`).Scan(&newest); err != nil {
		return nil, err
	}

	rows, err := tx.Query(`SELECT DISTINCT week_code FROM batch WHERE status='staging'`)
	if err != nil {
		return nil, err
	}
	var weeks []string
	for rows.Next() {
		var wc string
		if err := rows.Scan(&wc); err != nil {
			rows.Close()
			return nil, err
		}
		weeks = append(weeks, wc)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()

	if _, err := tx.Exec(`DELETE FROM batch WHERE status='staging'`); err != nil {
		return nil, err
	}
	// 清空 also ends the file list; committed weeks and their records are kept.
	if err := markFilesClearedTx(tx, newest); err != nil {
		return nil, err
	}

	orphaned := []string{}
	for _, wc := range weeks {
		var committed int
		if err := tx.QueryRow(`SELECT COUNT(1) FROM archive WHERE week_code=?`, wc).Scan(&committed); err != nil {
			return nil, err
		}
		if committed > 0 {
			continue
		}
		if _, err := tx.Exec(`DELETE FROM template WHERE week_code=?`, wc); err != nil {
			return nil, err
		}
		orphaned = append(orphaned, wc)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return orphaned, nil
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

// StagingFiles lists the files of the batch that is currently staged. It is
// what the toolbar's 已导入文件 button shows; an empty slice means the list is
// empty (nothing imported, or 清空 was pressed).
func (s *Store) StagingFiles() ([]StagedFileDetail, error) {
	sum, err := s.LoadStaging()
	if err != nil {
		return nil, err
	}
	if !sum.HasStaging {
		return []StagedFileDetail{}, nil
	}
	return s.StagedFiles(sum.BatchID)
}

// metaFilesCleared remembers the newest batch whose file list the user has
// already cleared with 清空, so the toolbar can start from an empty list again
// without deleting anything that was integrated.
const metaFilesCleared = "files_cleared_upto"

// CurrentBatchFiles returns the work set the 已导入文件 window shows: the
// staging batch while it is still open, and after 整合 the batch that was just
// integrated — it stays on screen as the record of that import until the user
// presses 清空 (which only moves the marker forward) or imports something new.
//
// The second and third results are the week code and the batch status
// ("staging" or "committed"); both are empty when there is nothing to show.
func (s *Store) CurrentBatchFiles() ([]StagedFileDetail, string, string, error) {
	var id int64
	var weekCode, status string
	err := s.db.QueryRow(
		`SELECT id, week_code, status FROM batch
		  WHERE id > COALESCE((SELECT CAST(value AS INTEGER) FROM app_meta WHERE key=?), 0)
		  ORDER BY id DESC LIMIT 1`, metaFilesCleared).Scan(&id, &weekCode, &status)
	if err == sql.ErrNoRows {
		return []StagedFileDetail{}, "", "", nil
	}
	if err != nil {
		return nil, "", "", err
	}
	files, err := s.StagedFiles(id)
	if err != nil {
		return nil, "", "", err
	}
	return files, weekCode, status, nil
}

// markFilesClearedTx records 清空 for every batch up to and including upto.
func markFilesClearedTx(tx *sql.Tx, upto int64) error {
	_, err := tx.Exec(
		`INSERT INTO app_meta(key,value) VALUES(?,?)
		 ON CONFLICT(key) DO UPDATE SET value=excluded.value`,
		metaFilesCleared, strconv.FormatInt(upto, 10))
	return err
}
