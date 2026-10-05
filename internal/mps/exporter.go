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
	// DxfStyles holds the batch's differential formats; DxfIndex maps the id a
	// stored rule carries onto an index into that slice.
	DxfStyles []string
	DxfIndex  func(id int) (int, bool)
	// FirstDataRowInTemplate is where the template's own data starts; the
	// template's original rows are reused so styles carry over exactly.
	Clean bool
}

// dxfIndex resolves a stored differential-format id to an entry in DxfStyles.
// Without an explicit mapping the ids are positions, which is what the tests
// and a single-file export produce.
func (in ExportInput) dxfIndex(id int) (int, bool) {
	if in.DxfIndex != nil {
		return in.DxfIndex(id)
	}
	if id < 0 || id >= len(in.DxfStyles) {
		return 0, false
	}
	return id, true
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
	Comments CommentMap
	// SourceRow is the worksheet row the row came from, used to decide
	// whether a rule still refers to a row that survived filtering.
	SourceRow int
	// FileName identifies the source workbook, for logging.
	FileName string
}

// ExportStats summarises what an export produced.
type ExportStats struct {
	Rows       int    `json:"rows"`
	Cols       int    `json:"cols"`
	Mode       string `json:"mode"`
	Comments   int    `json:"comments"`
	CFRows     int    `json:"cfRows"`
	StylesUsed int    `json:"stylesUsed"`
	// CFRulesDropped is the historical name for the count of cells that could not
	// be written or whose style could not be restored. It is reported to the UI
	// under that name, so the field keeps it.
	CFRulesDropped int   `json:"cfRulesDropped"`
	PreservedVBA   bool  `json:"preservedVba"`
	DurationMS     int64 `json:"durationMs"`
	SizeBytes      int64 `json:"sizeBytes"`
	// Warnings lists the detail that could not be reproduced in the output.
	Warnings []string `json:"warnings,omitempty"`
}

// warn records a detail that could not be reproduced.
func (e *ExportStats) warn(format string, args ...any) {
	e.Warnings = append(e.Warnings, fmt.Sprintf(format, args...))
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

	// Rebuild the sheet before excelize opens the copy: the decorative rows go
	// away and the header takes their place, so the data block starts at
	// ExportFirstDataRow.
	if err := RebaseSheetPart(in.DestPath, totalCols); err != nil {
		return err
	}

	f, err := excelize.OpenFile(in.DestPath)
	if err != nil {
		return fmt.Errorf("打开模板失败: %w", err)
	}
	defer f.Close()

	// The original format is kept, but only its MPS page: the other sheets
	// (Datadump, PSI, Summary, the helper sheets behind them) describe how the
	// report was produced and are not what gets handed on.
	if err := dropOtherSheets(f); err != nil {
		return err
	}
	// Rows 1..54 are the report's decoration, not data. The sheet part was
	// rebased before this file was opened (see rebase.go), so what is left is
	// the header on rows 1..3 and nothing below it. There is therefore nothing
	// to blank underneath the merged block: the rebase already removed every
	// row the template had below the header. (A blanking pass used to sit here;
	// it could never run, because it measured the sheet after that rebase — so
	// it is gone rather than left to look like a safeguard.)

	res := newStyleResolver(f)
	stats.CFRulesDropped = writeRows(f, in, res, totalCols)
	if stats.CFRulesDropped > 0 {
		stats.warn("有 %d 个单元格写入失败或样式无法还原", stats.CFRulesDropped)
	}
	stats.StylesUsed = len(res.bySig)

	// Annotations are re-anchored rather than copied: whatever the template
	// carried in the data area belonged to its own rows, which the merged data
	// has just replaced.
	clearDataComments(f)
	stats.Comments = addComments(f, in)

	if err := f.SaveAs(in.DestPath); err != nil {
		return fmt.Errorf("保存导出文件失败: %w", err)
	}

	cfRows := buildCFRows(in)
	cfRows, err = prepareConditionalFormats(in, in.DestPath, cfRows, headerCFRows(in.TemplatePath))
	if err != nil {
		return err
	}
	stats.CFRows = len(cfRows)
	if err := StripThreadedComments(in.DestPath, SheetName, ExportFirstDataRow); err != nil {
		return err
	}
	return InjectConditionalFormats(in.DestPath, SheetName, cfRows, totalCols)
}

// prepareConditionalFormats gives every rule a home for its differential format
// in the exported workbook, then returns the rows to inject.
//
// A rule's dxfId was assigned by the workbook it came from, so it cannot be
// copied across as-is: the format tables differ per file, and an id past the
// end of the exported table makes Excel refuse the sheet. The batch's formats
// are appended to the target's (sharing an entry when the format is already
// there) and each rule is repointed at its new index.
func prepareConditionalFormats(in ExportInput, dest string, cfRows, headerRows map[int]CFRow) (map[int]CFRow, error) {
	extras := append([]string{}, in.DxfStyles...)
	// The header rules come straight from the template sheet, so they still
	// carry the template's own numbering; its table goes in after the batch's
	// and those ids are shifted past it.
	templateBase := len(extras)
	if len(headerRows) > 0 && in.TemplatePath != "" {
		if styles, err := zipPart(in.TemplatePath, "xl/styles.xml"); err == nil {
			extras = append(extras, ParseDxfs(styles)...)
		}
	}

	if len(extras) > 0 {
		styles, err := zipPart(dest, "xl/styles.xml")
		if err != nil {
			return nil, err
		}
		merged, at, err := MergeDxfs(styles, extras)
		if err != nil {
			return nil, err
		}
		if err := ReplaceParts(dest, map[string][]byte{"xl/styles.xml": merged}); err != nil {
			return nil, err
		}
		RemapDxfIDs(cfRows, func(id int) (int, bool) {
			k, ok := in.dxfIndex(id)
			if !ok || k >= len(at) {
				return 0, false
			}
			return at[k], true
		})
		RemapDxfIDs(headerRows, func(id int) (int, bool) {
			k := templateBase + id
			if id < 0 || k >= len(at) {
				return 0, false
			}
			return at[k], true
		})
	}

	// The report title keeps its own rules, unless a data row already claimed
	// that row number.
	for row, prog := range headerRows {
		if _, taken := cfRows[row]; !taken {
			cfRows[row] = prog
		}
	}
	return cfRows, nil
}

// dropOtherSheets removes every worksheet except MPS, leaving the rest of the
// package — styles, conditional formats, macros — as the source file had it.
func dropOtherSheets(f *excelize.File) error {
	for _, name := range f.GetSheetList() {
		if name == SheetName {
			continue
		}
		if err := f.DeleteSheet(name); err != nil {
			return fmt.Errorf("移除工作表 %q 失败: %w", name, err)
		}
	}
	for i, name := range f.GetSheetList() {
		if name == SheetName {
			f.SetActiveSheet(i)
			return nil
		}
	}
	return fmt.Errorf("模板中缺少 %q 工作表", SheetName)
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
	// NewFile starts with exactly one sheet. Rename it rather than deleting it
	// and adding another: a workbook must keep at least one sheet, so deleting
	// the last one leaves an empty "Sheet1" behind next to MPS.
	if err := f.SetSheetName(f.GetSheetList()[0], SheetName); err != nil {
		return fmt.Errorf("创建工作表失败: %w", err)
	}
	f.SetActiveSheet(0)

	if err := copyHeaderBlock(src, f, totalCols); err != nil {
		return fmt.Errorf("复制表头区域失败: %w", err)
	}

	res := newStyleResolver(f)
	stats.CFRulesDropped = writeRows(f, in, res, totalCols)
	if stats.CFRulesDropped > 0 {
		stats.warn("有 %d 个单元格写入失败或样式无法还原", stats.CFRulesDropped)
	}
	stats.StylesUsed = len(res.bySig)
	stats.Comments = addComments(f, in)

	if err := f.SaveAs(in.DestPath); err != nil {
		return fmt.Errorf("保存导出文件失败: %w", err)
	}

	cfRows := buildCFRows(in)
	// The title block keeps its own rules, so the report still looks like the
	// report; the data rows bring theirs from whichever file they came from.
	cfRows, err = prepareConditionalFormats(in, in.DestPath, cfRows, headerCFRows(in.TemplatePath))
	if err != nil {
		return err
	}
	stats.CFRows = len(cfRows)
	return InjectConditionalFormats(in.DestPath, SheetName, cfRows, totalCols)
}

// writeRows fills the data block and returns each source row's new position,
// which the caller needs to re-anchor conditional formats.
func writeRows(f *excelize.File, in ExportInput, res *styleResolver, totalCols int) int {
	// styleCache avoids re-resolving the same interned id row after row.
	styleCache := map[int]int{}
	var problems int

	for i, row := range in.Rows {
		newRow := ExportFirstDataRow + i

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
		newRow := ExportFirstDataRow + i
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
		newRow := ExportFirstDataRow + i
		for col, c := range row.Comments {
			text := strings.TrimSpace(c.Text)
			if col < 1 || col > IndexCols+len(in.Weeks) || text == "" {
				continue
			}
			author := c.Author
			if author == "" {
				author = "CLEAR"
			}
			if err := f.AddComment(SheetName, excelize.Comment{
				Cell:   CellRef(col, newRow),
				Author: author,
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
		if err != nil || row < ExportFirstDataRow {
			continue
		}
		_ = f.DeleteComment(SheetName, c.Cell)
	}
}

// copyHeaderBlock reproduces the template's header into a clean workbook: the
// index/week header row, the week start dates and the styled separator row,
// with their merges and column widths.
//
// Those live on rows 55..57 of the source and land on rows 1..3 here — the
// decorative block above them is not part of the data and is left out.
func copyHeaderBlock(src, dst *excelize.File, totalCols int) error {
	res := newStyleResolver(dst)
	raw, err := src.GetRows(SheetName, excelize.Options{RawCellValue: true})
	if err != nil {
		return err
	}
	if len(raw) > BlankRow {
		raw = raw[:BlankRow]
	}

	// Values: source row r -> exported row r-DropRows.
	for r := HeaderRow; r <= len(raw) && r <= BlankRow; r++ {
		outRow := r - DropRows
		source := raw[r-1]
		for c := 1; c <= totalCols && c <= len(source); c++ {
			if v := source[c-1]; v != "" {
				if err := dst.SetCellStr(SheetName, CellRef(c, outRow), v); err != nil {
					return err
				}
			}
		}
	}
	// Styles for the header block.
	for r := HeaderRow; r <= BlankRow; r++ {
		outRow := r - DropRows
		var ids []int
		for c := 1; c <= totalCols; c++ {
			ref := CellRef(c, r)
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
			h := CellRef(run.from, outRow)
			v := CellRef(run.to, outRow)
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
			// Only the header's own merges: a merge inside the data block would
			// fight the merged rows that are about to be written, and the
			// decorative merges above the header have no row to land on.
			if r1 < HeaderRow || r2 > BlankRow {
				continue
			}
			c1, _, err := cellToCoords(start)
			if err != nil {
				continue
			}
			c2, _, err := cellToCoords(end)
			if err != nil {
				continue
			}
			_ = dst.MergeCell(SheetName, CellRef(c1, r1-DropRows), CellRef(c2, r2-DropRows))
		}
	}
	return nil
}

// headerCFRows returns the template's header-area conditional formats, keyed by
// the row they land on in the export. Rules anchored in the decorative block
// above the header have no row to land on and are left out.
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
				if r < HeaderRow {
					continue
				}
				rules, err := rebaseRules(rawRules, r)
				if err != nil {
					continue
				}
				// rebaseRules keeps the formulas relative to the source row;
				// the block moves up with everything else.
				exportRow := r - DropRows
				row := out[exportRow]
				row.Blocks = append(row.Blocks, CFBlock{C1: c1, C2: c2, Rules: rules})
				out[exportRow] = row
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
