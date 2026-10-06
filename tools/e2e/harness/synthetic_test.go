package harness

import (
	"bytes"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"project_clear/internal/mps"

	"github.com/xuri/excelize/v2"
)

// The acceptance run is the only thing that proves an export is faithful, and
// until now it could only be run by hand over the real workbooks in raw_data —
// which are not in version control, so nothing guarded the export on a fresh
// checkout or in CI. This builds a small workbook with the same layout instead,
// and runs the whole acceptance path (import, integrate, export, audit) over it.
//
// It is deliberately kept to one file and a handful of rows: the point is that
// the contract can be checked anywhere, not that it replaces the run over the
// real data.

// excelSerial converts a date into the 1900-system serial the header stores.
func excelSerial(t *testing.T, day time.Time) int {
	t.Helper()
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return int(day.Sub(base).Hours() / 24)
}

// writeSyntheticSource creates one workbook with the MPS layout described in
// internal/mps/header.go: decoration above row 55, the index headers on row 55
// with the week codes from column P, the week start dates on row 56, the styled
// separator on row 57 and data from row 58.
func writeSyntheticSource(t *testing.T, dir string) string {
	t.Helper()
	f := excelize.NewFile()
	t.Cleanup(func() { _ = f.Close() })

	idx, err := f.NewSheet(mps.SheetName)
	if err != nil {
		t.Fatalf("new sheet: %v", err)
	}
	f.SetActiveSheet(idx)
	if err := f.DeleteSheet("Sheet1"); err != nil {
		t.Fatalf("delete default sheet: %v", err)
	}
	sheet := mps.SheetName

	// Rows 1..54 are the decorative block: a title, a logo row and spacing.
	if err := f.SetCellValue(sheet, "A1", "SYNTHETIC MPS WEEKLY REPORT"); err != nil {
		t.Fatal(err)
	}
	if err := f.SetCellValue(sheet, "A3", "Generated for testing only"); err != nil {
		t.Fatal(err)
	}

	// Row 55: the fixed A..O index headers, then the week codes.
	indexNames := []string{
		"P5", "P4", "P3", "P2", "P1", "MFG GROUP", "MFG CLASS CODE", "MFG DESCR",
		"ITEM", "US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC",
	}
	for i, name := range indexNames {
		col, _ := mps.ColumnName(i + 1)
		if err := f.SetCellValue(sheet, col+strconv.Itoa(mps.HeaderRow), name); err != nil {
			t.Fatal(err)
		}
	}
	codes := []string{"2639", "2640"}
	for i, code := range codes {
		col, _ := mps.ColumnName(mps.FirstWeekCol + i)
		if err := f.SetCellValue(sheet, col+strconv.Itoa(mps.HeaderRow), code); err != nil {
			t.Fatal(err)
		}
		w, err := mps.ParseWeekCode(code)
		if err != nil {
			t.Fatalf("week code %s: %v", code, err)
		}
		// Row 56 holds the Monday as a date serial; the reader cross-checks it
		// against the code, so it has to be exact.
		if err := f.SetCellValue(sheet, col+strconv.Itoa(mps.DateRow), excelSerial(t, w.Start)); err != nil {
			t.Fatal(err)
		}
	}

	// Row 57 is visually blank but carries the report body's styling, which the
	// audit checks survives the export.
	bodyStyle, err := f.NewStyle(&excelize.Style{
		Font:      &excelize.Font{Size: 9},
		Fill:      excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFF2F2F2"}},
		Alignment: &excelize.Alignment{Horizontal: "center"},
	})
	if err != nil {
		t.Fatalf("body style: %v", err)
	}
	lastCol, _ := mps.ColumnName(mps.FirstWeekCol + len(codes) - 1)
	if err := f.SetCellStyle(sheet, "A"+strconv.Itoa(mps.BlankRow),
		lastCol+strconv.Itoa(mps.BlankRow), bodyStyle); err != nil {
		t.Fatal(err)
	}

	// Row 56's serials are formatted as dates, the way the real report has them
	// (dd-mmm). A serial without a date format is just a number, so without this
	// the audit could not tell an exported date from an exported number.
	dateFmt := "dd-mmm"
	dateStyle, err := f.NewStyle(&excelize.Style{CustomNumFmt: &dateFmt})
	if err != nil {
		t.Fatalf("date style: %v", err)
	}
	startCol, _ := mps.ColumnName(mps.FirstWeekCol)
	if err := f.SetCellStyle(sheet, startCol+strconv.Itoa(mps.DateRow),
		lastCol+strconv.Itoa(mps.DateRow), dateStyle); err != nil {
		t.Fatal(err)
	}

	// Data from row 58. Row L is the LOC filter column.
	rows := []struct {
		index []string
		loc   string
		weeks []float64
	}{
		{[]string{"P5", "P4", "P3", "P2", "P1", "GROUP A", "CLASS 1", "DESCR A", "ITEM-0001", "US-1", "EU-1"}, "WH_CNB", []float64{120, 130}},
		{[]string{"P5", "P4", "P3", "P2", "P1", "GROUP A", "CLASS 1", "DESCR B", "ITEM-0002", "US-2", "EU-2"}, "WH_CNB", []float64{0, 45.5}},
		{[]string{"P5", "P4", "P3", "P2", "P1", "GROUP B", "CLASS 2", "DESCR C", "ITEM-0003", "US-3", "EU-3"}, "WH_CNB", []float64{7, 8}},
		// This one must be filtered out: its LOC is not the configured filter.
		{[]string{"P5", "P4", "P3", "P2", "P1", "GROUP B", "CLASS 2", "DESCR D", "ITEM-0004", "US-4", "EU-4"}, "OTHER_WH", []float64{999, 999}},
	}
	firstData := mps.FirstDataRow
	for i, row := range rows {
		r := firstData + i
		for c, v := range row.index {
			col, _ := mps.ColumnName(c + 1)
			if err := f.SetCellValue(sheet, col+strconv.Itoa(r), v); err != nil {
				t.Fatal(err)
			}
		}
		if err := f.SetCellValue(sheet, "L"+strconv.Itoa(r), row.loc); err != nil {
			t.Fatal(err)
		}
		// Column O carries the element type even though its header reads LOC.
		if err := f.SetCellValue(sheet, "O"+strconv.Itoa(r), "AdjDmd"); err != nil {
			t.Fatal(err)
		}
		for w, v := range row.weeks {
			col, _ := mps.ColumnName(mps.FirstWeekCol + w)
			if err := f.SetCellValue(sheet, col+strconv.Itoa(r), v); err != nil {
				t.Fatal(err)
			}
		}
	}

	// A conditional format over the week columns of the two kept rows, so the
	// rule rewriting and the dxf re-interning are exercised.
	dxf, err := f.NewConditionalStyle(&excelize.Style{
		Font: &excelize.Font{Color: "FF9C0006"},
		Fill: excelize.Fill{Type: "pattern", Pattern: 1, Color: []string{"FFFFC7CE"}},
	})
	if err != nil {
		t.Fatalf("conditional style: %v", err)
	}
	cfRange := "P" + strconv.Itoa(firstData) + ":" + lastCol + strconv.Itoa(firstData+len(rows)-1)
	if err := f.SetConditionalFormat(sheet, cfRange, []excelize.ConditionalFormatOptions{{
		Type:     "cell",
		Criteria: ">",
		Value:    "100",
		Format:   &dxf,
	}}); err != nil {
		t.Fatalf("conditional format: %v", err)
	}

	// A note on one kept cell, so the comment path is exercised too.
	if err := f.AddComment(sheet, excelize.Comment{
		Cell:   "P" + strconv.Itoa(firstData),
		Author: "tester",
		Text:   "synthetic note",
	}); err != nil {
		t.Fatalf("comment: %v", err)
	}

	path := filepath.Join(dir, "2026 WK39 Synthetic 100 WW_XX.xlsm")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save %s: %v", path, err)
	}
	return path
}

// TestAcceptanceRunOnASyntheticWorkbook runs the whole acceptance path — import,
// integrate, export, and every fidelity audit — over the small workbook above,
// in both export modes.
func TestAcceptanceRunOnASyntheticWorkbook(t *testing.T) {
	for _, mode := range []struct {
		name  string
		clean bool
	}{{"template", false}, {"clean", true}} {
		t.Run(mode.name, func(t *testing.T) {
			in := t.TempDir()
			writeSyntheticSource(t, in)

			var report bytes.Buffer
			err := Run(Options{
				In:     in,
				Out:    t.TempDir(),
				Clean:  mode.clean,
				Report: &report,
			})
			if err != nil {
				t.Fatalf("acceptance run failed: %v\n--- report ---\n%s", err, report.String())
			}
			if !bytes.Contains(report.Bytes(), []byte("全部检查通过")) {
				t.Errorf("the run did not report success:\n%s", report.String())
			}
		})
	}
}
