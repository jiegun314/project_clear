package mps

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// MonthEndIndexCols is the number of A..R columns on the Month End sheet: the
// merged MPS A..I block, then the resolved code chain and the second LOC.
const MonthEndIndexCols = 18

// MonthEndFirstDataRow is where the first 14-row group starts on Month End.
// Rows 1/2 hold the totals, row 3 the header, row 4 the dates and row 5 is the
// spacer the business file keeps blank.
const MonthEndFirstDataRow = 6

// MonthEndFields is the canonical field order of one 14-row group. The Month
// End page replaces AdjInbound and AdjOutbound with the two calculated fields,
// but keeps the other twelve rows untouched.
var MonthEndFields = []string{
	"AdjDmd",
	"SchedRcpts",
	"PlanOrders",
	"DeployIn",
	"DeployOut",
	"AdjInbound",
	"AdjOutbound",
	"ProxyDemand",
	"ProxySupply",
	"CalcOH",
	"CalcOH2",
	"SS",
	"WOS",
	"BackOrder $",
}

// MonthEndHeaders is the A..R header row.
var MonthEndHeaders = []string{
	"P5", "P4", "P3", "P2", "P1",
	"MFG GROUP", "MFG CLASS CODE", "MFG DESCR", "ITEM",
	"Base Code", "SAP Code", "Category",
	"US CATALOG", "EU CATALOG", "LOC", "BO", "OH", "LOC",
}

// MonthEndPrice is the resolved master-data chain for an Item. StandardCost is
// nil when the SAP code has no price row.
type MonthEndPrice struct {
	BaseCode     string
	SapCode      string
	Category     string
	StandardCost *float64
}

// MonthEndInput is everything needed to build one Month End sheet.
type MonthEndInput struct {
	Weeks        []Week
	Rows         []ExportRow
	PriceByItem  map[string]MonthEndPrice
	ExchangeRate float64
	// Columns optionally overrides the automatically detected month-end
	// columns. Indices are zero-based into Weeks. It exists for tests and for a
	// future explicit schedule; nil means the 4-4-5 fiscal period starts.
	Columns []int
}

// MonthEndRow is one output row: A..R followed by the week values.
type MonthEndRow struct {
	Index [MonthEndIndexCols]string
	Weeks []any
}

// MonthEndSheet is the calculated Month End page.
type MonthEndSheet struct {
	Weeks        []Week
	Rows         []MonthEndRow
	TotalINV     []float64
	TotalValue   []float64
	MonthEndCols []int

	// Groups is the number of complete 14-row groups written.
	Groups int
	// MissingPrices counts groups with no usable standard cost. Their value is
	// written as zero and this count is reported as a warning.
	MissingPrices int
	// SkippedGroups counts groups that were not exactly one complete 14-field
	// set and were therefore not written.
	SkippedGroups int
}

// LastMondayColumns returns the zero-based week columns whose start date is the
// last Monday of its calendar month. A month that is not complete in the
// horizon has no column: the rule is based on the Monday seven days later
// crossing into a new month. It is kept for callers that want calendar months;
// the business Month End sheet uses the 4-4-5 fiscal calendar below.
func LastMondayColumns(weeks []Week) []int {
	out := make([]int, 0, 6)
	for i, w := range weeks {
		if w.Start.IsZero() {
			continue
		}
		next := w.Start.AddDate(0, 0, 7)
		if next.Month() != w.Start.Month() {
			out = append(out, i)
		}
	}
	return out
}

// fiscalMonthEndAnchor is the first period start visible in the business
// samples: 2026-09-21 (week 2639). From there the fiscal calendar uses four
// weeks, four weeks and then six weeks for the 53-week 2026 fourth quarter,
// followed by the normal 4-4-5 cycle. The resulting starts are 2026-09-21,
// 2026-10-19, 2026-11-16, 2026-12-28, 2027-01-25, 2027-02-22, 2027-03-29,
// 2027-04-26, matching both supplied Month End workbooks.
var fiscalMonthEndAnchor = time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)

// FiscalMonthEndColumns returns the week columns that start a fiscal period.
// The business writes the previous period's Month End INV/Value in the first
// week of the next period, so these period-start columns are the Month End
// columns. The 4-4-5 cycle is generated far enough forward for normal exports;
// weeks before the first sample fall back to the start of the visible horizon.
func FiscalMonthEndColumns(weeks []Week) []int {
	starts := map[string]bool{}
	date := fiscalMonthEndAnchor
	starts[date.Format("2006-01-02")] = true
	// 2026 Q4 is the 53-week quarter: 4 + 4 + 6 weeks.
	for _, n := range []int{4, 4, 6} {
		date = date.AddDate(0, 0, n*7)
		starts[date.Format("2006-01-02")] = true
	}
	// Normal 4-4-5 quarters. Generate a long horizon so a weekly export keeps
	// working beyond 2027.
	cycle := []int{4, 4, 5}
	for i := 0; i < 60; i++ {
		date = date.AddDate(0, 0, cycle[i%len(cycle)]*7)
		starts[date.Format("2006-01-02")] = true
	}
	out := make([]int, 0, 6)
	for i, w := range weeks {
		if w.Start.IsZero() {
			continue
		}
		if starts[w.Start.Format("2006-01-02")] {
			out = append(out, i)
		}
	}
	return out
}

// BuildMonthEnd calculates the Month End page from the merged rows.
//
// For every (ITEM, warehouse) group the current CalOH plus the next three weeks
// of SchedRcpts and DeployIn gives Month End INV at each month-end column; the
// value is INV × standard cost / exchange rate. The two fields are blank in all
// other week columns, matching the business 4-4-5 samples. The other twelve rows are
// copied through unchanged.
func BuildMonthEnd(in MonthEndInput) (*MonthEndSheet, error) {
	if len(in.Weeks) == 0 {
		return nil, fmt.Errorf("Month End 没有可写入的周数据列")
	}
	if in.ExchangeRate <= 0 {
		return nil, fmt.Errorf("汇率必须大于 0")
	}
	cols := in.Columns
	if cols == nil {
		cols = FiscalMonthEndColumns(in.Weeks)
	}
	cols = validColumns(cols, len(in.Weeks))

	type group struct {
		key  string
		item string
		rows []ExportRow
	}
	groups := make([]*group, 0, 160)
	byKey := map[string]*group{}
	for _, row := range in.Rows {
		if len(row.Index) < IndexCols {
			continue
		}
		item := strings.TrimSpace(row.Index[8])
		wh := strings.TrimSpace(row.Index[11])
		if item == "" || wh == "" {
			continue
		}
		key := item + "\x00" + wh
		g := byKey[key]
		if g == nil {
			g = &group{key: key, item: item}
			byKey[key] = g
			groups = append(groups, g)
		}
		g.rows = append(g.rows, row)
	}

	out := &MonthEndSheet{
		Weeks:        in.Weeks,
		Rows:         make([]MonthEndRow, 0, len(groups)*len(MonthEndFields)),
		TotalINV:     make([]float64, len(in.Weeks)),
		TotalValue:   make([]float64, len(in.Weeks)),
		MonthEndCols: cols,
	}
	for _, g := range groups {
		if len(g.rows) != len(MonthEndFields) {
			out.SkippedGroups++
			continue
		}
		byField := make(map[string]ExportRow, len(g.rows))
		for _, row := range g.rows {
			if len(row.Index) < IndexCols {
				continue
			}
			byField[monthEndCanonicalField(strings.TrimSpace(row.Index[14]))] = row
		}
		complete := true
		for _, f := range MonthEndFields {
			if _, ok := byField[f]; !ok {
				complete = false
				break
			}
		}
		if !complete {
			out.SkippedGroups++
			continue
		}

		price, hasPrice := in.PriceByItem[g.item]
		cost := 0.0
		if hasPrice && price.StandardCost != nil {
			cost = *price.StandardCost
		} else {
			out.MissingPrices++
		}

		inv := make([]float64, len(in.Weeks))
		val := make([]float64, len(in.Weeks))
		for _, j := range cols {
			inv[j] = weekNumber(byField["CalcOH"].Weeks, j)
			for k := 1; k <= 3 && j+k < len(in.Weeks); k++ {
				inv[j] += weekNumber(byField["SchedRcpts"].Weeks, j+k)
				inv[j] += weekNumber(byField["DeployIn"].Weeks, j+k)
			}
			val[j] = inv[j] * cost / in.ExchangeRate
			out.TotalINV[j] += inv[j]
			out.TotalValue[j] += val[j]
		}

		for _, field := range MonthEndFields {
			src := byField[field]
			outField := field
			switch field {
			case "AdjInbound":
				outField = "Month End INV"
			case "AdjOutbound":
				outField = "Month End Value"
			}
			var r MonthEndRow
			if len(src.Index) == IndexCols {
				for i := 0; i < 9; i++ {
					r.Index[i] = src.Index[i]
				}
				r.Index[9] = price.BaseCode
				r.Index[10] = price.SapCode
				r.Index[11] = price.Category
				r.Index[12] = src.Index[9]
				r.Index[13] = src.Index[10]
				r.Index[14] = src.Index[11]
				r.Index[15] = src.Index[12]
				r.Index[16] = src.Index[13]
				r.Index[17] = outField
			}
			r.Weeks = cloneWeeks(src.Weeks, len(in.Weeks))
			switch field {
			case "AdjInbound":
				for j := range r.Weeks {
					r.Weeks[j] = nil
				}
				for _, j := range cols {
					r.Weeks[j] = inv[j]
				}
			case "AdjOutbound":
				for j := range r.Weeks {
					r.Weeks[j] = nil
				}
				for _, j := range cols {
					r.Weeks[j] = val[j]
				}
			}
			out.Rows = append(out.Rows, r)
		}
		out.Groups++
	}
	return out, nil
}

// monthEndCanonicalField normalizes source labels. Older Local/Month End
// workbooks already carry the calculated field names, while MPS workbooks use
// AdjInbound/AdjOutbound; the calculation needs one canonical key.
func monthEndCanonicalField(name string) string {
	switch strings.ToUpper(strings.Join(strings.Fields(name), " ")) {
	case "MONTH END INV", "MONTH END INVENTORY":
		return "AdjInbound"
	case "MONTH END VALUE", "MONTH END INV (VALUE)", "MONTH END INV VALUE":
		return "AdjOutbound"
	default:
		return name
	}
}

func validColumns(cols []int, n int) []int {
	out := make([]int, 0, len(cols))
	seen := map[int]bool{}
	for _, c := range cols {
		if c >= 0 && c < n && !seen[c] {
			out = append(out, c)
			seen[c] = true
		}
	}
	return out
}

func cloneWeeks(in []any, n int) []any {
	out := make([]any, n)
	for i := 0; i < n && i < len(in); i++ {
		out[i] = in[i]
	}
	return out
}

func weekNumber(in []any, i int) float64 {
	if i < 0 || i >= len(in) || in[i] == nil {
		return 0
	}
	switch v := in[i].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	case string:
		s := strings.TrimSpace(strings.ReplaceAll(v, ",", ""))
		if s == "" {
			return 0
		}
		n, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return n
	default:
		return 0
	}
}

// MonthEndColumnName is the spreadsheet column of a zero-based week index.
func MonthEndColumnName(i int) string {
	name, _ := ColumnName(MonthEndIndexCols + 1 + i)
	return name
}

// MonthEndDate is a convenience for tests and callers that build a schedule.
func MonthEndDate(y int, m time.Month, d int) time.Time {
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}
