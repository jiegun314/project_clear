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
		for c := 1; c <= 9; c++ {
			dr.Index[c-1] = localCell(row, c)
		}
		for c := 12; c <= 17; c++ { // L..Q on a 本地 sheet are J..O of the MPS layout
			dr.Index[9+(c-12)] = localCell(row, c)
		}
		for _, col := range weekCols {
			dr.Weeks = append(dr.Weeks, localCell(row, col+1))
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
