package mps

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

// ExportInput is everything the exporter needs to write a workbook.
type ExportInput struct {
	// TemplatePath is the stored copy of the first good source workbook.
	TemplatePath string
	// DestPath is where the merged workbook is written.
	DestPath string
	// Weeks are the week columns, in workbook order.
	Weeks []Week
	// IndexNames are the A..O headers.
	IndexNames []string
	// Rows are the merged rows, already ordered.
	Rows []ExportRow
	// StyleByID resolves a persisted style id to its signature.
	StyleByID func(id int) (json.RawMessage, bool)
	// FirstDataRowInTemplate is where the template's own data starts; the
	// template's original rows are reused so styles carry over exactly.
	Clean bool
}

// ExportRow is one row to write.
type ExportRow struct {
	// Index holds columns A..O.
	Index [IndexCols]string
	// Weeks holds the week cell values: float64, string or nil.
	Weeks []any
	// StyleIDs holds one style id per column, A..O then the week columns.
	StyleIDs []int
	// CF is the conditional-format program to re-anchor onto this row.
	CF CFRow
	// Comments maps a column number to its comment text.
	Comments map[int]string
	// SourceRow is the worksheet row the row came from, used to decide
	// whether a rule still refers to a row that survived filtering.
	SourceRow int
	// FileName identifies the source workbook, for logging.
	FileName string
}

// ExportStats summarises what an export produced.
type ExportStats struct {
	Rows           int    `json:"rows"`
	Cols           int    `json:"cols"`
	Mode           string `json:"mode"`
	Comments       int    `json:"comments"`
	CFRows         int    `json:"cfRows"`
	StylesUsed     int    `json:"stylesUsed"`
	CFRulesDropped int    `json:"cfRulesDropped"`
	BlankedRows    int    `json:"blankedRows"`
	PreservedVBA   bool   `json:"preservedVba"`
	DurationMS     int64  `json:"durationMs"`
	SizeBytes      int64  `json:"sizeBytes"`
}

// styleResolver maps a stored style signature onto a style index in the target
// workbook, reusing an identical existing style whenever it can so the
// template's own formatting is preserved bit for bit.
type styleResolver struct {
	f       *excelize.File
	bySig   map[string]int
	created int
}

func newStyleResolver(f *excelize.File) *styleResolver {
	r := &styleResolver{f: f, bySig: map[string]int{}}
	// Index whatever the target already has.
	for i := 0; i < 4096; i++ {
		st, err := f.GetStyle(i)
		if err != nil || st == nil {
			break
		}
		b, err := json.Marshal(st)
		if err != nil {
			continue
		}
		key := string(b)
		if _, ok := r.bySig[key]; !ok {
			r.bySig[key] = i
		}
	}
	return r
}

func (r *styleResolver) resolve(sig json.RawMessage) (int, bool) {
	key := string(sig)
	if id, ok := r.bySig[key]; ok {
		return id, false
	}
	var st excelize.Style
	if err := json.Unmarshal(sig, &st); err != nil {
		return 0, false
	}
	id, err := r.f.NewStyle(&st)
	if err != nil {
		return 0, false
	}
	r.bySig[key] = id
	r.created++
	return id, true
}

// Export writes the merged data, reproducing the original workbook's styling.
//
// Two engines are available. The default rewrites a stored copy of the source
// workbook, which keeps every visual and structural detail including the VBA
// payload. The clean engine rebuilds a fresh workbook that contains only the
// MPS sheet.
func Export(in ExportInput) (*ExportStats, error) {
	if len(in.Weeks) == 0 {
		return nil, fmt.Errorf("没有可导出的周数据列")
	}
	totalCols := IndexCols + len(in.Weeks)
	stats := &ExportStats{Rows: len(in.Rows), Cols: totalCols, Mode: "template"}
	if in.Clean {
		stats.Mode = "clean"
	}
	if err := os.MkdirAll(filepath.Dir(in.DestPath), 0o755); err != nil {
		return nil, err
	}

	if in.Clean {
		if err := exportClean(in, totalCols, stats); err != nil {
			return nil, err
		}
	} else {
		if err := exportTemplate(in, totalCols, stats); err != nil {
			return nil, err
		}
	}
	if fi, err := os.Stat(in.DestPath); err == nil {
		stats.SizeBytes = fi.Size()
	}
	return stats, nil
}

// exportTemplate rewrites a copy of the source workbook in place.
func exportTemplate(in ExportInput, totalCols int, stats *ExportStats) error {
	if in.TemplatePath == "" {
		return fmt.Errorf("缺少导出模板文件")
	}
	if err := copyFile(in.TemplatePath, in.DestPath); err != nil {
		return fmt.Errorf("复制模板文件失败: %w", err)
	}
	stats.PreservedVBA = HasPart(in.DestPath, "xl/vbaProject.bin")

	f, err := excelize.OpenFile(in.DestPath)
	if err != nil {
		return fmt.Errorf("打开模板失败: %w", err)
	}
	defer f.Close()

	// The template's own data rows are overwritten rather than deleted:
	// excelize's RemoveRow re-adjusts the whole sheet on every call, which is
	// quadratic and would also renumber the conditional formats.
	lastTemplateRow, err := lastDataRow(f)
	if err != nil {
		return err
	}

	res := newStyleResolver(f)
	stats.CFRulesDropped = writeRows(f, in, res, totalCols)
	stats.StylesUsed = len(res.bySig)

	// Blank whatever the template had below the merged block so no stale rows
	// survive underneath the new data.
	newLast := FirstDataRow + len(in.Rows) - 1
	if lastTemplateRow > newLast {
		for r := newLast + 1; r <= lastTemplateRow; r++ {
			for c := 1; c <= totalCols; c++ {
				if err := f.SetCellStr(SheetName, CellRef(c, r), ""); err != nil {
					return err
				}
			}
		}
		stats.BlankedRows = lastTemplateRow - newLast
	}
	// Any comment the template carried in the data area refers to cells that no
	// longer hold the same thing.
	clearDataComments(f)
	stats.Comments = addComments(f, in)

	if err := f.SaveAs(in.DestPath); err != nil {
		return fmt.Errorf("保存导出文件失败: %w", err)
	}

	cfRows := buildCFRows(in)
	stats.CFRows = len(cfRows)
	return InjectConditionalFormats(in.DestPath, SheetName, cfRows, totalCols)
}

// exportClean rebuilds a workbook that contains only the MPS sheet.
func exportClean(in ExportInput, totalCols int, stats *ExportStats) error {
	src, err := excelize.OpenFile(in.TemplatePath)
	if err != nil {
		return fmt.Errorf("打开模板失败: %w", err)
	}
	defer src.Close()

	f := excelize.NewFile()
	defer f.Close()
	// NewFile starts with Sheet1; MPS becomes the only sheet.
	const defaultSheet = "Sheet1"
	for _, name := range f.GetSheetList() {
		if name != defaultSheet {
			continue
		}
		if err := f.DeleteSheet(name); err != nil {
			return err
		}
	}
	if _, err := f.NewSheet(SheetName); err != nil {
		return err
	}
	f.SetActiveSheet(0)

	if err := copyHeaderBlock(src, f, totalCols); err != nil {
		return fmt.Errorf("复制表头区域失败: %w", err)
	}

	res := newStyleResolver(f)
	stats.CFRulesDropped = writeRows(f, in, res, totalCols)
	stats.StylesUsed = len(res.bySig)
	stats.Comments = addComments(f, in)

	if err := f.SaveAs(in.DestPath); err != nil {
		return fmt.Errorf("保存导出文件失败: %w", err)
	}

	// The new workbook has no differential formats, so the template's dxfs are
	// adopted wholesale; the rule dxfIds then stay valid.
	if styles, err := zipPart(in.DestPath, "xl/styles.xml"); err == nil {
		if srcStyles, err := zipPart(in.TemplatePath, "xl/styles.xml"); err == nil {
			if merged, _, ok, err := CopyDxfsInto(styles, srcStyles); err == nil && ok {
				if err := ReplaceParts(in.DestPath, map[string][]byte{"xl/styles.xml": merged}); err != nil {
					return err
				}
			}
		}
	}

	cfRows := buildCFRows(in)
	stats.CFRows = len(cfRows)
	// Keep the header-area rules too, so the report title keeps its colours.
	if header := headerCFRows(in.TemplatePath); len(header) > 0 {
		for k, v := range header {
			if _, taken := cfRows[k]; !taken {
				cfRows[k] = v
			}
		}
	}
	return InjectConditionalFormats(in.DestPath, SheetName, cfRows, totalCols)
}

// writeRows fills the data block and returns each source row's new position,
// which the caller needs to re-anchor conditional formats.
func writeRows(f *excelize.File, in ExportInput, res *styleResolver, totalCols int) int {
	// styleCache avoids re-resolving the same interned id row after row.
	styleCache := map[int]int{}
	var problems int

	for i, row := range in.Rows {
		newRow := FirstDataRow + i

		for c := 1; c <= IndexCols; c++ {
			if v := row.Index[c-1]; v != "" {
				if err := f.SetCellStr(SheetName, CellRef(c, newRow), v); err != nil {
					problems++
				}
			}
		}
		for w := range in.Weeks {
			if w >= len(row.Weeks) {
				break
			}
			col := FirstWeekCol + w
			ref := CellRef(col, newRow)
			switch v := row.Weeks[w].(type) {
			case nil:
				// leave the cell as the template left it, then blank it below
			case float64:
				if err := f.SetCellFloat(SheetName, ref, v, -1, 64); err != nil {
					problems++
				}
			case string:
				if v != "" {
					if err := f.SetCellStr(SheetName, ref, v); err != nil {
						problems++
					}
				}
			}
		}

		// Styles: coalesce equal neighbouring ids so a uniform row costs one
		// call per run instead of one per cell.
		ids := make([]int, totalCols)
		resolved := true
		for c := 0; c < totalCols && c < len(row.StyleIDs); c++ {
			dbID := row.StyleIDs[c]
			sid, ok := styleCache[dbID]
			if !ok {
				sig, found := in.StyleByID(dbID)
				if !found {
					resolved = false
					break
				}
				sid, _ = res.resolve(sig)
				styleCache[dbID] = sid
			}
			ids[c] = sid
		}
		if !resolved {
			// A style the database does not know about must not silently strip
			// the row's formatting; count it so the caller can report it.
			problems++
			continue
		}
		for _, run := range coalesceRuns(ids) {
			h := CellRef(run.from, newRow)
			v := CellRef(run.to, newRow)
			if err := f.SetCellStyle(SheetName, h, v, run.styleID); err != nil {
				problems++
			}
		}
	}
	return problems
}

type styleRun struct{ from, to, styleID int }

func coalesceRuns(ids []int) []styleRun {
	var out []styleRun
	for i := 0; i < len(ids); {
		j := i
		for j+1 < len(ids) && ids[j+1] == ids[i] {
			j++
		}
		out = append(out, styleRun{from: i + 1, to: j + 1, styleID: ids[i]})
		i = j + 1
	}
	return out
}

// buildCFRows re-anchors each row's format program onto its new position,
// dropping rules that referenced a row which is no longer adjacent.
func buildCFRows(in ExportInput) map[int]CFRow {
	// Merged index -> the worksheet row it came from, which is what decides
	// whether a neighbouring reference still points where it used to.
	srcOf := make([]int, len(in.Rows))
	for i, row := range in.Rows {
		srcOf[i] = row.SourceRow
	}
	out := make(map[int]CFRow, len(in.Rows))
	for i, row := range in.Rows {
		if row.CF.Empty() {
			continue
		}
		newRow := FirstDataRow + i
		filtered := dropUnreachableRules(row.CF, srcOf, i)
		if filtered.Empty() {
			continue
		}
		out[newRow] = filtered
	}
	return out
}

// dropUnreachableRules removes a rule when one of its row references no longer
// points at the same neighbour. Those rules compare an element row with the row
// below it, so after filtering they could otherwise paint a row using another
// item's number.
func dropUnreachableRules(in CFRow, srcOf []int, index int) CFRow {
	anchor := srcOf[index]
	// A reference is portable when the row it pointed at is still exactly
	// d rows further down in the merged output. Anything else would compare
	// this row against a different item, so the rule is discarded.
	reachable := func(d int) bool {
		j := index + d
		return j >= 0 && j < len(srcOf) && srcOf[j] == anchor+d
	}
	out := CFRow{}
	for _, blk := range in.Blocks {
		nb := CFBlock{C1: blk.C1, C2: blk.C2}
		for _, rule := range blk.Rules {
			portable := true
			for _, d := range rule.Offsets {
				if !reachable(d) {
					portable = false
					break
				}
			}
			if portable {
				nb.Rules = append(nb.Rules, rule)
			}
		}
		if len(nb.Rules) > 0 {
			out.Blocks = append(out.Blocks, nb)
		}
	}
	return out
}

func addComments(f *excelize.File, in ExportInput) int {
	n := 0
	for i, row := range in.Rows {
		if len(row.Comments) == 0 {
			continue
		}
		newRow := FirstDataRow + i
		for col, text := range row.Comments {
			if col < 1 || col > IndexCols+len(in.Weeks) || text == "" {
				continue
			}
			if err := f.AddComment(SheetName, excelize.Comment{
				Cell:   CellRef(col, newRow),
				Author: "CLEAR",
				Text:   text,
			}); err == nil {
				n++
			}
		}
	}
	return n
}

// clearDataComments removes template comments that pointed into the data block,
// which would otherwise annotate the wrong cells after the rewrite.
func clearDataComments(f *excelize.File) {
	list, err := f.GetComments(SheetName)
	if err != nil {
		return
	}
	for _, c := range list {
		_, row, err := cellToCoords(c.Cell)
		if err != nil || row < FirstDataRow {
			continue
		}
		_ = f.DeleteComment(SheetName, c.Cell)
	}
}

func lastDataRow(f *excelize.File) (int, error) {
	rows, err := f.GetRows(SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return 0, err
	}
	last := 0
	for i, r := range rows {
		if i+1 < FirstDataRow {
			continue
		}
		for _, v := range r {
			if strings.TrimSpace(v) != "" {
				last = i + 1
				break
			}
		}
	}
	return last, nil
}

// copyHeaderBlock reproduces rows 1..57 of the template into a clean workbook:
// the decorative title block, both header rows, the styled blank row, merged
// cells and column widths.
func copyHeaderBlock(src, dst *excelize.File, totalCols int) error {
	res := newStyleResolver(dst)
	raw, err := src.GetRows(SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return err
	}
	if len(raw) > BlankRow {
		raw = raw[:BlankRow]
	}
	for i := 0; i < len(raw); i++ {
		rowNum := i + 1
		for c := 1; c <= totalCols && c <= len(raw[i]); c++ {
			if v := raw[i][c-1]; v != "" {
				if err := dst.SetCellStr(SheetName, CellRef(c, rowNum), v); err != nil {
					return err
				}
			}
		}
	}
	// Styles for the header block.
	for rowNum := 1; rowNum <= BlankRow; rowNum++ {
		var ids []int
		for c := 1; c <= totalCols; c++ {
			ref := CellRef(c, rowNum)
			sid, err := src.GetCellStyle(SheetName, ref)
			if err != nil {
				ids = append(ids, 0)
				continue
			}
			st, err := src.GetStyle(sid)
			if err != nil {
				ids = append(ids, 0)
				continue
			}
			nb, err := json.Marshal(st)
			if err != nil {
				ids = append(ids, 0)
				continue
			}
			id, _ := res.resolve(nb)
			ids = append(ids, id)
		}
		for _, run := range coalesceRuns(ids) {
			h := CellRef(run.from, rowNum)
			v := CellRef(run.to, rowNum)
			if run.styleID != 0 {
				if err := dst.SetCellStyle(SheetName, h, v, run.styleID); err != nil {
					return err
				}
			}
		}
	}
	// Column widths.
	for c := 1; c <= totalCols; c++ {
		name, err := ColumnName(c)
		if err != nil {
			continue
		}
		w, err := src.GetColWidth(SheetName, name)
		if err != nil || w == 0 {
			continue
		}
		if err := dst.SetColWidth(SheetName, name, name, w); err != nil {
			return err
		}
	}
	// Merged cells in the header block.
	merges, err := src.GetMergeCells(SheetName)
	if err == nil {
		for _, m := range merges {
			start, end := m.GetStartAxis(), m.GetEndAxis()
			_, r1, err := cellToCoords(start)
			if err != nil {
				continue
			}
			_, r2, err := cellToCoords(end)
			if err != nil {
				continue
			}
			// Only header merges: a merge inside the data block would fight the
			// merged rows that are about to be written.
			if r1 < 1 || r2 > BlankRow {
				continue
			}
			_ = dst.MergeCell(SheetName, start, end)
		}
	}
	return nil
}

// headerCFRows returns the conditional formats that belong to the report header
// so a clean export keeps them.
func headerCFRows(templatePath string) map[int]CFRow {
	part, err := SheetPartPath(templatePath, SheetName)
	if err != nil {
		return nil
	}
	raw, err := zipPart(templatePath, part)
	if err != nil {
		return nil
	}
	// Read the blocks directly: ParseCFRows only reports data rows, and here
	// the rules anchored in the report header are the ones being kept.
	out := map[int]CFRow{}
	for _, blk := range extractElements(string(raw), cfOpenTag, cfCloseTag) {
		sqref, err := attrValue(blk.attrs, "sqref")
		if err != nil {
			continue
		}
		rawRules, err := parseRawRules(blk.inner)
		if err != nil {
			continue
		}
		for _, part := range strings.Fields(sqref) {
			c1, r1, c2, r2, err := parseRangeRef(part)
			if err != nil || r1 < 1 || r2 >= FirstDataRow {
				continue
			}
			for r := r1; r <= r2; r++ {
				rules, err := rebaseRules(rawRules, r)
				if err != nil {
					continue
				}
				row := out[r]
				row.Blocks = append(row.Blocks, CFBlock{C1: c1, C2: c2, Rules: rules})
				out[r] = row
			}
		}
	}
	return out
}

func copyFile(src, dst string) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, b, 0o644)
}
