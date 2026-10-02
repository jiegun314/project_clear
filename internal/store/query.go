package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"project_clear/internal/mps"
)

// Query describes a page of the data grid. The same shape serves the staging
// area and any committed week, so the UI has one code path.
type Query struct {
	// Source is "" for the staging area or a week code such as "2639".
	Source string
	// Page is 1-based.
	Page int
	// PageSize caps the rows returned.
	PageSize int
	// Search matches the index columns a planner would type into.
	Search string
	// SortField is c1..c15, "seq", or "wk_<code>".
	SortField string
	// SortDesc flips the order.
	SortDesc bool
	// Filters restricts specific week columns to a numeric threshold, e.g.
	// {"2639": ">0"}.
	Filters map[string]string
}

// GridRow is one row as the frontend receives it.
type GridRow struct {
	Seq      int64                 `json:"seq"`
	Index    [mps.IndexCols]string `json:"index"`
	Weeks    []string              `json:"weeks"`
	WeekMeta []CellMeta            `json:"weekMeta"`
	FileName string                `json:"fileName"`
	SrcRow   int                   `json:"srcRow"`
}

// CellMeta carries what the grid needs beyond the number itself: the fill
// colour the source cell was painted with, and the note attached to it. Both
// are aligned to the week columns, one entry per week.
type CellMeta struct {
	Color   string `json:"color,omitempty"`
	Comment string `json:"comment,omitempty"`
	Author  string `json:"author,omitempty"`
}

// GridResult is a page plus the totals the grid needs.
type GridResult struct {
	Rows     []GridRow `json:"rows"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}

// sourceSQL resolves a query source into a table name and a flag telling
// whether it is the staging area.
func (s *Store) sourceSQL(q Query) (table string, staging bool, weekCodes []string, indexNames []string, err error) {
	if q.Source == "" {
		sum, err := s.LoadStaging()
		if err != nil {
			return "", false, nil, nil, err
		}
		if !sum.HasStaging {
			return "", false, nil, nil, fmt.Errorf("没有临时数据")
		}
		SetStagingWeekOrder(sum.WeekCodes)
		return "stg_row", true, sum.WeekCodes, sum.IndexNames, nil
	}
	code, err := mustWeek(q.Source)
	if err != nil {
		return "", false, nil, nil, err
	}
	entry, err := s.ArchiveEntryFor(code)
	if err != nil {
		return "", false, nil, nil, err
	}
	if entry == nil {
		return "", false, nil, nil, fmt.Errorf("周码 %s 尚未整合入库", code)
	}
	return entry.TableName, false, entry.WeekCodes, entry.IndexNames, nil
}

// weekExpr is the SQL expression for a week cell. Committed tables have a real
// column; the staging area keeps weeks in one JSON array, so the value is
// pulled out with json_extract and cast, which still sorts numerically.
func weekExpr(staging bool, code string) string {
	col := WeekColumnName(code)
	if !staging {
		return quoteIdent(col)
	}
	for i, wc := range stagingWeekOrder {
		if wc == code {
			return fmt.Sprintf("CAST(json_extract(weeks, '$[%d]') AS REAL)", i)
		}
	}
	return "NULL"
}

// stagingWeekOrder is the week order used to decode the staging JSON array.
var stagingWeekOrder []string

// SetStagingWeekOrder tells the query layer how the staging JSON array is laid
// out. It is set once per staging batch.
func SetStagingWeekOrder(codes []string) { stagingWeekOrder = codes }

// sortColumn validates a sort target against the columns that actually exist,
// so a crafted request cannot reach SQL as a raw identifier.
func sortColumn(field string, weekCodes []string, staging bool) string {
	switch field {
	case "":
		return "seq"
	case "seq":
		return "seq"
	case "fileName", "file_name":
		return "file_name"
	case "srcRow", "src_row":
		return "src_row"
	}
	if strings.HasPrefix(field, "c") {
		if n, err := strconv.Atoi(strings.TrimPrefix(field, "c")); err == nil && n >= 1 && n <= mps.IndexCols {
			return fmt.Sprintf("c%d", n)
		}
	}
	if strings.HasPrefix(field, "wk_") {
		code := strings.TrimPrefix(field, "wk_")
		for _, wc := range weekCodes {
			if wc == code {
				return weekExpr(staging, code)
			}
		}
	}
	return "seq"
}

// QueryRows returns one page of rows.
func (s *Store) QueryRows(q Query) (*GridResult, error) {
	table, staging, weekCodes, _, err := s.sourceSQL(q)
	if err != nil {
		return nil, err
	}
	page := q.Page
	if page < 1 {
		page = 1
	}
	size := q.PageSize
	if size < 1 {
		size = 200
	}
	if size > 5000 {
		size = 5000
	}

	where := []string{}
	var args []any
	if staging {
		where = append(where, "batch_id = (SELECT id FROM batch WHERE status='staging' ORDER BY id DESC LIMIT 1)")
	}
	if kw := strings.TrimSpace(q.Search); kw != "" {
		like := "%" + kw + "%"
		var parts []string
		for c := 1; c <= mps.IndexCols; c++ {
			parts = append(parts, fmt.Sprintf("c%d LIKE ?", c))
			args = append(args, like)
		}
		where = append(where, "("+strings.Join(parts, " OR ")+")")
	}
	for code, cond := range q.Filters {
		if !ValidWeekCode(code) {
			continue
		}
		col := weekExpr(staging, code)
		op, value, ok := splitCondition(cond)
		if !ok {
			continue
		}
		switch op {
		case ">", ">=", "<", "<=", "=", "!=":
			n, err := strconv.ParseFloat(value, 64)
			if err != nil {
				continue
			}
			where = append(where, fmt.Sprintf("%s %s ?", col, op))
			args = append(args, n)
		case "empty":
			where = append(where, col+" IS NULL")
		case "notempty":
			where = append(where, col+" IS NOT NULL")
		}
	}
	clause := ""
	if len(where) > 0 {
		clause = " WHERE " + strings.Join(where, " AND ")
	}

	var total int
	if err := s.db.QueryRow(fmt.Sprintf(`SELECT COUNT(*) FROM %s%s`, quoteIdent(table), clause), args...).
		Scan(&total); err != nil {
		return nil, err
	}

	order := sortColumn(q.SortField, weekCodes, staging)
	dir := "ASC"
	if q.SortDesc {
		dir = "DESC"
	}
	// Loaded before the row cursor opens: the pool deliberately holds one
	// connection, so a second query cannot run while rows are streaming.
	colors, err := s.fillColors()
	if err != nil {
		return nil, err
	}
	// seq breaks ties so paging is stable.
	sqlText := fmt.Sprintf(`SELECT %s FROM %s%s ORDER BY %s %s, seq ASC LIMIT ? OFFSET ?`,
		columnList(staging, weekCodes), quoteIdent(table), clause, order, dir)

	// LIMIT/OFFSET values must not reuse the filter placeholders.
	execArgs := append(append([]any{}, args...), size, (page-1)*size)
	rows, err := s.db.Query(sqlText, execArgs...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := &GridResult{Rows: []GridRow{}, Total: total, Page: page, PageSize: size}
	for rows.Next() {
		gr, err := scanGridRow(rows, staging, weekCodes, colors)
		if err != nil {
			return nil, err
		}
		out.Rows = append(out.Rows, gr)
	}
	return out, rows.Err()
}

func columnList(staging bool, weekCodes []string) string {
	cols := []string{"seq"}
	for i := 1; i <= mps.IndexCols; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
	}
	if !staging {
		for _, wc := range weekCodes {
			cols = append(cols, WeekColumnName(wc))
		}
	} else {
		// The staging area keeps every week in one JSON array instead of one
		// column per week; it has to be selected or the grid shows blanks.
		cols = append(cols, "weeks")
	}
	cols = append(cols, "style_ids", "comments", "file_name", "src_row")
	return strings.Join(cols, ",")
}

func scanGridRow(rows *sql.Rows, staging bool, weekCodes []string, colors map[int]string) (GridRow, error) {
	var gr GridRow
	seq := &gr.Seq
	scans := make([]any, 0, 4+mps.IndexCols+len(weekCodes))
	scans = append(scans, seq)
	texts := make([]sql.NullString, mps.IndexCols)
	for i := range texts {
		scans = append(scans, &texts[i])
	}
	var weekVals []sql.NullString
	var weeksJSON sql.NullString
	if !staging {
		weekVals = make([]sql.NullString, len(weekCodes))
		for i := range weekVals {
			scans = append(scans, &weekVals[i])
		}
	} else {
		scans = append(scans, &weeksJSON)
	}
	var styleJSON, commentsJSON sql.NullString
	var fileName sql.NullString
	var srcRow sql.NullInt64
	scans = append(scans, &styleJSON, &commentsJSON, &fileName, &srcRow)
	if err := rows.Scan(scans...); err != nil {
		return gr, err
	}
	for i := range texts {
		gr.Index[i] = texts[i].String
	}
	gr.Weeks = make([]string, len(weekCodes))
	if staging {
		var values []string
		_ = json.Unmarshal([]byte(weeksJSON.String), &values)
		for i := range gr.Weeks {
			if i < len(values) {
				gr.Weeks[i] = values[i]
			}
		}
	} else {
		for i, v := range weekVals {
			gr.Weeks[i] = nullToCell(v)
		}
	}
	gr.WeekMeta = buildWeekMeta(weekCodes, styleJSON.String, commentsJSON.String, colors)
	gr.FileName = fileName.String
	gr.SrcRow = int(srcRow.Int64)
	return gr, nil
}

// buildWeekMeta aligns the row's interned style ids and its notes with the week
// columns. Both are stored against absolute column numbers, so the week at
// index i corresponds to entry FirstWeekCol-1+i.
func buildWeekMeta(weekCodes []string, styleJSON, commentsJSON string, colors map[int]string) []CellMeta {
	var styleIDs []int
	if styleJSON != "" {
		_ = json.Unmarshal([]byte(styleJSON), &styleIDs)
	}
	var comments mps.CommentMap
	if commentsJSON != "" {
		_ = json.Unmarshal([]byte(commentsJSON), &comments)
	}
	out := make([]CellMeta, len(weekCodes))
	for i := range weekCodes {
		col := mps.FirstWeekCol + i
		if col-1 < len(styleIDs) {
			out[i].Color = colors[styleIDs[col-1]]
		}
		if c, ok := comments[col]; ok {
			out[i].Comment = c.Text
			out[i].Author = c.Author
		}
	}
	return out
}

// fillColors maps every interned style id onto the background colour it paints,
// as #RRGGBB. Styles that paint nothing are simply absent from the map, which
// is what the grid renders as a plain cell.
func (s *Store) fillColors() (map[int]string, error) {
	list, err := s.LoadStyles()
	if err != nil {
		return nil, err
	}
	out := make(map[int]string, len(list))
	for _, e := range list {
		if hex := fillColorOf(e.Sig); hex != "" {
			out[e.ID] = hex
		}
	}
	return out, nil
}

// fillColorOf reads the pattern fill out of an excelize style signature.
func fillColorOf(sig json.RawMessage) string {
	var st struct {
		Fill struct {
			Pattern int      `json:"Pattern"`
			Color   []string `json:"Color"`
		} `json:"Fill"`
	}
	if err := json.Unmarshal(sig, &st); err != nil {
		return ""
	}
	if st.Fill.Pattern == 0 || len(st.Fill.Color) == 0 {
		return ""
	}
	return normaliseColor(st.Fill.Color[0])
}

// normaliseColor turns the ARGB or RGB string Excel stores into a #RRGGBB CSS
// colour, rejecting the fully transparent value.
func normaliseColor(v string) string {
	v = strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(v, "#")))
	switch len(v) {
	case 6:
		return "#" + v
	case 8:
		if v[:2] == "00" {
			return ""
		}
		return "#" + v[2:]
	}
	return ""
}

func nullToCell(v sql.NullString) string {
	if !v.Valid {
		return ""
	}
	return v.String
}

func splitCondition(cond string) (op, value string, ok bool) {
	cond = strings.TrimSpace(cond)
	switch {
	case cond == "empty":
		return "empty", "", true
	case cond == "notempty":
		return "notempty", "", true
	}
	for _, o := range []string{">=", "<=", "!=", "=", ">", "<"} {
		if strings.HasPrefix(cond, o) {
			return o, strings.TrimSpace(cond[len(o):]), true
		}
	}
	return "", "", false
}

// ExportRow is a committed row with everything needed to rebuild it.
type ExportRow struct {
	Seq       int64
	Index     [mps.IndexCols]string
	Weeks     []any // float64, string or nil
	StyleIDs  []int
	CF        mps.CFRow
	Comments  mps.CommentMap
	FileName  string
	SourceRow int
}

// FetchExportRows reads a committed week in full, for export.
func (s *Store) FetchExportRows(weekCode string) (*ArchiveEntry, []ExportRow, error) {
	entry, err := s.ArchiveEntryFor(weekCode)
	if err != nil {
		return nil, nil, err
	}
	if entry == nil {
		return nil, nil, fmt.Errorf("周码 %s 尚未整合入库", weekCode)
	}
	cols := []string{"seq"}
	for i := 1; i <= mps.IndexCols; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
	}
	for _, wc := range entry.WeekCodes {
		cols = append(cols, WeekColumnName(wc))
	}
	cols = append(cols, "cf_id", "file_name", "src_row", "style_ids", "comments")

	// Load the format programs up front: querying them while the row cursor
	// is open would need a second connection, and the pool allows only one.
	cfPatterns, err := s.LoadCFPatterns()
	if err != nil {
		return nil, nil, err
	}

	rows, err := s.db.Query(fmt.Sprintf(`SELECT %s FROM %s ORDER BY seq`,
		strings.Join(cols, ","), quoteIdent(entry.TableName)))
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []ExportRow
	for rows.Next() {
		var r ExportRow
		texts := make([]sql.NullString, mps.IndexCols)
		weeks := make([]sql.RawBytes, len(entry.WeekCodes))
		var cfID sql.NullInt64
		var fileName sql.NullString
		var srcRow sql.NullInt64
		var styleJSON sql.NullString
		var commentsJSON sql.NullString
		scans := []any{&r.Seq}
		for i := range texts {
			scans = append(scans, &texts[i])
		}
		for i := range weeks {
			scans = append(scans, &weeks[i])
		}
		scans = append(scans, &cfID, &fileName, &srcRow, &styleJSON, &commentsJSON)
		if err := rows.Scan(scans...); err != nil {
			return nil, nil, err
		}
		for i := range texts {
			r.Index[i] = texts[i].String
		}
		r.Weeks = make([]any, len(entry.WeekCodes))
		for i := range weeks {
			r.Weeks[i] = decodeWeekCell(weeks[i])
		}
		r.FileName = fileName.String
		r.SourceRow = int(srcRow.Int64)
		if styleJSON.Valid && styleJSON.String != "" {
			_ = json.Unmarshal([]byte(styleJSON.String), &r.StyleIDs)
		}
		if commentsJSON.Valid && commentsJSON.String != "" {
			_ = json.Unmarshal([]byte(commentsJSON.String), &r.Comments)
		}
		if cfID.Valid {
			if payload, ok := cfPatterns[cfID.Int64]; ok {
				r.CF = mps.UnmarshalRow(payload)
			}
		}
		out = append(out, r)
	}
	return entry, out, rows.Err()
}

// FetchStagingExportRows reads the staging area in the same shape as a
// committed week, so 导出 works on data that has been imported but not yet
// integrated — the merged rows the main grid is showing.
func (s *Store) FetchStagingExportRows() (*StagingSummary, []ExportRow, error) {
	sum, err := s.LoadStaging()
	if err != nil {
		return nil, nil, err
	}
	if !sum.HasStaging {
		return nil, nil, fmt.Errorf("没有待导出的临时数据")
	}

	// Loaded up front: the pool allows one connection, so a second query
	// cannot run while the row cursor below is open.
	cfPatterns, err := s.LoadCFPatterns()
	if err != nil {
		return nil, nil, err
	}

	cols := make([]string, 0, mps.IndexCols+7)
	cols = append(cols, "seq")
	for i := 1; i <= mps.IndexCols; i++ {
		cols = append(cols, fmt.Sprintf("c%d", i))
	}
	cols = append(cols, "weeks", "style_ids", "cf_id", "comments", "file_name", "src_row")

	rows, err := s.db.Query(fmt.Sprintf(`SELECT %s FROM stg_row WHERE batch_id=? ORDER BY seq`,
		strings.Join(cols, ",")), sum.BatchID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var out []ExportRow
	for rows.Next() {
		var r ExportRow
		var weeksJSON, styleJSON string
		var cfID sql.NullInt64
		var commentsJSON, fileName sql.NullString
		var srcRow sql.NullInt64
		texts := make([]sql.NullString, mps.IndexCols)
		scans := []any{&r.Seq}
		for i := range texts {
			scans = append(scans, &texts[i])
		}
		scans = append(scans, &weeksJSON, &styleJSON, &cfID, &commentsJSON, &fileName, &srcRow)
		if err := rows.Scan(scans...); err != nil {
			return nil, nil, err
		}
		for i := range texts {
			r.Index[i] = texts[i].String
		}
		var weeks []string
		_ = json.Unmarshal([]byte(weeksJSON), &weeks)
		r.Weeks = make([]any, len(sum.WeekCodes))
		for i := range r.Weeks {
			if i < len(weeks) {
				r.Weeks[i] = weekValue(weeks[i])
			}
		}
		_ = json.Unmarshal([]byte(styleJSON), &r.StyleIDs)
		if commentsJSON.Valid && commentsJSON.String != "" {
			_ = json.Unmarshal([]byte(commentsJSON.String), &r.Comments)
		}
		if cfID.Valid {
			if payload, ok := cfPatterns[cfID.Int64]; ok {
				r.CF = mps.UnmarshalRow(payload)
			}
		}
		r.FileName = fileName.String
		r.SourceRow = int(srcRow.Int64)
		out = append(out, r)
	}
	return sum, out, rows.Err()
}

// decodeWeekCell turns a stored cell back into the value to write out.
func decodeWeekCell(raw []byte) any {
	if len(raw) == 0 {
		return nil
	}
	if n, err := strconv.ParseFloat(string(raw), 64); err == nil {
		return n
	}
	return string(raw)
}

var _ = json.Marshal
