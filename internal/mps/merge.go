package mps

import (
	"fmt"
	"sort"
	"strings"
)

// MergedData is a 本地 workbook merged into the global rows.
//
// The rule is the one the business uses: an item the global data has keeps the
// global rows, and an item only the local file has brings its local rows. The
// result is expressed in the MPS layout, so the grid, the database and the export
// treat it exactly like a global workbook.
type MergedData struct {
	// Header is the result's columns: the global index names, and the weeks both
	// sides have in the global order.
	Header Header
	// Rows are the merged rows: the global ones first, in their own order, then the
	// rows of the items only the local file has.
	Rows []*DataRow
	// Sources is aligned to Rows and says which input row each one came from, so
	// the caller can keep the source file name and row number.
	Sources []RowSource `json:"-"`
	// FromGlobal and FromLocal are how many rows came from each side; Duplicates is
	// how many fully identical rows were collapsed into one.
	FromGlobal int `json:"fromGlobal"`
	FromLocal  int `json:"fromLocal"`
	Duplicates int `json:"duplicates"`
	// Replaced lists the items the local file also has, whose local rows were
	// dropped in favour of the global ones.
	Replaced []string `json:"replaced,omitempty"`
	// VisualsDropped counts rows whose conditional formats and annotations were
	// dropped because the merged week columns are narrower than the global ones:
	// those records address columns, and keeping them would point at the wrong ones.
	VisualsDropped int `json:"visualsDropped,omitempty"`
}

// RowSource identifies the input row a merged row came from: the global row slice
// or the local sheet, and that side's index.
type RowSource struct {
	Local bool
	Index int
}

// MergeLocal merges the local workbook into the global rows.
//
// Items are identified by the ITEM column, which is what the business compares on.
// The merged week columns are the ones both sides have — a week only one side has
// cannot be filled for the other — and every value is re-indexed by week code, so
// the result never depends on where the columns happened to sit.
func MergeLocal(global Header, rows []*DataRow, local *LocalSheet) (*MergedData, error) {
	if local == nil {
		return nil, fmt.Errorf("没有本地文件可合并")
	}
	localWeeks := map[string]bool{}
	for _, w := range local.Header.Weeks {
		localWeeks[w.Code] = true
	}
	var weeks []Week
	for _, w := range global.Weeks {
		if localWeeks[w.Code] {
			weeks = append(weeks, w)
		}
	}
	if len(weeks) == 0 {
		return nil, fmt.Errorf("本地文件与 global 没有共同的周（global: %s；本地: %s）",
			weekList(global.Weeks), weekList(local.Header.Weeks))
	}

	out := &MergedData{
		Header: Header{
			IndexNames: append([]string{}, global.IndexNames...),
			Weeks:      weeks,
		},
	}
	narrower := len(weeks) != len(global.Weeks)

	globalItems := map[string]bool{}
	for _, r := range rows {
		globalItems[itemOf(r)] = true
	}

	seen := map[string]bool{}
	for i, r := range rows {
		key := rowKey(r)
		if seen[key] {
			out.Duplicates++
			continue
		}
		seen[key] = true
		cp := *r
		cp.Weeks = alignValues(r.Weeks, global.Weeks, weeks)
		cp.StyleIDs = alignStyles(r.StyleIDs, global.Weeks, weeks, len(global.IndexNames))
		cp.CFColors = alignValues(r.CFColors, global.Weeks, weeks)
		if narrower {
			// Conditional formats and annotations address columns directly; with a
			// narrower week block they would move onto the wrong cells.
			cp.CF = CFRow{}
			cp.Comments = nil
			out.VisualsDropped++
		}
		out.Rows = append(out.Rows, &cp)
		out.Sources = append(out.Sources, RowSource{Index: i})
		out.FromGlobal++
	}

	localItems := map[string]bool{}
	for _, r := range local.Rows {
		localItems[itemOf(r)] = true
	}
	for item := range globalItems {
		if localItems[item] {
			out.Replaced = append(out.Replaced, item)
		}
	}
	sort.Strings(out.Replaced)

	for i, r := range local.Rows {
		if globalItems[itemOf(r)] {
			continue // the global data wins for this item
		}
		key := rowKey(r)
		if seen[key] {
			out.Duplicates++
			continue
		}
		seen[key] = true
		cp := *r
		cp.Weeks = alignValues(r.Weeks, local.Header.Weeks, weeks)
		// The local workbook is read for its values; it carries no styles, formats
		// or annotations yet, so the merged rows start without them.
		cp.StyleIDs = nil
		cp.CF = CFRow{}
		cp.Comments = nil
		cp.CFColors = nil
		cp.SourceRow = r.SourceRow
		out.Rows = append(out.Rows, &cp)
		out.Sources = append(out.Sources, RowSource{Local: true, Index: i})
		out.FromLocal++
	}
	return out, nil
}

// AlignedTo restates the merged rows in another week list — typically the global
// one — so every dataset of a batch shares one column layout. When the lists are
// the same it returns the rows unchanged.
//
// The merged week columns are the ones both sides have, while the batch keeps the
// global columns: a week the local file lacks leaves an empty cell rather than
// dropping a column the global data still fills.
func (m *MergedData) AlignedTo(weeks []Week) []*DataRow {
	if len(weeks) == len(m.Header.Weeks) {
		same := true
		for i := range weeks {
			if weeks[i].Code != m.Header.Weeks[i].Code {
				same = false
				break
			}
		}
		if same {
			return m.Rows
		}
	}
	out := make([]*DataRow, len(m.Rows))
	for i, r := range m.Rows {
		cp := *r
		cp.Weeks = alignValues(r.Weeks, m.Header.Weeks, weeks)
		cp.StyleIDs = alignStyles(r.StyleIDs, m.Header.Weeks, weeks, len(m.Header.IndexNames))
		cp.CFColors = alignValues(r.CFColors, m.Header.Weeks, weeks)
		if len(weeks) != len(m.Header.Weeks) {
			// These records name columns; a different width would point them at
			// the wrong cells.
			cp.CF = CFRow{}
			cp.Comments = nil
		}
		out[i] = &cp
	}
	return out
}

// itemOf is the ITEM cell, which is what identifies a product.
func itemOf(r *DataRow) string {
	if r == nil || len(r.Index) <= 8 {
		return ""
	}
	return strings.TrimSpace(r.Index[8])
}

// rowKey fingerprints everything the row shows, so "identical" means identical to
// the reader as well as to the eye.
func rowKey(r *DataRow) string {
	if r == nil {
		return ""
	}
	return strings.Join(r.Index[:], "\x1f") + "\x1e" + strings.Join(r.Weeks, "\x1f")
}

// alignValues re-indexes per-week values from one week list to another by code.
func alignValues(vals []string, from, to []Week) []string {
	if len(from) == 0 || len(to) == 0 {
		return nil
	}
	at := map[string]int{}
	for i, w := range from {
		at[w.Code] = i
	}
	out := make([]string, len(to))
	for i, w := range to {
		if j, ok := at[w.Code]; ok && j < len(vals) {
			out[i] = vals[j]
		}
	}
	return out
}

// alignStyles does the same for the style ids, which are one per column: A..O
// followed by the week columns.
func alignStyles(ids []int, from, to []Week, indexCols int) []int {
	if len(ids) == 0 {
		return nil
	}
	at := map[string]int{}
	for i, w := range from {
		at[w.Code] = i
	}
	out := make([]int, 0, indexCols+len(to))
	if indexCols > len(ids) {
		indexCols = len(ids)
	}
	out = append(out, ids[:indexCols]...)
	for _, w := range to {
		if j, ok := at[w.Code]; ok {
			if k := indexCols + j; k < len(ids) {
				out = append(out, ids[k])
				continue
			}
		}
		out = append(out, 0)
	}
	return out
}

// weekList renders week codes for an error message.
func weekList(ws []Week) string {
	if len(ws) == 0 {
		return "（无）"
	}
	codes := make([]string, len(ws))
	for i, w := range ws {
		codes[i] = w.Code
	}
	if len(codes) > 6 {
		return strings.Join(codes[:3], ",") + "…" + strings.Join(codes[len(codes)-2:], ",")
	}
	return strings.Join(codes, ",")
}
