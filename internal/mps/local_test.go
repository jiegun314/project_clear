package mps

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/xuri/excelize/v2"
)

// writeLocalWorkbook builds a workbook in the 本地 layout: the 17 headers on row 1
// (with Base Code and SAP Code at J/K), week codes from R, data from row 4.
func writeLocalWorkbook(t *testing.T, sheet string, header []string, rows [][]string) string {
	t.Helper()
	f := excelize.NewFile()
	defer f.Close()
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		t.Fatalf("rename sheet: %v", err)
	}
	for i, h := range header {
		name, _ := ColumnName(i + 1)
		if err := f.SetCellStr(sheet, name+"1", h); err != nil {
			t.Fatal(err)
		}
	}
	for r, row := range rows {
		for c, v := range row {
			if v == "" {
				continue
			}
			name, _ := ColumnName(c + 1)
			if err := f.SetCellStr(sheet, name+strconv.Itoa(LocalDataRow+r), v); err != nil {
				t.Fatal(err)
			}
		}
	}
	path := filepath.Join(t.TempDir(), "local.xlsx")
	if err := f.SaveAs(path); err != nil {
		t.Fatalf("save: %v", err)
	}
	return path
}

// localHeaderWithWeeks is the expected row 1 plus two week columns at R and S.
func localHeaderWithWeeks() []string {
	h := append([]string{}, LocalHeader...)
	return append(h, "2639", "2640")
}

// dataRow lays out one data row for the 本地 sheet: A..Q then the week values.
func dataRow(index [15]string, base, sap, w1, w2 string) []string {
	row := make([]string, 19)
	for i := 0; i < 9; i++ {
		row[i] = index[i]
	}
	row[9] = base
	row[10] = sap
	for i := 9; i < 15; i++ {
		row[11+i-9] = index[i]
	}
	row[17] = w1
	row[18] = w2
	return row
}

func TestReadLocalFileKeepsTheMPSLayout(t *testing.T) {
	item1 := [15]string{"P5_EP_BW", "P4_A", "P3_A", "P2_A", "P1_A", "BUY", "965", "ACME", "ITEM-1",
		"US-1", "EU-1", "WH_CNB", "0", "6803", "AdjDmd"}
	item2 := [15]string{"P5_EP_BW", "P4_B", "P3_B", "P2_B", "P1_B", "BUY", "810", "ACME", "ITEM-2",
		"US-2", "EU-2", "OTHER_LOC", "0", "1", "AdjDmd"}
	path := writeLocalWorkbook(t, LocalSheetName, localHeaderWithWeeks(), [][]string{
		dataRow(item1, "BASE-1", "SAP-1", "-5", "7"),
		dataRow(item2, "BASE-2", "SAP-2", "99", "99"),                                // filtered out by LOC
		{"", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", "", ""}, // blank ITEM
	})

	sh, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err != nil {
		t.Fatalf("ReadLocalFile: %v", err)
	}
	if len(sh.Header.Weeks) != 2 || sh.Header.Weeks[0].Code != "2639" || sh.Header.Weeks[1].Code != "2640" {
		t.Fatalf("weeks = %+v, want 2639 and 2640", sh.Header.Weeks)
	}
	// Base Code and SAP Code are not part of the MPS layout.
	if len(sh.Header.IndexNames) != IndexCols {
		t.Fatalf("index names = %d, want %d", len(sh.Header.IndexNames), IndexCols)
	}
	if sh.RowsTotal != 2 || sh.RowsKept != 1 {
		t.Fatalf("rows total/kept = %d/%d, want 2/1 (one LOC filtered, one blank)", sh.RowsTotal, sh.RowsKept)
	}
	got := sh.Rows[0]
	want := []string{"P5_EP_BW", "P4_A", "P3_A", "P2_A", "P1_A", "BUY", "965", "ACME", "ITEM-1",
		"US-1", "EU-1", "WH_CNB", "0", "6803", "AdjDmd"}
	if strings.Join(got.Index[:], "|") != strings.Join(want, "|") {
		t.Fatalf("index = %v, want %v", got.Index, want)
	}
	if strings.Join(got.Weeks, "|") != "-5|7" {
		t.Fatalf("weeks = %v, want [-5 7]", got.Weeks)
	}
	if got.SourceRow != LocalDataRow {
		t.Errorf("SourceRow = %d, want %d", got.SourceRow, LocalDataRow)
	}
}

func TestReadLocalFileRefusesAWrongHeader(t *testing.T) {
	header := localHeaderWithWeeks()
	header[10] = "SAP CODE" // right column, wrong spelling is fine...
	header[9] = "BASE"      // ...but a shifted layout is not
	path := writeLocalWorkbook(t, LocalSheetName, header, [][]string{})
	_, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err == nil {
		t.Fatal("a sheet without the expected row 1 must be refused")
	}
	if !strings.Contains(err.Error(), "J 列") || !strings.Contains(err.Error(), "Base Code") {
		t.Errorf("error should name the column and the expected header, got: %v", err)
	}
}

func TestReadLocalFileRefusesAnUnreadableWeekCode(t *testing.T) {
	header := append([]string{}, LocalHeader...)
	header = append(header, "2639", "WK40")
	path := writeLocalWorkbook(t, LocalSheetName, header, [][]string{})
	_, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err == nil || !strings.Contains(err.Error(), "WK40") {
		t.Fatalf("a week code that does not parse must be refused with the cell named, got: %v", err)
	}
}

func TestReadLocalFileRefusesWhenNoRowMatchesTheFilter(t *testing.T) {
	item := [15]string{"P5", "P4", "P3", "P2", "P1", "BUY", "1", "ACME", "ITEM-1",
		"US", "EU", "SOME_OTHER", "0", "1", "AdjDmd"}
	path := writeLocalWorkbook(t, LocalSheetName, localHeaderWithWeeks(), [][]string{
		dataRow(item, "B", "S", "1", "2"),
	})
	_, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err == nil || !strings.Contains(err.Error(), "WH_CNB") {
		t.Fatalf("no LOC match must be refused, got: %v", err)
	}
}

// The sheet name may differ as long as row 1 is the layout: the name is a hint, the
// header is the contract.
func TestReadLocalFileFindsTheDataOnARenamedSheet(t *testing.T) {
	item := [15]string{"P5", "P4", "P3", "P2", "P1", "BUY", "1", "ACME", "ITEM-1",
		"US", "EU", "WH_CNB", "0", "1", "AdjDmd"}
	path := writeLocalWorkbook(t, "本地数据", localHeaderWithWeeks(), [][]string{
		dataRow(item, "B", "S", "1", "2"),
	})
	sh, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err != nil {
		t.Fatalf("ReadLocalFile: %v", err)
	}
	if sh.Sheet != "本地数据" || sh.RowsKept != 1 {
		t.Fatalf("sheet = %q, kept = %d; want the renamed sheet with one row", sh.Sheet, sh.RowsKept)
	}
}

// A workbook with none of the expected sheets says which sheets it does have, so the
// user can see at once that the wrong file was picked.
func TestReadLocalFileNamesTheSheetsItFound(t *testing.T) {
	path := filepath.Join(t.TempDir(), "other.xlsx")
	f := excelize.NewFile()
	if err := f.SetCellStr("Sheet1", "A1", "not a local file"); err != nil {
		t.Fatal(err)
	}
	if err := f.SaveAs(path); err != nil {
		t.Fatal(err)
	}
	f.Close()
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}

	_, err := ReadLocalFile(path, ReadOptions{LOCFilter: "WH_CNB"})
	if err == nil {
		t.Fatal("a workbook without the local layout must be refused")
	}
	if !strings.Contains(err.Error(), "Sheet1") {
		t.Errorf("error should list the sheets it did find, got: %v", err)
	}
}
