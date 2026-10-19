package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"project_clear/internal/mps"
)

// DatasetQuery is a page request for one dataset's rows.
type DatasetQuery struct {
	DatasetID int64
	Page      int
	PageSize  int
	Search    string
	SortField string
	SortDesc  bool
	Filters   map[string]string
}

// GridRow is one row of the data grid.
type GridRow struct {
	Seq      int64                 `json:"seq"`
	Index    [mps.IndexCols]string `json:"index"`
	Weeks    []string              `json:"weeks"`
	WeekMeta []CellMeta            `json:"weekMeta"`
	FileName string                `json:"fileName"`
	SrcRow   int                   `json:"srcRow"`
}

// CellMeta is the per-week display detail: fill colour and note.
type CellMeta struct {
	Color   string `json:"color,omitempty"`
	Comment string `json:"comment,omitempty"`
	Author  string `json:"author,omitempty"`
}

// GridResult is one page of rows.
type GridResult struct {
	Rows     []GridRow `json:"rows"`
	Total    int       `json:"total"`
	Page     int       `json:"page"`
	PageSize int       `json:"pageSize"`
}

// QueryDataset returns one page of a dataset.
func (s *Store) QueryDataset(q DatasetQuery) (*GridResult, error) {
	d, err := s.Dataset(q.DatasetID)
	if err != nil {
		return nil, err
	}
	if d == nil {
		return nil, fmt.Errorf("数据集不存在")
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

	where := []string{"dataset_id = ?"}
	args := []any{d.ID}
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
		col := datasetWeekExpr(d.WeekCodes, code)
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
			where = append(where, fmt.Sprintf("(%s IS NULL OR %s = '')", col, col))
		case "notempty":
			where = append(where, fmt.Sprintf("(%s IS NOT NULL AND %s <> '')", col, col))
		}
	}
	condition := strings.Join(where, " AND ")

	var total int
	if err := s.db.QueryRow(
		`SELECT COUNT(1) FROM dataset_row WHERE `+condition, args...).Scan(&total); err != nil {
		return nil, err
	}

	// Loaded before the cursor: the pool has one connection, so a second query
	// while this one is open would wait for itself.
	colors, err := s.fillColors()
	if err != nil {
		return nil, err
	}

	order := datasetSort(q.SortField, q.SortDesc, d.WeekCodes)
	rows, err := s.db.Query(fmt.Sprintf(
		`SELECT seq, c1,c2,c3,c4,c5,c6,c7,c8,c9,c10,c11,c12,c13,c14,c15,
		        weeks, style_ids, comments, cf_colors, file_name, src_row
		 FROM dataset_row WHERE %s ORDER BY %s LIMIT ? OFFSET ?`,
		condition, order), append(args, size, (page-1)*size)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := &GridResult{Rows: []GridRow{}, Total: total, Page: page, PageSize: size}
	for rows.Next() {
		var gr GridRow
		var weeksJSON, styleJSON string
		var commentsJSON, cfColorsJSON, fileName sql.NullString
		var srcRow sql.NullInt64
		idx := make([]sql.NullString, mps.IndexCols)
		scan := []any{&gr.Seq}
		for i := range idx {
			scan = append(scan, &idx[i])
		}
		scan = append(scan, &weeksJSON, &styleJSON, &commentsJSON, &cfColorsJSON, &fileName, &srcRow)
		if err := rows.Scan(scan...); err != nil {
			return nil, err
		}
		for i := range idx {
			gr.Index[i] = idx[i].String
		}
		var values []string
		s.decodeJSON("周数据", weeksJSON, &values)
		gr.Weeks = make([]string, len(d.WeekCodes))
		for i := range gr.Weeks {
			if i < len(values) {
				gr.Weeks[i] = values[i]
			}
		}
		gr.WeekMeta = s.buildWeekMeta(d.WeekCodes, styleJSON, commentsJSON.String, cfColorsJSON.String, colors)
		gr.FileName = fileName.String
		gr.SrcRow = int(srcRow.Int64)
		out.Rows = append(out.Rows, gr)
	}
	return out, rows.Err()
}

// datasetWeekExpr is the JSON expression for one week column of a dataset.
func datasetWeekExpr(weekCodes []string, code string) string {
	for i, wc := range weekCodes {
		if wc == code {
			return fmt.Sprintf("CAST(json_extract(weeks, '$[%d]') AS REAL)", i)
		}
	}
	return "NULL"
}

// datasetSort validates a sort field against the columns this dataset has.
func datasetSort(field string, desc bool, weekCodes []string) string {
	dir := ""
	if desc {
		dir = " DESC"
	}
	col := "seq"
	switch field {
	case "", "seq":
		col = "seq"
	case "fileName", "file_name":
		col = "file_name"
	case "srcRow", "src_row":
		col = "src_row"
	default:
		if strings.HasPrefix(field, "c") {
			if n, err := strconv.Atoi(strings.TrimPrefix(field, "c")); err == nil && n >= 1 && n <= mps.IndexCols {
				col = fmt.Sprintf("c%d", n)
			}
		} else if strings.HasPrefix(field, "wk_") {
			code := strings.TrimPrefix(field, "wk_")
			expr := datasetWeekExpr(weekCodes, code)
			if expr != "NULL" {
				col = expr
			}
		}
	}
	return col + dir + ", seq"
}

func (s *Store) buildWeekMeta(weekCodes []string, styleJSON, commentsJSON, cfColorsJSON string, colors map[int]string) []CellMeta {
	var styleIDs []int
	s.decodeJSON("单元格样式", styleJSON, &styleIDs)
	var comments mps.CommentMap
	s.decodeJSON("批注", commentsJSON, &comments)
	var cfColors []string
	s.decodeJSON("条件格式颜色", cfColorsJSON, &cfColors)
	out := make([]CellMeta, len(weekCodes))
	for i := range weekCodes {
		col := mps.FirstWeekCol + i
		if i < len(cfColors) && cfColors[i] != "" {
			out[i].Color = cfColors[i]
		} else if col-1 < len(styleIDs) {
			out[i].Color = colors[styleIDs[col-1]]
		}
		if c, ok := comments[col]; ok {
			out[i].Comment = c.Text
			out[i].Author = c.Author
		}
	}
	return out
}

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

// ExportRow is a dataset row with everything needed to rebuild it.
type ExportRow struct {
	Seq       int64
	Index     [mps.IndexCols]string
	Weeks     []any // float64, string or nil
	StyleIDs  []int
	CF        mps.CFRow
	Comments  mps.CommentMap
	CFColors  []string
	FileName  string
	SourceRow int
}

// ExportRows reads a dataset in full, for export.
func (s *Store) ExportRows(datasetID int64) (*Dataset, []ExportRow, error) {
	d, err := s.Dataset(datasetID)
	if err != nil {
		return nil, nil, err
	}
	if d == nil {
		return nil, nil, fmt.Errorf("数据集不存在")
	}
	rows, err := s.DatasetRows(datasetID)
	if err != nil {
		return nil, nil, err
	}
	out := make([]ExportRow, 0, len(rows))
	for _, r := range rows {
		weeks := make([]any, len(d.WeekCodes))
		for i := range weeks {
			if i < len(r.Weeks) {
				weeks[i] = weekValue(r.Weeks[i])
			}
		}
		out = append(out, ExportRow{
			Index: r.Index, Weeks: weeks, StyleIDs: r.StyleIDs,
			CF: r.CF, Comments: r.Comments, CFColors: r.CFColors,
			SourceRow: r.SourceRow, FileName: r.FileName,
		})
	}
	return d, out, nil
}
