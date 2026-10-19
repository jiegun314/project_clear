package mps

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// A 本地 MPS workbook (BW_CN_MPS … (Month End)) is the MPS layout with two extra
// columns: J = Base Code and K = SAP Code. Everything after them shifts by two, so
// LOC sits in N rather than L and the week codes start at R rather than P. That is
// why it needs a reader of its own: ReadFile would take US CATALOG for LOC and OH
// for the first week, and the values would merge into the result under the wrong
// column names.
const (
	// LocalFirstWeekCol is where the week codes start on a 本地 sheet (R).
	LocalFirstWeekCol = 18
	// LocalDataRow is the first data row.
	LocalDataRow = 4
	// LocalItemCol is the ITEM column (I).
	LocalItemCol = 9
	// LocalLOCCol is the LOC column (N).
	LocalLOCCol = 14
	// LocalSheetName is the sheet the local workbook keeps its data on.
	LocalSheetName = "Month End"
)

// LocalHeader is row 1 of a 本地 MPS sheet, columns A..Q.
var LocalHeader = []string{
	"P5", "P4", "P3", "P2", "P1", "MFG GROUP", "MFG CLASS CODE", "MFG DESCR",
	"ITEM", "Base Code", "SAP Code", "US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC",
}

// LocalSheet is a 本地 MPS workbook read into the shape the rest of the pipeline
// uses: the A..O index columns and the week values, exactly as a global workbook
// produces them, so the merge, the database and the grid need no special case.
//
// Cell styling is deliberately not read here yet: the local workbook is used for
// its values, and the sheets written on export are styled separately.
type LocalSheet struct {
	// Sheet is the worksheet the data was found on.
	Sheet string `json:"sheet"`
	// Header carries the week columns (from R) and the A..O index names.
	Header Header `json:"header"`
	// Rows are the kept rows, normalised to the MPS layout: Base Code and SAP Code
	// dropped, Weeks aligned to Header.Weeks.
	Rows []*DataRow `json:"-"`
	// Dict holds the styles interned while reading; the caller maps them to the
	// database ids when the Local dataset is written.
	Dict *StyleDict `json:"-"`
	// RowsTotal is how many data rows the sheet holds; RowsKept how many survived
	// the LOC filter.
	RowsTotal int `json:"rowsTotal"`
	RowsKept  int `json:"rowsKept"`
}

// ReadLocalFile opens a 本地 MPS workbook and checks its layout before reading rows.
//
// A sheet that does not look right is refused with a reason naming the cell: read
// with the wrong layout its columns would land in the wrong place, and nothing
// later would be able to tell.
func ReadLocalFile(path string, opt ReadOptions) (*LocalSheet, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, fmt.Errorf("打开本地 MPS 文件失败: %w", err)
	}
	defer f.Close()

	sheet, err := localSheetName(f)
	if err != nil {
		return nil, err
	}
	rows, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("读取工作表 %q 失败: %w", sheet, err)
	}
	// The header is the contract, so it is checked before anything else: a file that
	// is not the local layout should say so about row 1, not about how many rows it
	// happens to have.
	if len(rows) == 0 {
		return nil, fmt.Errorf("工作表 %q 是空的", sheet)
	}
	if err := checkLocalHeader(sheet, rows[0]); err != nil {
		return nil, err
	}

	headerRow := rows[0]
	out := &LocalSheet{Sheet: sheet}
	dict := opt.Dict
	if dict == nil {
		dict = NewStyleDict()
	}
	out.Dict = dict
	styleCache := map[int]int{}
	// Comments are read once and indexed by worksheet row/column; the MPS layout
	// renumbers columns, so they are translated when each row is built.
	comments := map[int]map[int]Comment{}
	if list, err := f.GetComments(sheet); err == nil {
		for _, c := range list {
			col, row, err := excelize.CellNameToCoordinates(c.Cell)
			if err != nil {
				continue
			}
			if comments[row] == nil {
				comments[row] = map[int]Comment{}
			}
			comments[row][col] = Comment{Text: c.Text, Author: c.Author}
		}
	}
	// Week columns in the order they appear, so Rows.Weeks lines up with the header
	// even if a blank cell leaves a gap.
	var weekCols []int
	for i := LocalFirstWeekCol - 1; i < len(headerRow); i++ {
		code := strings.TrimSpace(headerRow[i])
		if code == "" {
			continue
		}
		w, err := ParseWeekCode(code)
		if err != nil {
			name, _ := ColumnName(i + 1)
			return nil, fmt.Errorf("第 1 行 %s 列的周码 %q 无法识别: %w", name, code, err)
		}
		out.Header.Weeks = append(out.Header.Weeks, w)
		weekCols = append(weekCols, i)
	}
	if len(out.Header.Weeks) == 0 {
		name, _ := ColumnName(LocalFirstWeekCol)
		return nil, fmt.Errorf("第 1 行 %s 列起没有找到任何周码", name)
	}
	for i, name := range LocalHeader {
		if i == 9 || i == 10 { // Base Code, SAP Code: not part of the MPS layout
			continue
		}
		out.Header.IndexNames = append(out.Header.IndexNames, name)
	}

	for r := LocalDataRow - 1; r < len(rows); r++ {
		row := rows[r]
		if localCell(row, LocalItemCol) == "" {
			continue
		}
		out.RowsTotal++
		if want := strings.TrimSpace(opt.LOCFilter); want != "" &&
			!strings.EqualFold(localCell(row, LocalLOCCol), want) {
			continue
		}
		dr := &DataRow{SourceRow: r + 1}
		dr.StyleIDs = make([]int, IndexCols+len(weekCols))
		for c := 1; c <= 9; c++ {
			dr.Index[c-1] = localCell(row, c)
			dr.StyleIDs[c-1] = localStyleID(f, sheet, dict, styleCache, c, r+1)
		}
		for c := 12; c <= 17; c++ { // L..Q on a 本地 sheet are J..O of the MPS layout
			dr.Index[9+(c-12)] = localCell(row, c)
			dr.StyleIDs[9+(c-12)] = localStyleID(f, sheet, dict, styleCache, c, r+1)
		}
		for i, col := range weekCols {
			dr.Weeks = append(dr.Weeks, localCell(row, col+1))
			dr.StyleIDs[IndexCols+i] = localStyleID(f, sheet, dict, styleCache, col+1, r+1)
		}
		for localCol, c := range comments[r+1] {
			if mpsCol := localToMPSColumn(localCol); mpsCol > 0 {
				if dr.Comments == nil {
					dr.Comments = CommentMap{}
				}
				dr.Comments[mpsCol] = c
			}
		}
		out.Rows = append(out.Rows, dr)
		out.RowsKept++
	}
	if out.RowsKept == 0 {
		if want := strings.TrimSpace(opt.LOCFilter); want != "" {
			return nil, fmt.Errorf("工作表 %q 有 %d 行数据，但没有一行 LOC=%s", sheet, out.RowsTotal, want)
		}
		return nil, fmt.Errorf("工作表 %q 没有数据行（第 %d 行起）", sheet, LocalDataRow)
	}
	return out, nil
}

// localStyleID interns the style of one Local cell. Styles repeat across rows, so
// the excelize style id is cached before doing the JSON round trip.
func localStyleID(f *excelize.File, sheet string, dict *StyleDict, cache map[int]int, col, row int) int {
	sid, err := f.GetCellStyle(sheet, CellRef(col, row))
	if err != nil || sid == 0 {
		return 0
	}
	if id, ok := cache[sid]; ok {
		return id
	}
	st, err := f.GetStyle(sid)
	if err != nil || st == nil {
		return 0
	}
	id, err := dict.InternStyle(st)
	if err != nil {
		return 0
	}
	cache[sid] = id
	return id
}

// localToMPSColumn maps a 本地 workbook column onto the merged A..O + week
// layout. Base Code and SAP Code (J/K) have no MPS equivalent.
func localToMPSColumn(col int) int {
	switch {
	case col >= 1 && col <= 9:
		return col
	case col >= 12 && col <= 17:
		// L..Q on a 本地 sheet are J..O (10..15) of the MPS layout.
		return 10 + (col - 12)
	case col >= LocalFirstWeekCol:
		return IndexCols + (col - LocalFirstWeekCol) + 1
	}
	return 0
}

// localSheetName finds the sheet holding the local data: the usual name when it is
// there, otherwise any sheet whose first row looks like the layout.
func localSheetName(f *excelize.File) (string, error) {
	sheets := f.GetSheetList()
	for _, name := range sheets {
		if strings.EqualFold(strings.TrimSpace(name), LocalSheetName) {
			return name, nil
		}
	}
	for _, name := range sheets {
		rows, err := f.GetRows(name)
		if err != nil || len(rows) == 0 {
			continue
		}
		if checkLocalHeader(name, rows[0]) == nil {
			return name, nil
		}
	}
	return "", fmt.Errorf("找不到本地 MPS 工作表 %q（文件里的工作表: %s）", LocalSheetName, strings.Join(sheets, "、"))
}

// checkLocalHeader compares row 1 with the layout the reader expects.
func checkLocalHeader(sheet string, row []string) error {
	for i, want := range LocalHeader {
		got := localCell(row, i+1)
		if !strings.EqualFold(got, want) {
			name, _ := ColumnName(i + 1)
			return fmt.Errorf("工作表 %q 第 1 行 %s 列是 %q，期望 %q：这不是本地 MPS 的版式",
				sheet, name, got, want)
		}
	}
	return nil
}

// localCell returns a 1-based column's text, empty when the row is short.
func localCell(row []string, col int) string {
	if col < 1 || col > len(row) {
		return ""
	}
	return strings.TrimSpace(row[col-1])
}
