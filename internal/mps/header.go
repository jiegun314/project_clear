// Package mps reads, merges and re-exports the MPS worksheets exported from the
// MPS system.
//
// Sheet layout, as established against the real workbooks in raw_data:
//
//	rows 1..54   decorative title block, never read
//	row  55      fixed index headers in columns A..O, week codes from P on
//	row  56      week start dates, stored as Excel serial numbers
//	row  57      visually blank but carries styling; must survive export
//	row  58..    data
//
// Columns A..O are a fixed index. Column L is LOC (the filter column) and
// column O carries the element type (AdjDmd, SchedRcpts, ...) even though its
// header also reads "LOC", so the natural key of a row is the full A..O tuple.
package mps

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// SheetName is the only worksheet this application reads.
const SheetName = "MPS"

const (
	// HeaderRow holds the index headers and the week codes.
	HeaderRow = 55
	// DateRow holds the week start dates.
	DateRow = 56
	// BlankRow separates the header from the data but is not empty: it
	// carries the styling of the report body, so it is preserved on export.
	BlankRow = 57
	// FirstDataRow is where data begins.
	FirstDataRow = 58
	// IndexCols is the width of the fixed A..O index block.
	IndexCols = 15
	// FirstWeekCol is column P.
	FirstWeekCol = 16
	// LOCCol is the filter column (L).
	LOCCol = 12
)

// Week is a single week column, stored as one unit so the week code and its
// start date can never drift apart. The source workbook keeps them on two rows
// (55 and 56); the UI and the exporter expand them back to two rows.
type Week struct {
	// Code is the 4-digit week code, e.g. 2639 = 2026 week 39.
	Code string `json:"code"`
	// Year is the 2-digit year taken from the code, e.g. 26.
	Year int `json:"year"`
	// WeekNo is the ISO week number taken from the code, e.g. 39.
	WeekNo int `json:"weekNo"`
	// Start is the Monday that opens the week.
	Start time.Time `json:"start"`
}

// FullYear expands the 2-digit year, treating 00..68 as 2000..2068.
func (w Week) FullYear() int { return 2000 + w.Year }

// StartText renders the Monday as YYYY-MM-DD.
func (w Week) StartText() string { return w.Start.Format("2006-01-02") }

// Label is the single-line form used by the one-row header mode.
func (w Week) Label() string { return fmt.Sprintf("%s / %s", w.Code, w.StartText()) }

// ParseWeekCode turns a raw header cell into a Week. Codes are 4 digits: the
// first two are the year offset from 2000, the last two the ISO week number.
func ParseWeekCode(raw string) (Week, error) {
	code := strings.TrimSpace(raw)
	// Tolerate a leading apostrophe or a numeric cell rendered as "2639.0".
	code = strings.TrimPrefix(code, "'")
	if i := strings.IndexByte(code, '.'); i > 0 {
		code = code[:i]
	}
	if len(code) != 4 {
		return Week{}, fmt.Errorf("周码格式非法: %q", raw)
	}
	n, err := strconv.Atoi(code)
	if err != nil {
		return Week{}, fmt.Errorf("周码格式非法: %q", raw)
	}
	yy, ww := n/100, n%100
	if ww < 1 || ww > 53 {
		return Week{}, fmt.Errorf("周码 %q 的周数超出 1..53", code)
	}
	year := 2000 + yy
	// ISO weeks 52 and 53 do not exist in every year; reject codes that point
	// at a week the calendar does not have. Compare against the ISO week-based
	// year rather than the calendar year of the Monday, which can still be in
	// December of the year before.
	start := ISOWeekStart(year, ww)
	if isoYear, isoWeek := start.ISOWeek(); isoWeek != ww || isoYear != year {
		return Week{}, fmt.Errorf("周码 %q 指向不存在的 ISO 周", code)
	}
	return Week{Code: code, Year: yy, WeekNo: ww, Start: start}, nil
}

// ISOWeekStart returns the Monday of the given ISO week.
func ISOWeekStart(year, week int) time.Time {
	// 4 January always falls in ISO week 1.
	d := time.Date(year, time.January, 4, 0, 0, 0, 0, time.UTC)
	wd := int(d.Weekday())
	if wd == 0 {
		wd = 7 // Sunday
	}
	monday := d.AddDate(0, 0, -(wd - 1))
	return monday.AddDate(0, 0, (week-1)*7)
}

// WeekFromSerial converts an Excel date serial (as stored on row 56) into the
// week it belongs to. The serial uses the 1900 date system with the leap-year
// quirk Excel keeps for backwards compatibility.
func WeekFromSerial(serial float64) (time.Time, error) {
	if serial <= 0 {
		return time.Time{}, fmt.Errorf("无效的日期序列号: %v", serial)
	}
	base := time.Date(1899, 12, 30, 0, 0, 0, 0, time.UTC)
	return base.AddDate(0, 0, int(serial)), nil
}

// Header describes the parsed header block of one file.
type Header struct {
	// IndexNames are the A..O headers, e.g. P5, P4, ... LOC.
	IndexNames []string `json:"indexNames"`
	// Weeks are the parsed week columns, in workbook order.
	Weeks []Week `json:"weeks"`
}

// Signature is a comparable fingerprint of the index headers, used to enforce
// the "headers must match exactly" merge rule.
func (h Header) Signature() string {
	return strings.Join(h.IndexNames, "\x1f")
}

// WeekSignature fingerprints the week column list.
func (h Header) WeekSignature() string {
	codes := make([]string, len(h.Weeks))
	for i, w := range h.Weeks {
		codes[i] = w.Code
	}
	return strings.Join(codes, "\x1f")
}

// FirstWeekCode is the code of the first week column, which names the archive
// table (2639 -> data_2639).
func (h Header) FirstWeekCode() string {
	if len(h.Weeks) == 0 {
		return ""
	}
	return h.Weeks[0].Code
}

// TotalCols is the width of the table we reproduce: index block plus weeks.
func (h Header) TotalCols() int { return IndexCols + len(h.Weeks) }

// LastColName is the final column letter of the reproduced table.
func (h Header) LastColName() string {
	n, err := ColumnName(h.TotalCols())
	if err != nil {
		return ""
	}
	return n
}

// ColumnName converts a 1-based column number to its letter.
func ColumnName(n int) (string, error) {
	if n < 1 {
		return "", fmt.Errorf("列号非法: %d", n)
	}
	name := ""
	for n > 0 {
		n--
		name = string(rune('A'+n%26)) + name
		n /= 26
	}
	return name, nil
}

// ColumnNumber converts a column letter to its 1-based number.
func ColumnNumber(name string) (int, error) {
	n := 0
	for _, r := range strings.ToUpper(name) {
		if r < 'A' || r > 'Z' {
			return 0, fmt.Errorf("列名非法: %q", name)
		}
		n = n*26 + int(r-'A'+1)
	}
	if n == 0 {
		return 0, fmt.Errorf("列名非法: %q", name)
	}
	return n, nil
}

// CellRef builds an A1-style reference.
func CellRef(col, row int) string {
	name, err := ColumnName(col)
	if err != nil {
		return ""
	}
	return name + strconv.Itoa(row)
}
