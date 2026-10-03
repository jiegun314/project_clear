package mps

import (
	"regexp"
	"strconv"
	"strings"
)

// MergeRange is one merged block of the sheet.
type MergeRange struct {
	Col1, Row1, Col2, Row2 int
}

var mergeCellRe = regexp.MustCompile(`<mergeCell[^>]*ref="([^"]+)"`)
var refRe = regexp.MustCompile(`^\$?([A-Z]{1,3})\$?(\d+)$`)

// ParseMergeRanges reads the <mergeCells> block of a worksheet. Excel keeps the
// value in the top-left cell of a merge and leaves the rest empty; the planner
// needs to see the same value on every row the merge covers, so the ranges are
// expanded while reading.
func ParseMergeRanges(sheetXML []byte) []MergeRange {
	out := []MergeRange{}
	for _, m := range mergeCellRe.FindAllSubmatch(sheetXML, -1) {
		parts := strings.Split(string(m[1]), ":")
		c1, r1, ok := parseRef(parts[0])
		if !ok {
			continue
		}
		c2, r2 := c1, r1
		if len(parts) == 2 {
			c2, r2, ok = parseRef(parts[1])
			if !ok {
				continue
			}
		}
		out = append(out, MergeRange{Col1: c1, Row1: r1, Col2: c2, Row2: r2})
	}
	return out
}

func parseRef(ref string) (col, row int, ok bool) {
	m := refRe.FindStringSubmatch(strings.TrimSpace(ref))
	if m == nil {
		return 0, 0, false
	}
	c, err := ColumnNumber(m[1])
	if err != nil {
		return 0, 0, false
	}
	r, err := strconv.Atoi(m[2])
	if err != nil {
		return 0, 0, false
	}
	return c, r, true
}

// mergeFiller carries the anchor values while rows stream past.
type mergeFiller struct {
	ranges []MergeRange
	// values[anchorCol] holds the anchor cell's text for the merge that starts
	// at the current anchor row.
	values map[int]string
}

func newMergeFiller(ranges []MergeRange) *mergeFiller {
	return &mergeFiller{ranges: ranges, values: map[int]string{}}
}

// expand copies the anchor value of every merge covering this row into the
// blank cells of that merge. Rows stream top-down, so the anchor row is always
// seen before the rows it covers.
func (f *mergeFiller) expand(cells []string, rowNum, total int) []string {
	if len(f.ranges) == 0 {
		return cells
	}
	for _, m := range f.ranges {
		if rowNum < m.Row1 || rowNum > m.Row2 {
			continue
		}
		last := m.Col2
		if last > total {
			last = total
		}
		for c := m.Col1; c <= last; c++ {
			if c < 1 {
				continue
			}
			for len(cells) < c {
				cells = append(cells, "")
			}
			if rowNum == m.Row1 {
				// Remember what the anchor row holds, so the rows below can
				// repeat it.
				if strings.TrimSpace(cells[c-1]) != "" {
					f.values[c] = cells[c-1]
				}
				continue
			}
			if strings.TrimSpace(cells[c-1]) != "" {
				continue
			}
			if v, ok := f.values[c]; ok {
				cells[c-1] = v
			}
		}
		// A horizontal merge keeps its value in the left-most cell only.
		if rowNum >= m.Row1 {
			anchor := ""
			if c := m.Col1; c-1 < len(cells) {
				anchor = cells[c-1]
			}
			if anchor == "" {
				anchor = f.values[m.Col1]
			}
			for c := m.Col1 + 1; c <= last && anchor != ""; c++ {
				if strings.TrimSpace(cells[c-1]) == "" {
					cells[c-1] = anchor
				}
			}
		}
	}
	return cells
}
