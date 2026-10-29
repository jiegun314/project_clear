package mps

import (
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"
)

// Local workbooks come in two layouts.
//
//   - Month End: the older layout with J = Base Code and K = SAP Code. Everything
//     after them shifts by two, so LOC sits in N rather than L and the week codes
//     start at R rather than P.
//   - Local: the newer layout, which is the A..O MPS layout itself; LOC is in L
//     and the week codes start at P.
//
// Both are normalised to the same A..O + week rows before storage, so the merge
// and everything downstream see one shape.
type localLayout int

const (
	localMonthEnd localLayout = iota
	localGlobalStyle
)

const (
	// LocalFirstWeekCol is where the week codes start on a Month End sheet (R).
	LocalFirstWeekCol = 18
	// LocalNewFirstWeekCol is where the week codes start on the newer Local
	// sheet (P).
	LocalNewFirstWeekCol = 16
	// LocalDataRow is the first data row in both layouts.
	LocalDataRow = 4
	// LocalItemCol is the ITEM column (I) in both layouts.
	LocalItemCol = 9
	// LocalLOCCol is the LOC column on a Month End sheet (N).
	LocalLOCCol = 14
	// LocalNewLOCCol is the LOC column on a newer Local sheet (L).
	LocalNewLOCCol = 12
	// LocalSheetName is the sheet the older local workbook keeps its data on.
	LocalSheetName = "Month End"
	// LocalNewSheetName is the sheet the newer local workbook uses.
	LocalNewSheetName = "Local"
)

// LocalHeader is row 1 of a 本地 MPS sheet, columns A..Q.
var LocalHeader = []string{
	"P5", "P4", "P3", "P2", "P1", "MFG GROUP", "MFG CLASS CODE", "MFG DESCR",
	"ITEM", "Base Code", "SAP Code", "US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC",
}

// localMPSHeader is the A..O header both layouts normalise to: the Month End
// header without Base Code and SAP Code.
var localMPSHeader = func() []string {
	out := append([]string{}, LocalHeader[:9]...)
	return append(out, LocalHeader[11:]...)
}()

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
	// Warnings are cosmetic losses while reading (currently conditional-format
	// colours); the data remains usable.
	Warnings []string `json:"warnings,omitempty"`
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

	sheet, layout, err := localSheet(f)
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
	headerRow := rows[0]
	firstWeek := LocalFirstWeekCol
	locCol := LocalLOCCol
	if layout == localGlobalStyle {
		firstWeek = LocalNewFirstWeekCol
		locCol = LocalNewLOCCol
	}
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
	// Conditional formats are the source of the signal colours on CalcOH rows.
	// Local workbooks use them just like Global ones; capture and evaluate them
	// here so the Local sheet keeps the same colours as its source file.
	cfByRow, cfErr := readSheetCFRowsFor(path, sheet)
	var dxfColors map[int]string
	if cfErr == nil {
		var dxfErr error
		dxfColors, dxfErr = readDxfColors(path)
		if dxfErr != nil {
			cfErr = dxfErr
		}
	}
	if cfErr != nil {
		out.Warnings = append(out.Warnings, fmt.Sprintf("条件格式读取失败（Local 颜色将缺失）: %v", cfErr))
	}
	cfLookup := func(row, col int) string {
		if row >= 1 && row <= len(rows) && col >= 1 && col <= len(rows[row-1]) {
			return rows[row-1][col-1]
		}
		v, _ := f.GetCellValue(sheet, CellRef(col, row), excelize.Options{RawCellValue: true})
		return v
	}

	// Week columns in the order they appear, so Rows.Weeks lines up with the header
	// even if a blank cell leaves a gap.
	var weekCols []int
	for i := firstWeek - 1; i < len(headerRow); i++ {
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
		name, _ := ColumnName(firstWeek)
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
		if !LOCMatches(localCell(row, locCol), opt.LOCFilter) {
			continue
		}
		dr := &DataRow{SourceRow: r + 1}
		dr.StyleIDs = make([]int, IndexCols+len(weekCols))
		if layout == localGlobalStyle {
			for c := 1; c <= IndexCols; c++ {
				dr.Index[c-1] = localCell(row, c)
				dr.StyleIDs[c-1] = localStyleID(f, sheet, dict, styleCache, c, r+1)
			}
		} else {
			for c := 1; c <= 9; c++ {
				dr.Index[c-1] = localCell(row, c)
				dr.StyleIDs[c-1] = localStyleID(f, sheet, dict, styleCache, c, r+1)
			}
			for c := 12; c <= 17; c++ { // L..Q on a 本地 sheet are J..O of the MPS layout
				dr.Index[9+(c-12)] = localCell(row, c)
				dr.StyleIDs[9+(c-12)] = localStyleID(f, sheet, dict, styleCache, c, r+1)
			}
		}
		for i, col := range weekCols {
			dr.Weeks = append(dr.Weeks, localCell(row, col+1))
			dr.StyleIDs[IndexCols+i] = localStyleID(f, sheet, dict, styleCache, col+1, r+1)
		}
		if len(dxfColors) > 0 {
			if prog, ok := cfByRow[r+1]; ok && !prog.Empty() {
				colors := EvalCFColors(prog, r+1, dxfColors, cfLookup)
				if len(colors) > 0 {
					dr.CFColors = make([]string, len(weekCols))
					for i, col := range weekCols {
						dr.CFColors[i] = colors[col+1]
					}
				}
			}
		}
		for localCol, c := range comments[r+1] {
			mpsCol := 0
			if layout == localGlobalStyle {
				mpsCol = localNewToMPSColumn(localCol)
			} else {
				mpsCol = localMonthEndToMPSColumn(localCol)
			}
			if mpsCol > 0 {
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

// localMonthEndToMPSColumn maps a Month End workbook column onto the merged
// A..O + week layout. Base Code and SAP Code (J/K) have no MPS equivalent.
func localMonthEndToMPSColumn(col int) int {
	switch {
	case col >= 1 && col <= 9:
		return col
	case col >= 12 && col <= 17:
		// L..Q on a Month End sheet are J..O (10..15) of the MPS layout.
		return 10 + (col - 12)
	case col >= LocalFirstWeekCol:
		return IndexCols + (col - LocalFirstWeekCol) + 1
	}
	return 0
}

// localNewToMPSColumn maps the newer Local sheet's A..O + week columns; they
// already are the MPS layout.
func localNewToMPSColumn(col int) int {
	switch {
	case col >= 1 && col <= IndexCols:
		return col
	case col >= LocalNewFirstWeekCol:
		return IndexCols + (col - LocalNewFirstWeekCol) + 1
	}
	return 0
}

// localSheet finds the sheet holding Local data and identifies its layout. The
// two known layouts have different headers, so detection is by row 1 rather than
// by trusting the sheet name.
func localSheet(f *excelize.File) (string, localLayout, error) {
	sheets := f.GetSheetList()
	order := []string{}
	for _, preferred := range []string{LocalSheetName, LocalNewSheetName} {
		for _, name := range sheets {
			if strings.EqualFold(strings.TrimSpace(name), preferred) {
				order = append(order, name)
			}
		}
	}
	for _, name := range sheets {
		found := false
		for _, existing := range order {
			if existing == name {
				found = true
				break
			}
		}
		if !found {
			order = append(order, name)
		}
	}
	for _, name := range order {
		cells := localHeaderRow(f, name)
		if matchHeader(cells, LocalHeader) {
			return name, localMonthEnd, nil
		}
		if matchHeader(cells, localMPSHeader) {
			return name, localGlobalStyle, nil
		}
	}
	return "", localMonthEnd, fmt.Errorf("找不到本地 MPS 工作表（支持 Month End A–Q 或 Local A–O 版式；文件里的工作表: %s）",
		strings.Join(sheets, "、"))
}

// localHeaderRow reads row 1 without loading the whole worksheet, which matters
// for the newer Local files with 14k+ rows.
func localHeaderRow(f *excelize.File, sheet string) []string {
	out := make([]string, 0, len(LocalHeader))
	for c := 1; c <= len(LocalHeader); c++ {
		v, _ := f.GetCellValue(sheet, CellRef(c, 1), excelize.Options{RawCellValue: true})
		out = append(out, v)
	}
	return out
}

func matchHeader(cells, want []string) bool {
	if len(cells) < len(want) {
		return false
	}
	for i, expected := range want {
		if !strings.EqualFold(strings.TrimSpace(cells[i]), expected) {
			return false
		}
	}
	return true
}

// localCell returns a 1-based column's text, empty when the row is short.
func localCell(row []string, col int) string {
	if col < 1 || col > len(row) {
		return ""
	}
	return strings.TrimSpace(row[col-1])
}
