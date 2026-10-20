package mps

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/xuri/excelize/v2"
)

// BundleSheet is one sheet of the three-table export: the merged MPS data, the
// local rows and the global rows, each with the columns it actually has.
type BundleSheet struct {
	// Name is the sheet name: MPS, Local or Global.
	Name string
	// Weeks are the week columns, in order.
	Weeks []Week
	// IndexNames are the A..O headers.
	IndexNames []string
	// Rows are the data rows, already ordered.
	Rows []ExportRow
	// StyleByID resolves the persisted style ids carried by each row. The plain
	// bundle dump used to drop them along with the notes; both are part of the
	// source pages and have to travel with the three tables.
	StyleByID func(id int) (json.RawMessage, bool)
}

// BundleInput is a whole three-table workbook: one sheet per dataset.
type BundleInput struct {
	// DestPath is where the workbook is written; an existing file is replaced,
	// which is what the save dialog's overwrite confirmation is about.
	DestPath string
	// Sheets are written in order. The business names them MPS, Local, Global.
	Sheets []BundleSheet
}

// BundleSheetNames are the sheet names and order of the three-table file, as the
// business file has them.
var BundleSheetNames = []string{"MPS", "Local", "Global"}

// ExportBundle writes the three-table workbook: each sheet carries its dataset's
// header row (A..O plus its week codes), the week start dates beneath it, a
// spacer row, then the data.
//
// This is deliberately a plain dump of the three datasets rather than a re-use of
// the template export: the point of the file is to hand on the merged result
// together with both inputs, and each of them has its own columns. Cell styling,
// conditional formats and notes are not reproduced here; 导出原文件格式 still
// writes the single-sheet report from the stored template.
func ExportBundle(in BundleInput) (*ExportStats, error) {
	if len(in.Sheets) == 0 {
		return nil, fmt.Errorf("没有可写入的数据表")
	}
	if in.DestPath == "" {
		return nil, fmt.Errorf("没有指定导出文件")
	}

	f := excelize.NewFile()
	defer f.Close()

	// The workbook Excelize starts from has one sheet; the first dataset takes it
	// over and the rest are appended, so the order is exactly the one given.
	if err := f.SetSheetName(f.GetSheetName(0), in.Sheets[0].Name); err != nil {
		return nil, fmt.Errorf("建立工作表 %q 失败: %w", in.Sheets[0].Name, err)
	}

	headerStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 8},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EB1700"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return nil, err
	}
	dateFmt := "[$-409]d-mmm;@"
	dateStyle, err := f.NewStyle(&excelize.Style{
		Font:         &excelize.Font{Bold: true, Color: "FFFFFF", Size: 8},
		Fill:         excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EB1700"}},
		Alignment:    &excelize.Alignment{Horizontal: "center", Vertical: "center"},
		CustomNumFmt: &dateFmt,
	})
	if err != nil {
		return nil, err
	}
	spacerStyle, err := f.NewStyle(&excelize.Style{
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"D5CFC9"}},
	})
	if err != nil {
		return nil, err
	}
	dataStyle, err := f.NewStyle(&excelize.Style{
		Font: &excelize.Font{Size: 8},
	})
	if err != nil {
		return nil, err
	}

	stats := &ExportStats{Mode: "bundle", Sheets: len(in.Sheets), PreservedVBA: false}
	for i, sheet := range in.Sheets {
		if i > 0 {
			if _, err := f.NewSheet(sheet.Name); err != nil {
				return nil, fmt.Errorf("建立工作表 %q 失败: %w", sheet.Name, err)
			}
		}
		if err := writeBundleSheet(f, sheet, headerStyle, dateStyle, spacerStyle, dataStyle); err != nil {
			return nil, err
		}
		stats.Rows += len(sheet.Rows)
		if cols := IndexCols + len(sheet.Weeks); cols > stats.Cols {
			stats.Cols = cols
		}
	}

	if err := f.SaveAs(in.DestPath); err != nil {
		return nil, fmt.Errorf("写入 %s 失败: %w", in.DestPath, err)
	}
	if info, err := os.Stat(in.DestPath); err == nil {
		stats.SizeBytes = info.Size()
	}
	return stats, nil
}

// writeBundleSheet fills one sheet: header, dates, spacer, then the rows.
func writeBundleSheet(f *excelize.File, sheet BundleSheet, headerStyle, dateStyle, spacerStyle, dataStyle int) error {
	totalCols := IndexCols + len(sheet.Weeks)
	lastCol, err := ColumnName(totalCols)
	if err != nil {
		return err
	}

	for i, name := range sheet.IndexNames {
		ref := CellRef(i+1, 1)
		if err := f.SetCellStr(sheet.Name, ref, name); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet.Name, ref, ref, headerStyle); err != nil {
			return err
		}
	}
	for i, w := range sheet.Weeks {
		col := FirstWeekCol + i
		ref := CellRef(col, 1)
		// The week code is a number in the business file; a non-numeric code is
		// kept as text rather than dropped.
		if n, err := strconv.Atoi(w.Code); err == nil {
			if err := f.SetCellInt(sheet.Name, ref, int64(n)); err != nil {
				return err
			}
		} else if err := f.SetCellStr(sheet.Name, ref, w.Code); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet.Name, ref, ref, headerStyle); err != nil {
			return err
		}
		startRef := CellRef(col, 2)
		if err := f.SetCellValue(sheet.Name, startRef, w.Start); err != nil {
			return err
		}
		if err := f.SetCellStyle(sheet.Name, startRef, startRef, dateStyle); err != nil {
			return err
		}
	}

	// The second row is part of the title band: the date cells carry the dates, and
	// the A..O cells beside them keep the same colour instead of a white gap.
	if totalCols > 0 {
		if err := f.SetCellStyle(sheet.Name, CellRef(1, 2), CellRef(totalCols, 2), dateStyle); err != nil {
			return err
		}
	}

	// The spacer row is what the business file has between the dates and the
	// data; it is painted but carries no values.
	if totalCols > 0 {
		if err := f.SetCellStyle(sheet.Name, CellRef(1, 3), CellRef(totalCols, 3), spacerStyle); err != nil {
			return err
		}
	}

	resolver := newStyleResolver(f)
	styleCache := map[bundleStyleKey]int{}
	cfCache := map[string]int{}
	for r, row := range sheet.Rows {
		excelRow := FirstDataRowOfBundle + r
		for c := 0; c < IndexCols; c++ {
			v := row.Index[c]
			if v == "" {
				continue
			}
			if err := f.SetCellStr(sheet.Name, CellRef(c+1, excelRow), v); err != nil {
				return err
			}
		}
		for i := range sheet.Weeks {
			if i >= len(row.Weeks) || row.Weeks[i] == nil {
				continue
			}
			ref := CellRef(FirstWeekCol+i, excelRow)
			if err := f.SetCellValue(sheet.Name, ref, row.Weeks[i]); err != nil {
				return err
			}
		}
		if err := f.SetCellStyle(sheet.Name, CellRef(1, excelRow), CellRef(totalCols, excelRow), dataStyle); err != nil {
			return err
		}

		// Restore the source cell styles (fills, fonts, borders, number formats)
		// in coalesced runs, the same way the single-sheet export does.
		if sheet.StyleByID != nil {
			keepTimeFill := bundleRowKeepsWeekFill(row)
			ids := make([]int, totalCols)
			for c := 0; c < totalCols; c++ {
				ids[c] = dataStyle
				if c >= len(row.StyleIDs) {
					continue
				}
				dbID := row.StyleIDs[c]
				if dbID == 0 {
					continue
				}
				// Only the week cells of a CalcOH/CalcOH2 row keep their source
				// fill; every other data cell falls back to no fill.
				allowFill := keepTimeFill && c+1 >= FirstWeekCol
				key := bundleStyleKey{id: dbID, fill: allowFill}
				if sid, ok := styleCache[key]; ok {
					ids[c] = sid
					continue
				}
				sig, ok := sheet.StyleByID(dbID)
				if !ok {
					continue
				}
				if !allowFill {
					sig = bundleWithoutFill(sig)
				}
				sid, _ := resolver.resolve(sig)
				styleCache[key] = sid
				ids[c] = sid
			}
			for _, run := range coalesceRuns(ids) {
				if err := f.SetCellStyle(sheet.Name, CellRef(run.from, excelRow), CellRef(run.to, excelRow), run.styleID); err != nil {
					return err
				}
			}
		}

		// The bundle has no live conditional-format rules. Write the colour they
		// would paint, on top of the static style, so the week cells keep the
		// signal colours of the original pages.
		if !bundleRowKeepsWeekFill(row) {
			continue
		}
		for i, color := range row.CFColors {
			if i >= len(sheet.Weeks) {
				break
			}
			fill := bundleARGB(color)
			if fill == "" {
				continue
			}
			var st excelize.Style
			if sheet.StyleByID != nil {
				if c := IndexCols + i; c < len(row.StyleIDs) && row.StyleIDs[c] != 0 {
					if sig, ok := sheet.StyleByID(row.StyleIDs[c]); ok {
						_ = json.Unmarshal(sig, &st)
					}
				}
			}
			if st.Font == nil {
				st.Font = &excelize.Font{Size: 8}
			}
			st.Fill = excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{fill}}
			payload, err := json.Marshal(st)
			if err != nil {
				continue
			}
			key := string(payload)
			sid, ok := cfCache[key]
			if !ok {
				sid, _ = resolver.resolve(payload)
				cfCache[key] = sid
			}
			ref := CellRef(FirstWeekCol+i, excelRow)
			if err := f.SetCellStyle(sheet.Name, ref, ref, sid); err != nil {
				return err
			}
		}

		// Notes travel with the row exactly as they did in the source.
		for col, c := range row.Comments {
			text := strings.TrimSpace(c.Text)
			if col < 1 || col > totalCols || text == "" {
				continue
			}
			author := c.Author
			if author == "" {
				author = "CLEAR"
			}
			if err := f.AddComment(sheet.Name, excelize.Comment{
				Cell: CellRef(col, excelRow), Author: author, Text: text,
			}); err != nil {
				return err
			}
		}
	}

	// The used range has to cover the data. Excel recalculates it on open, but
	// dimension-respecting readers (pandas / openpyxl read_only among them) trust
	// it as written and would report an empty sheet — the same defect the
	// single-sheet export guards against.
	lastRow := FirstDataRowOfBundle + len(sheet.Rows) - 1
	if lastRow < 2 {
		lastRow = 2
	}
	if err := f.SetSheetDimension(sheet.Name, fmt.Sprintf("A1:%s%d", lastCol, lastRow)); err != nil {
		return err
	}
	if err := f.SetColWidth(sheet.Name, "A", lastCol, 13); err != nil {
		return err
	}
	if len(sheet.Rows) > 0 {
		// As in the business file the filter starts on the spacer row, which is
		// what makes "filter by week" reach the data without a title row in the
		// way.
		if err := f.AutoFilter(sheet.Name, fmt.Sprintf("A3:%s%d", lastCol, FirstDataRowOfBundle+len(sheet.Rows)-1), nil); err != nil {
			return err
		}
	}
	return nil
}

type bundleStyleKey struct {
	id   int
	fill bool
}

// bundleRowKeepsWeekFill is the business rule for the three-table file: only the
// P-onward week cells of a row whose O column is CalcOH or CalcOH2 keep colour.
func bundleRowKeepsWeekFill(row ExportRow) bool {
	if len(row.Index) < IndexCols {
		return false
	}
	value := strings.TrimSpace(row.Index[IndexCols-1])
	return strings.EqualFold(value, "CalcOH") || strings.EqualFold(value, "CalcOH2")
}

// bundleWithoutFill keeps fonts, borders and number formats but drops the fill.
func bundleWithoutFill(sig json.RawMessage) json.RawMessage {
	var st excelize.Style
	if err := json.Unmarshal(sig, &st); err != nil {
		return sig
	}
	st.Fill = excelize.Fill{}
	b, err := json.Marshal(st)
	if err != nil {
		return sig
	}
	return b
}

// bundleARGB turns the grid's #RRGGBB conditional-format colour into the
// ARGB string Excelize's fill expects.
func bundleARGB(color string) string {
	v := strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(color, "#")))
	switch len(v) {
	case 6:
		return v
	case 8:
		// Excelize's fill colours are RGB here; drop the alpha prefix rather than
		// handing it an ARGB value it reads back as black.
		return v[2:]
	}
	return ""
}

// FirstDataRowOfBundle is where the data starts in a three-table sheet: the
// header row, the week dates and the spacer row come first.
const FirstDataRowOfBundle = 4
