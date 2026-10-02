package mps

import (
	"fmt"
	"strconv"
	"strings"
)

// An export keeps the report's data and nothing else: the header block that the
// source keeps on rows 55..57 becomes rows 1..3, and the rows above it — the
// report title, the logo row, the spacers — are dropped.
//
// The template route cannot simply ask excelize to delete the rows: its
// RemoveRow rebuilds the sheet on every call and needs minutes on a report this
// size, and the template's own calculation columns (AJ onwards, tens of
// thousands of formulas) would have to be re-pointed too. Instead the sheet XML
// is rebuilt once, before excelize opens the copy: only the header rows
// survive, so there is nothing left to shift.

// RebaseSheetPart rewrites the MPS part of a workbook in place so the sheet
// starts at the header and carries only the columns the export writes.
func RebaseSheetPart(workbook string, totalCols int) error {
	part, err := SheetPartPath(workbook, SheetName)
	if err != nil {
		return err
	}
	raw, err := zipPart(workbook, part)
	if err != nil {
		return err
	}
	rebuilt, err := rebaseSheetXML(raw, totalCols)
	if err != nil {
		return err
	}
	return ReplaceParts(workbook, map[string][]byte{part: rebuilt})
}

// rebaseSheetXML keeps the header rows (renumbered to start at row 1) and drops
// everything else from sheetData, together with the conditional formats, which
// the export rebuilds for its own row numbers.
func rebaseSheetXML(doc []byte, totalCols int) ([]byte, error) {
	sheet := strings.Replace(string(StripAllCF(doc)), "<sheetData/>", "<sheetData></sheetData>", 1)

	var rows strings.Builder
	kept := 0
	for _, el := range extractElements(sheet, "<row", "</row>") {
		rowNum, err := attrIntFrom(el.attrs, "r")
		if err != nil || rowNum < HeaderRow || rowNum > BlankRow {
			continue
		}
		row, err := rebaseRow(el, rowNum-DropRows, totalCols)
		if err != nil {
			return nil, err
		}
		rows.WriteString(row)
		kept++
	}
	if kept == 0 {
		return nil, fmt.Errorf("模板的第 %d–%d 行不存在，无法重建表头", HeaderRow, BlankRow)
	}

	open := strings.Index(sheet, "<sheetData")
	if open < 0 {
		return nil, fmt.Errorf("模板缺少 sheetData")
	}
	_, _, bodyStart, selfClosing, err := openTagEnd(sheet, open)
	if err != nil {
		return nil, err
	}
	if selfClosing {
		return nil, fmt.Errorf("模板的 sheetData 为空")
	}
	closeIdx := strings.Index(sheet[bodyStart:], "</sheetData>")
	if closeIdx < 0 {
		return nil, fmt.Errorf("模板的 sheetData 未闭合")
	}
	closeIdx += bodyStart

	rebuilt := sheet[:bodyStart] + rows.String() + sheet[closeIdx:]
	rebuilt = rebaseDimension(rebuilt, totalCols, ExportBlankRow)
	rebuilt = rebaseMergeCells(rebuilt)
	return []byte(rebuilt), nil
}

// rebaseRow renumbers one row and its cells, keeps only the exported columns
// and drops formulas: the header is a static block of names and dates, and its
// formulas refer to rows that no longer exist. Their cached values are kept.
func rebaseRow(el xmlElement, newRow, totalCols int) (string, error) {
	name, attrs, _, selfClosing, err := openTagEnd(el.raw, 0)
	if err != nil {
		return "", err
	}
	_ = name
	if selfClosing {
		return "<row " + setAttr(attrs, "r", strconv.Itoa(newRow)) + "/>", nil
	}
	head := "<row " + setAttr(attrs, "r", strconv.Itoa(newRow)) + ">"

	var cells strings.Builder
	for _, cell := range extractElements(el.inner, "<c", "</c>") {
		ref, err := attrValue(cell.attrs, "r")
		if err != nil {
			continue
		}
		col, _, err := cellToCoords(ref)
		if err != nil || col > totalCols {
			continue
		}
		body := stripFormulas(cell.inner)
		if cell.raw != "" && strings.HasSuffix(cell.raw, "/>") {
			// A style-only cell: keep it so the header keeps its look.
			cells.WriteString("<c " + setAttr(cell.attrs, "r", CellRef(col, newRow)) + "/>")
			continue
		}
		cells.WriteString("<c " + setAttr(cell.attrs, "r", CellRef(col, newRow)) + ">" + body + "</c>")
	}
	return head + cells.String() + "</row>", nil
}

// stripFormulas removes the <f> elements, leaving the cached <v> behind.
func stripFormulas(inner string) string {
	out := inner
	for _, f := range extractElements(out, "<f", "</f>") {
		out = strings.Replace(out, f.raw, "", 1)
	}
	return out
}

// rebaseDimension points the used range at the rebuilt sheet.
func rebaseDimension(doc string, totalCols, lastRow int) string {
	last, err := ColumnName(totalCols)
	if err != nil {
		return doc
	}
	at := strings.Index(doc, "<dimension ")
	if at < 0 {
		return doc
	}
	_, _, end, selfClosing, err := openTagEnd(doc, at)
	if err != nil {
		return doc
	}
	ref := fmt.Sprintf("A1:%s%d", last, lastRow)
	if selfClosing {
		return doc[:at] + `<dimension ref="` + ref + `"/>` + doc[end:]
	}
	return doc[:at] + `<dimension ref="` + ref + `">` + doc[end:]
}

// rebaseMergeCells keeps the merges that live inside the header block and moves
// the rest out of the file: a merge anchored in the dropped rows would either
// dangle or, worse, cover cells the merged data now occupies.
func rebaseMergeCells(doc string) string {
	els := extractElements(doc, "<mergeCells", "</mergeCells>")
	if len(els) == 0 {
		return doc
	}
	block := els[0]
	var kept []string
	for _, m := range extractElements(block.inner, "<mergeCell", "</mergeCell>") {
		ref, err := attrValue(m.attrs, "ref")
		if err != nil {
			continue
		}
		c1, r1, c2, r2, err := parseRangeRef(ref)
		if err != nil || r1 < HeaderRow || r2 > BlankRow {
			continue
		}
		kept = append(kept, `<mergeCell ref="`+
			rangeRefOrEmpty(c1, r1-DropRows, c2, r2-DropRows)+`"/>`)
	}
	replacement := ""
	if len(kept) > 0 {
		replacement = fmt.Sprintf(`<mergeCells count="%d">%s</mergeCells>`, len(kept), strings.Join(kept, ""))
	}
	return strings.Replace(doc, block.raw, replacement, 1)
}

func rangeRefOrEmpty(c1, r1, c2, r2 int) string {
	ref, err := rangeRef(c1, r1, c2, r2)
	if err != nil {
		return ""
	}
	return ref
}

// attrIntFrom reads an integer attribute from a raw attribute string, which is
// what the hand-rolled element scanner hands back.
func attrIntFrom(attrs, name string) (int, error) {
	raw, err := attrValue(attrs, name)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(raw))
}

// setAttr replaces an attribute value, falling back to appending it.
func setAttr(attrs, name, value string) string {
	replaced := false
	out := replaceAttr(attrs, name, value, &replaced)
	if replaced {
		return out
	}
	return strings.TrimSpace(out + " " + name + `="` + value + `"`)
}
