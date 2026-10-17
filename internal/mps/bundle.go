package mps

import (
	"fmt"
	"os"
	"strconv"

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

	// The spacer row is what the business file has between the dates and the
	// data; it is painted but carries no values.
	if totalCols > 0 {
		if err := f.SetCellStyle(sheet.Name, CellRef(1, 3), CellRef(totalCols, 3), spacerStyle); err != nil {
			return err
		}
	}

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

// FirstDataRowOfBundle is where the data starts in a three-table sheet: the
// header row, the week dates and the spacer row come first.
const FirstDataRowOfBundle = 4
