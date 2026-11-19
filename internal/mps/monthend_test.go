package mps

import (
	"fmt"
	"math"
	"testing"
	"time"
)

func monthEndTestWeeks() []Week {
	return []Week{
		{Code: "2645", Start: MonthEndDate(2026, 11, 2)},
		{Code: "2646", Start: MonthEndDate(2026, 11, 9)},
		{Code: "2647", Start: MonthEndDate(2026, 11, 16)},
		{Code: "2648", Start: MonthEndDate(2026, 11, 23)},
		{Code: "2649", Start: MonthEndDate(2026, 11, 30)},
		{Code: "2650", Start: MonthEndDate(2026, 12, 7)},
		{Code: "2651", Start: MonthEndDate(2026, 12, 14)},
		{Code: "2652", Start: MonthEndDate(2026, 12, 21)},
		{Code: "2653", Start: MonthEndDate(2026, 12, 28)},
		{Code: "2701", Start: MonthEndDate(2027, 1, 4)},
	}
}

func monthEndTestRows(item string) []ExportRow {
	values := map[string][]any{}
	for i, f := range MonthEndFields {
		rowVals := make([]any, 10)
		for j := range rowVals {
			switch f {
			case "CalcOH":
				rowVals[j] = float64((j + 1) * 10)
			case "SchedRcpts":
				rowVals[j] = float64(j + 1)
			case "DeployIn":
				rowVals[j] = float64(j + 2)
			default:
				rowVals[j] = float64(i*100 + j)
			}
		}
		values[f] = rowVals
	}
	rows := make([]ExportRow, 0, len(MonthEndFields))
	for _, f := range MonthEndFields {
		var idx [IndexCols]string
		idx[0], idx[1], idx[2] = "P5", "P4", "P3"
		idx[3], idx[4], idx[5] = "P2", "P1", "MFG"
		idx[6], idx[7], idx[8] = "CLASS", "DESCR", item
		idx[9], idx[10] = "US", "EU"
		idx[11], idx[12], idx[13] = "WH_CNB", "BO", "OH"
		idx[14] = f
		rows = append(rows, ExportRow{Index: idx, Weeks: values[f]})
	}
	return rows
}

func TestLastMondayColumns(t *testing.T) {
	got := LastMondayColumns(monthEndTestWeeks())
	want := []int{4, 8}
	if len(got) != len(want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columns = %v, want %v", got, want)
		}
	}
}

func TestFiscalMonthEndColumns(t *testing.T) {
	got := FiscalMonthEndColumns(monthEndTestWeeks())
	want := []int{2, 8}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("columns = %v, want %v", got, want)
	}
}

func TestFiscalMonthEndColumnsMatchWeek46Sample(t *testing.T) {
	weeks := make([]Week, 0, 26)
	starts := []string{
		"2026-11-09", "2026-11-16", "2026-11-23", "2026-11-30", "2026-12-07",
		"2026-12-14", "2026-12-21", "2026-12-28", "2027-01-04", "2027-01-11",
		"2027-01-18", "2027-01-25", "2027-02-01", "2027-02-08", "2027-02-15",
		"2027-02-22", "2027-03-01", "2027-03-08", "2027-03-15", "2027-03-22",
		"2027-03-29", "2027-04-05", "2027-04-12", "2027-04-19", "2027-04-26",
		"2027-05-03",
	}
	for i, s := range starts {
		d, _ := time.Parse("2006-01-02", s)
		weeks = append(weeks, Week{Code: fmt.Sprintf("w%d", i), Start: d})
	}
	got := FiscalMonthEndColumns(weeks)
	want := []int{1, 7, 11, 15, 20, 24}
	if len(got) != len(want) {
		t.Fatalf("columns = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("columns = %v, want %v", got, want)
		}
	}
}

func TestBuildMonthEndAcceptsExistingMonthEndLabels(t *testing.T) {
	rows := monthEndTestRows("I1")
	for i := range rows {
		switch rows[i].Index[14] {
		case "AdjInbound":
			rows[i].Index[14] = "Month End INV"
		case "AdjOutbound":
			rows[i].Index[14] = "Month End INV (Value)"
		}
	}
	cost := 10.0
	sheet, err := BuildMonthEnd(MonthEndInput{
		Weeks:        monthEndTestWeeks(),
		Rows:         rows,
		PriceByItem:  map[string]MonthEndPrice{"I1": {BaseCode: "B1", SapCode: "S1", Category: "C1", StandardCost: &cost}},
		ExchangeRate: 7.1788,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sheet.Groups != 1 || sheet.SkippedGroups != 0 || len(sheet.Rows) != 14 {
		t.Fatalf("groups=%d skipped=%d rows=%d", sheet.Groups, sheet.SkippedGroups, len(sheet.Rows))
	}
}

func TestBuildMonthEndCalculatesOnlyMonthEndColumns(t *testing.T) {
	cost := 10.0
	sheet, err := BuildMonthEnd(MonthEndInput{
		Weeks:        monthEndTestWeeks(),
		Rows:         monthEndTestRows("I1"),
		PriceByItem:  map[string]MonthEndPrice{"I1": {BaseCode: "B1", SapCode: "S1", Category: "C1", StandardCost: &cost}},
		ExchangeRate: 7.1788,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sheet.Groups != 1 || sheet.MissingPrices != 0 || sheet.SkippedGroups != 0 {
		t.Fatalf("stats = %+v", sheet)
	}
	if len(sheet.Rows) != 14 {
		t.Fatalf("rows = %d", len(sheet.Rows))
	}
	var invRow, valRow *MonthEndRow
	for i := range sheet.Rows {
		switch sheet.Rows[i].Index[17] {
		case "Month End INV":
			invRow = &sheet.Rows[i]
		case "Month End Value":
			valRow = &sheet.Rows[i]
		}
	}
	if invRow == nil || valRow == nil {
		t.Fatal("Month End INV/Value rows missing")
	}
	// At 2647: 30 + 4+5+6 + 5+6+7 = 63; at 2653: 90 + 10 + 11 = 111.
	wantINV := map[int]float64{2: 63, 8: 111}
	for i, want := range wantINV {
		got, ok := invRow.Weeks[i].(float64)
		if !ok || math.Abs(got-want) > 1e-9 {
			t.Errorf("INV[%d] = %v, want %v", i, invRow.Weeks[i], want)
		}
		wantVal := want * 10 / 7.1788
		gotVal, ok := valRow.Weeks[i].(float64)
		if !ok || math.Abs(gotVal-wantVal) > 1e-9 {
			t.Errorf("Value[%d] = %v, want %v", i, valRow.Weeks[i], wantVal)
		}
	}
	for i := range monthEndTestWeeks() {
		if i == 2 || i == 8 {
			continue
		}
		if invRow.Weeks[i] != nil || valRow.Weeks[i] != nil {
			t.Errorf("non month-end column %d should be blank, got %v/%v", i, invRow.Weeks[i], valRow.Weeks[i])
		}
	}
	if math.Abs(sheet.TotalINV[2]-63) > 1e-9 || math.Abs(sheet.TotalINV[8]-111) > 1e-9 {
		t.Errorf("totals = %v", sheet.TotalINV)
	}
	if invRow.Index[9] != "B1" || invRow.Index[10] != "S1" || invRow.Index[11] != "C1" {
		t.Errorf("resolved chain = %v", invRow.Index)
	}
}

func TestBuildMonthEndCountsMissingPriceAndSkippedGroups(t *testing.T) {
	// A complete group without a price row still produces rows with value 0.
	sheet, err := BuildMonthEnd(MonthEndInput{
		Weeks: monthEndTestWeeks(), Rows: monthEndTestRows("I1"), ExchangeRate: 7.1788,
	})
	if err != nil {
		t.Fatal(err)
	}
	if sheet.MissingPrices != 1 || sheet.Groups != 1 {
		t.Fatalf("stats = %+v", sheet)
	}
	for _, row := range sheet.Rows {
		if row.Index[17] != "Month End Value" {
			continue
		}
		for i, v := range row.Weeks {
			if v != nil && v.(float64) != 0 {
				t.Errorf("missing price value[%d] = %v", i, v)
			}
		}
	}

	// A group with only 13 fields is skipped entirely.
	rows := monthEndTestRows("I2")
	sheet, err = BuildMonthEnd(MonthEndInput{Weeks: monthEndTestWeeks(), Rows: rows[:13], ExchangeRate: 7.1788})
	if err != nil {
		t.Fatal(err)
	}
	if sheet.SkippedGroups != 1 || len(sheet.Rows) != 0 {
		t.Fatalf("skipped stats = %+v rows=%d", sheet, len(sheet.Rows))
	}
}
