package mps

import (
	"fmt"
	"strconv"

	"github.com/xuri/excelize/v2"
)

// writeMonthEndSheet writes the calculated Month End page. Its layout follows
// the business sample: totals in rows 1/2, header in row 3, dates in row 4,
// spacer row 5, data from row 6, and A..R before the week columns.
func writeMonthEndSheet(f *excelize.File, sheet MonthEndSheet, headerStyle, dateStyle, spacerStyle, dataStyle int) error {
	totalCols := MonthEndIndexCols + len(sheet.Weeks)
	lastCol, err := ColumnName(totalCols)
	if err != nil {
		return err
	}
	labelStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Bold: true, Color: "FFFFFF", Size: 9},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"EB1700"}},
		Alignment: &excelize.Alignment{Horizontal: "center", Vertical: "center"},
	})
	if err != nil {
		return err
	}
	qtyStyle, err := f.NewStyle(&excelize.Style{
		Font:         &excelize.Font{Bold: true, Size: 9},
		CustomNumFmt: strPtr("#,##0"),
	})
	if err != nil {
		return err
	}
	valueStyle, err := f.NewStyle(&excelize.Style{
		Font:         &excelize.Font{Bold: true, Size: 9},
		CustomNumFmt: strPtr("#,##0.00"),
	})
	if err != nil {
		return err
	}

	// Total labels. The values themselves only exist under month-end columns.
	if err := f.SetCellStr("Month End", CellRef(MonthEndIndexCols, 1), "Total Month End INV"); err != nil {
		return err
	}
	if err := f.SetCellStr("Month End", CellRef(MonthEndIndexCols, 2), "Total Month End Value"); err != nil {
		return err
	}
	for _, c := range []int{MonthEndIndexCols} {
		for r := 1; r <= 2; r++ {
			if err := f.SetCellStyle("Month End", CellRef(c, r), CellRef(c, r), labelStyle); err != nil {
				return err
			}
		}
	}
	for _, j := range sheet.MonthEndCols {
		if j < 0 || j >= len(sheet.Weeks) {
			continue
		}
		col := MonthEndIndexCols + 1 + j
		if err := f.SetCellValue("Month End", CellRef(col, 1), sheet.TotalINV[j]); err != nil {
			return err
		}
		if err := f.SetCellStyle("Month End", CellRef(col, 1), CellRef(col, 1), qtyStyle); err != nil {
			return err
		}
		if err := f.SetCellValue("Month End", CellRef(col, 2), sheet.TotalValue[j]); err != nil {
			return err
		}
		if err := f.SetCellStyle("Month End", CellRef(col, 2), CellRef(col, 2), valueStyle); err != nil {
			return err
		}
	}

	// Header row 3: A..R plus the week codes.
	for i, name := range MonthEndHeaders {
		ref := CellRef(i+1, 3)
		if err := f.SetCellStr("Month End", ref, name); err != nil {
			return err
		}
		if err := f.SetCellStyle("Month End", ref, ref, headerStyle); err != nil {
			return err
		}
	}
	for i, w := range sheet.Weeks {
		col := MonthEndIndexCols + 1 + i
		ref := CellRef(col, 3)
		if n, err := strconv.Atoi(w.Code); err == nil {
			if err := f.SetCellInt("Month End", ref, int64(n)); err != nil {
				return err
			}
		} else if err := f.SetCellStr("Month End", ref, w.Code); err != nil {
			return err
		}
		if err := f.SetCellStyle("Month End", ref, ref, headerStyle); err != nil {
			return err
		}
	}

	// Date row 4.
	for i, w := range sheet.Weeks {
		col := MonthEndIndexCols + 1 + i
		ref := CellRef(col, 4)
		if err := f.SetCellValue("Month End", ref, w.Start); err != nil {
			return err
		}
		if err := f.SetCellStyle("Month End", ref, ref, dateStyle); err != nil {
			return err
		}
	}
	// Spacer row 5 only. Do not apply the date style across row 2: it would
	// overwrite the total cells' number format and Excel would read the values
	// as dates.
	if totalCols > 0 {
		if err := f.SetCellStyle("Month End", CellRef(1, 5), CellRef(totalCols, 5), spacerStyle); err != nil {
			return err
		}
	}

	for r, row := range sheet.Rows {
		excelRow := MonthEndFirstDataRow + r
		for c := 0; c < MonthEndIndexCols; c++ {
			if row.Index[c] == "" {
				continue
			}
			if err := f.SetCellStr("Month End", CellRef(c+1, excelRow), row.Index[c]); err != nil {
				return err
			}
		}
		for i := 0; i < len(sheet.Weeks); i++ {
			if i >= len(row.Weeks) || row.Weeks[i] == nil {
				continue
			}
			if err := f.SetCellValue("Month End", CellRef(MonthEndIndexCols+1+i, excelRow), row.Weeks[i]); err != nil {
				return err
			}
		}
		if err := f.SetCellStyle("Month End", CellRef(1, excelRow), CellRef(totalCols, excelRow), dataStyle); err != nil {
			return err
		}
	}

	lastRow := MonthEndFirstDataRow + len(sheet.Rows) - 1
	if lastRow < MonthEndFirstDataRow {
		lastRow = MonthEndFirstDataRow
	}
	if err := f.SetSheetDimension("Month End", fmt.Sprintf("A1:%s%d", lastCol, lastRow)); err != nil {
		return err
	}
	if err := f.SetColWidth("Month End", "A", lastCol, 13); err != nil {
		return err
	}
	if len(sheet.Rows) > 0 {
		if err := f.AutoFilter("Month End", fmt.Sprintf("A3:%s%d", lastCol, lastRow), nil); err != nil {
			return err
		}
	}
	return nil
}

func strPtr(s string) *string { return &s }
