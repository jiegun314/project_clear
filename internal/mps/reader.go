package mps

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// StyleDict interns cell styles by signature so a batch of workbooks that
// share a template costs one row per distinct look rather than one per cell.
type StyleDict struct {
	byHash map[string]int
	sigs   []json.RawMessage
}

// NewStyleDict creates an empty dictionary.
func NewStyleDict() *StyleDict {
	return &StyleDict{byHash: map[string]int{}}
}

// Intern resolves a style to its dictionary id, registering it on first sight.
func (d *StyleDict) Intern(sig json.RawMessage) int {
	h := sha256.Sum256(sig)
	key := hex.EncodeToString(h[:8])
	if id, ok := d.byHash[key]; ok {
		return id
	}
	id := len(d.sigs)
	d.sigs = append(d.sigs, sig)
	d.byHash[key] = id
	return id
}

// InternStyle serialises a resolved style and interns it.
func (d *StyleDict) InternStyle(s *excelize.Style) (int, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return 0, err
	}
	return d.Intern(b), nil
}

// Style returns the stored signature for an id.
func (d *StyleDict) Style(id int) (json.RawMessage, bool) {
	if id < 0 || id >= len(d.sigs) {
		return nil, false
	}
	return d.sigs[id], true
}

// Len is the number of distinct styles collected so far.
func (d *StyleDict) Len() int { return len(d.sigs) }

// DataRow is one kept worksheet row, normalised so the week code and its date
// travel together as one unit.
type DataRow struct {
	// Index holds columns A..O exactly as displayed.
	Index [IndexCols]string `json:"index"`
	// Weeks maps the week code to the raw cell text. Values are kept as text
	// so floats such as 2499.9999999999995 survive unchanged.
	Weeks []string `json:"weeks"`
	// StyleIDs holds one interned style per column, A..O followed by the week
	// columns, so styling can be rebuilt column by column on export.
	StyleIDs []int `json:"styleIds"`
	// CF is the conditional-format program captured for this row.
	CF CFRow `json:"cf"`
	// CFColors holds the fill colour the conditional formats paint on each
	// week cell, aligned to Weeks. The grid shows it instead of the static
	// fill, which is what Excel does; empty entries have no matching rule.
	CFColors []string `json:"cfColors,omitempty"`
	// Comments maps a column number to its comment text.
	Comments CommentMap `json:"comments,omitempty"`
	// SourceRow is the worksheet row the data came from.
	SourceRow int `json:"sourceRow"`
	// FileID identifies the source file within the batch.
	FileID int64 `json:"fileId"`
}

// FileResult is the outcome of reading one workbook.
type FileResult struct {
	Path      string     `json:"path"`
	Name      string     `json:"name"`
	Size      int64      `json:"size"`
	Header    Header     `json:"header"`
	Rows      []*DataRow `json:"-"`
	RowsTotal int        `json:"rowsTotal"`
	RowsKept  int        `json:"rowsKept"`
	Err       string     `json:"err,omitempty"`
}

// ReadOptions controls what a read keeps.
type ReadOptions struct {
	// LOCFilter is the value required in column L.
	LOCFilter string
	// ReadColumns is how many week columns to read from column P onwards.
	ReadColumns int
	// Dict interns cell styles; share one across a batch.
	Dict *StyleDict
	// Dxf interns conditional-format records; share one across a batch so ids
	// from different workbooks cannot collide.
	Dxf *DxfDict
}

// ReadFile extracts the header, the filtered rows and everything needed to
// reproduce their appearance.
//
// A failure here is never fatal to a batch: the returned FileResult carries the
// reason in Err so the caller can count the file as failed and carry on.
func ReadFile(path string, opt ReadOptions) (*FileResult, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	res := &FileResult{Path: path, Name: filepath.Base(path), Size: info.Size()}

	f, err := excelize.OpenFile(path)
	if err != nil {
		res.Err = fmt.Sprintf("打开失败: %v", err)
		return res, nil
	}
	defer f.Close()

	if !hasSheet(f, SheetName) {
		res.Err = fmt.Sprintf("缺少 %q 工作表", SheetName)
		return res, nil
	}

	header, err := readHeader(f, opt.ReadColumns)
	if err != nil {
		res.Err = err.Error()
		return res, nil
	}
	res.Header = header
	total := header.TotalCols()

	// Conditional formats come from the raw sheet XML: excelize's reader keys
	// by sqref and would drop three of every four rules in these workbooks.
	cfByRow, err := readSheetCFRows(path)
	if err != nil {
		// Missing conditional formats is a cosmetic loss, not a reason to
		// reject the data.
		cfByRow = map[int]CFRow{}
	}
	// Each workbook numbers its own <dxf> records. Keep the file-local table
	// (it is what the conditional formats are evaluated against) and re-intern
	// the ids for the batch after the rows have been read.
	var dxfColors map[int]string
	var dxfs []string
	if opt.Dxf != nil {
		if styles, err := zipPart(path, "xl/styles.xml"); err == nil {
			dxfs = ParseDxfs(styles)
			dxfColors = DxfFillColors(dxfs)
		}
	}

	commentsByRow, err := readComments(path, SheetName)
	if err != nil {
		commentsByRow = map[int]CommentMap{}
	}

	// Excel keeps a merged block's value in its top-left cell and leaves the
	// rest empty; the planner needs to see it on every row the merge covers
	// (MFG CLASS CODE and friends are merged across a whole item block).
	var merges []MergeRange
	if part, err := SheetPartPath(path, SheetName); err == nil {
		if raw, err := zipPart(path, part); err == nil {
			merges = ParseMergeRanges(raw)
		}
	}
	filler := newMergeFiller(merges)

	// Values come from a streaming pass over the sheet XML: excelize's
	// GetRows spends most of its time on number-format resolution that the
	// staging layer never needs.
	shared, err := sharedStrings(path)
	if err != nil {
		res.Err = fmt.Sprintf("读取字符串表失败: %v", err)
		return res, nil
	}
	vr, err := newValueReader(path, SheetName)
	if err != nil {
		res.Err = fmt.Sprintf("打开数据流失败: %v", err)
		return res, nil
	}
	defer vr.Close()

	styleIdxCache := map[int]int{}
	// Every row's cells, kept for the conditional-format evaluation below: the
	// rules compare a cell with the row two below it, which a forward-only pass
	// has not seen yet.
	allCells := map[int][]string{}
	err = vr.readRows(shared, FirstDataRow, 1, total, func(rowNum int, cells []string) error {
		cells = filler.expand(cells, rowNum, total)
		allCells[rowNum] = append([]string(nil), cells...)
		// Blank but styled rows still exist in these workbooks; they are not
		// data and must not inflate the reported totals.
		hasValue := false
		for _, v := range cells {
			if strings.TrimSpace(v) != "" {
				hasValue = true
				break
			}
		}
		if !hasValue {
			return nil
		}
		res.RowsTotal++
		if len(cells) < LOCCol || strings.TrimSpace(cells[LOCCol-1]) != opt.LOCFilter {
			return nil
		}

		row := &DataRow{
			Weeks:     make([]string, len(header.Weeks)),
			StyleIDs:  make([]int, total),
			SourceRow: rowNum,
			CF:        cfByRow[rowNum],
			Comments:  commentsByRow[rowNum],
		}
		for c := 0; c < IndexCols && c < len(cells); c++ {
			row.Index[c] = cells[c]
		}
		for w := range header.Weeks {
			// Week w lives in column P+w (1-based), so its 0-based cell index
			// is FirstWeekCol+w-1. Reading from one column earlier put the
			// element type of column O into the first week cell and shifted
			// every number one column to the left.
			col := FirstWeekCol + w
			if col-1 < len(cells) {
				row.Weeks[w] = cells[col-1]
			}
		}
		for c := 1; c <= total; c++ {
			ax := CellRef(c, rowNum)
			sid, err := f.GetCellStyle(SheetName, ax)
			if err != nil {
				continue
			}
			interned, ok := styleIdxCache[sid]
			if !ok {
				st, err := f.GetStyle(sid)
				if err != nil {
					continue
				}
				interned, err = opt.Dict.InternStyle(st)
				if err != nil {
					continue
				}
				styleIdxCache[sid] = interned
			}
			row.StyleIDs[c-1] = interned
		}
		res.Rows = append(res.Rows, row)
		res.RowsKept++
		return nil
	})
	if err != nil {
		res.Err = fmt.Sprintf("读取数据行失败: %v", err)
		return res, nil
	}
	applyCFColors(res.Rows, header, allCells, total, dxfColors, f)
	// The stored program must reference the batch-wide dxf ids, and the rows
	// carry their own copy of the program.
	if opt.Dxf != nil && len(dxfs) > 0 {
		remapSourceDxfs(cfByRow, dxfs, opt.Dxf)
		for _, row := range res.Rows {
			row.CF = cfByRow[row.SourceRow]
		}
	}
	return res, nil
}

// applyCFColors evaluates each kept row's conditional formats so the grid can
// show the colour Excel shows. The rules compare the cell with the row two
// below (the SS row) and with an absolute parameter cell such as $DN$56, so the
// lookup falls back to excelize for columns outside the read window.
func applyCFColors(rows []*DataRow, header Header, allCells map[int][]string, total int, dxfColors map[int]string, f *excelize.File) {
	if len(dxfColors) == 0 {
		return
	}
	outside := map[[2]int]string{}
	lookup := func(row, col int) string {
		if col >= 1 && col <= total {
			if cells, ok := allCells[row]; ok && col-1 < len(cells) {
				return cells[col-1]
			}
			return ""
		}
		key := [2]int{row, col}
		if v, ok := outside[key]; ok {
			return v
		}
		v, err := f.GetCellValue(SheetName, CellRef(col, row), excelize.Options{RawCellValue: true})
		if err != nil {
			v = ""
		}
		outside[key] = v
		return v
	}
	for _, row := range rows {
		if row.CF.Empty() {
			continue
		}
		colors := EvalCFColors(row.CF, row.SourceRow, dxfColors, lookup)
		if len(colors) == 0 {
			continue
		}
		week := make([]string, len(header.Weeks))
		for i := range header.Weeks {
			if c, ok := colors[FirstWeekCol+i]; ok {
				week[i] = c
			}
		}
		row.CFColors = week
	}
}

func hasSheet(f *excelize.File, name string) bool {
	for _, s := range f.GetSheetList() {
		if s == name {
			return true
		}
	}
	return false
}

func readHeader(f *excelize.File, readCols int) (Header, error) {
	h := Header{}
	for c := 1; c <= IndexCols; c++ {
		v, err := f.GetCellValue(SheetName, CellRef(c, HeaderRow), excelize.Options{RawCellValue: true})
		if err != nil {
			return h, fmt.Errorf("读取第 %d 行表头失败: %w", HeaderRow, err)
		}
		h.IndexNames = append(h.IndexNames, strings.TrimSpace(v))
	}
	// The LOC column is the one every row is filtered on; if it is not where
	// the specification says, refuse the file rather than silently reading
	// nonsense.
	if h.IndexNames[LOCCol-1] != "LOC" {
		return h, fmt.Errorf("第 %d 列表头为 %q，期望 \"LOC\"，无法确认筛选列",
			LOCCol, h.IndexNames[LOCCol-1])
	}

	for i := 0; i < readCols; i++ {
		col := FirstWeekCol + i
		codeCell := CellRef(col, HeaderRow)
		code, err := f.GetCellValue(SheetName, codeCell, excelize.Options{RawCellValue: true})
		if err != nil {
			return h, fmt.Errorf("读取周列表头 %s 失败: %w", codeCell, err)
		}
		code = strings.TrimSpace(code)
		if code == "" {
			break // the week block ended early
		}
		w, err := ParseWeekCode(code)
		if err != nil {
			return h, fmt.Errorf("周列表头 %s: %w", codeCell, err)
		}
		// Cross-check against row 56 when it holds a usable serial so a
		// mis-typed week code cannot slip through unnoticed.
		if dateCell := CellRef(col, DateRow); true {
			serial, err := f.GetCellValue(SheetName, dateCell, excelize.Options{RawCellValue: true})
			if err == nil && serial != "" {
				if sv, perr := strconv.ParseFloat(serial, 64); perr == nil {
					if d, derr := WeekFromSerial(sv); derr == nil {
						if !d.Equal(w.Start) {
							return h, fmt.Errorf("周码 %s 与起始日期 %s 不一致（第55/56行）",
								w.Code, d.Format("2006-01-02"))
						}
					}
				}
			}
		}
		h.Weeks = append(h.Weeks, w)
	}
	if len(h.Weeks) == 0 {
		return h, fmt.Errorf("从第 %d 列起未读取到任何周数据列", FirstWeekCol)
	}
	return h, nil
}

// ReadSheetCFRows exposes the conditional-format capture for testing.
func readSheetCFRows(path string) (map[int]CFRow, error) {
	part, err := SheetPartPath(path, SheetName)
	if err != nil {
		return nil, err
	}
	raw, err := zipPart(path, part)
	if err != nil {
		return nil, err
	}
	return ParseCFRows(raw)
}
