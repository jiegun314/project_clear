package mps

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Conditional formats are the real source of the data colours in these
// workbooks: every data row carries its own block of expression rules that
// compare the row against its neighbours, e.g.
//
//	(ROUND(P180,0)<0)                        -> red
//	(ROUND(P180,0)<P181)                     -> amber
//	(ROUND(P180,0)>=P181*(1+$DN$56))         -> green
//
// Two properties make excelize's GetConditionalFormats unusable here:
//
//  1. it keys results by sqref, but these files emit up to four separate
//     blocks under the *same* sqref, so three of every four rules are dropped;
//  2. the formulas are relative to the row they are anchored on, so they must
//     be re-anchored whenever rows move.
//
// So conditional formats are read straight out of the sheet XML, stored with
// row numbers expressed as offsets from the anchor row, and re-emitted after
// excelize has written the file. The dxf records the rules point at are
// re-interned on the way through (see dxf.go): each source workbook numbers its
// own, so an id only means something inside the file it came from.
const (
	cfOpenTag  = "<conditionalFormatting"
	cfCloseTag = "</conditionalFormatting>"
	ruleOpen   = "<cfRule"
)

// offsetToken encodes "the anchor row plus d" inside a stored formula.
const offsetToken = "#o"

// absRowToken encodes "source row r" for a row-absolute reference, which moves
// up with the export exactly like the anchor row does.
const absRowToken = "#a"

// CFRule is one <cfRule> with its formulas stored relative to the anchor row.
type CFRule struct {
	// Attrs is the verbatim attribute string, e.g. ` type="expression" dxfId="296" priority="362" stopIfTrue="1"`.
	Attrs string `json:"attrs"`
	// RawInner is the rule body with <formula> contents left as-is, so rule
	// types without formulas (colorScale, iconSet, dataBar) round-trip intact.
	RawInner string `json:"rawInner"`
	// Formulas holds the row-relative formulas. Row references that are not
	// absolute appear as `#o<delta>` tokens; absolute ones such as `$DN$56`
	// are untouched.
	Formulas []string `json:"formulas"`
	// Offsets lists the delta of every non-absolute row reference in Formulas,
	// in order. Used to check that a rule is still portable after filtering.
	Offsets []int `json:"offsets"`
}

// CFBlock is one <conditionalFormatting> element restricted to a single row.
type CFBlock struct {
	C1    int      `json:"c1"`
	C2    int      `json:"c2"`
	Rules []CFRule `json:"rules"`
}

// CFRow is the complete conditional-format program for one data row.
type CFRow struct {
	Blocks []CFBlock `json:"blocks"`
}

// Empty reports whether the row carries no conditional formatting.
func (c CFRow) Empty() bool { return len(c.Blocks) == 0 }

// DxfID is the differential format this rule paints with, when it names one.
// Rules that carry their own formatting (colorScale, dataBar, iconSet) do not.
func (r CFRule) DxfID() (int, bool) {
	raw, err := attrValue(r.Attrs, "dxfId")
	if err != nil {
		return 0, false
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0, false
	}
	return n, true
}

// SetDxfID points the rule at another differential format. A negative id drops
// the reference, which is what a rule whose format is missing from the source
// has to do to stay loadable.
func (r *CFRule) SetDxfID(id int) {
	fields := splitAttrs(r.Attrs)
	kept := make([]string, 0, len(fields)+1)
	for _, field := range fields {
		if key, _, ok := strings.Cut(field, "="); ok && strings.TrimSpace(key) == "dxfId" {
			continue
		}
		kept = append(kept, field)
	}
	if id >= 0 {
		kept = append(kept, fmt.Sprintf(`dxfId="%d"`, id))
	}
	if len(kept) == 0 {
		r.Attrs = ""
		return
	}
	r.Attrs = " " + strings.Join(kept, " ")
}

// RemapDxfIDs repoints every rule's differential format at its home in the
// exported workbook. index reports where a stored id ended up; an unknown id
// drops the reference rather than leaving a dangling one behind.
func RemapDxfIDs(rows map[int]CFRow, index func(id int) (int, bool)) {
	for row, prog := range rows {
		for bi := range prog.Blocks {
			for ri := range prog.Blocks[bi].Rules {
				rule := &prog.Blocks[bi].Rules[ri]
				id, ok := rule.DxfID()
				if !ok {
					continue
				}
				at, found := index(id)
				if !found {
					at = -1
				}
				rule.SetDxfID(at)
			}
		}
		rows[row] = prog
	}
}

type rawCFBlock struct {
	sqref string
	inner string
}

// ParseCFRows extracts the conditional formats that apply to the data rows
// (row >= FirstDataRow) of a worksheet XML document, keyed by row number.
//
// Rules are re-based onto their anchor row: the row a formula refers to becomes
// an offset, so the same program can be re-applied to any destination row.
func ParseCFRows(sheetXML []byte) (map[int]CFRow, error) {
	return ParseCFRowsFrom(sheetXML, FirstDataRow)
}

// ParseCFRowsFrom is ParseCFRows with an explicit first row, for callers looking
// at a sheet whose data does not start where a source workbook's does — an
// export, for instance, which drops the decorative rows above the header.
func ParseCFRowsFrom(sheetXML []byte, fromRow int) (map[int]CFRow, error) {
	raw := extractElements(string(sheetXML), cfOpenTag, cfCloseTag)
	out := make(map[int]CFRow, len(raw))
	for _, blk := range raw {
		sqref, err := attrValue(blk.attrs, "sqref")
		if err != nil {
			return nil, fmt.Errorf("conditionalFormatting 缺少 sqref: %w", err)
		}
		rawRules, err := parseRawRules(blk.inner)
		if err != nil {
			return nil, err
		}
		// A sqref may list several space-separated areas; each area anchors
		// its own set of rules, so rebasing happens per row.
		for _, part := range strings.Fields(sqref) {
			c1, r1, c2, r2, err := parseRangeRef(part)
			if err != nil {
				return nil, fmt.Errorf("无法解析条件格式范围 %q: %w", part, err)
			}
			for r := r1; r <= r2; r++ {
				if r < fromRow {
					continue // the header block keeps the original formatting
				}
				rules, err := rebaseRules(rawRules, r)
				if err != nil {
					return nil, err
				}
				row := out[r]
				row.Blocks = append(row.Blocks, CFBlock{C1: c1, C2: c2, Rules: rules})
				out[r] = row
			}
		}
	}
	return out, nil
}

// HasDataAreaCF reports whether the document contains conditional formats that
// touch the data rows, i.e. whether rewriting is required at all.
func HasDataAreaCF(sheetXML []byte) bool {
	for _, blk := range extractElements(string(sheetXML), cfOpenTag, cfCloseTag) {
		sqref, err := attrValue(blk.attrs, "sqref")
		if err != nil {
			continue
		}
		for _, part := range strings.Fields(sqref) {
			_, r1, _, _, err := parseRangeRef(part)
			if err == nil && r1 >= FirstDataRow {
				return true
			}
		}
	}
	return false
}

// rawRule is a <cfRule> as written in the source, before row rebasing.
type rawRule struct {
	head     string
	body     string
	formulas []string
}

func parseRawRules(inner string) ([]rawRule, error) {
	var out []rawRule
	for _, el := range extractElements(inner, ruleOpen, "</cfRule>") {
		head, body, selfClosing, err := splitElement(el.raw)
		if err != nil {
			return nil, fmt.Errorf("解析 cfRule 失败: %w", err)
		}
		formulas, err := extractFormulas(body)
		if err != nil {
			return nil, err
		}
		_ = selfClosing
		out = append(out, rawRule{head: strings.TrimSpace(head), body: body, formulas: formulas})
	}
	return out, nil
}

// rebaseRules turns raw rules into rules whose formulas are expressed as
// offsets from anchorRow, so the program can be replayed on any destination
// row.
func rebaseRules(raws []rawRule, anchorRow int) ([]CFRule, error) {
	out := make([]CFRule, 0, len(raws))
	for _, r := range raws {
		stored := make([]string, 0, len(r.formulas))
		var offsets []int
		for _, f := range r.formulas {
			rebased, deltas, err := rebaseRefs(f, anchorRow)
			if err != nil {
				return nil, err
			}
			stored = append(stored, rebased)
			offsets = append(offsets, deltas...)
		}
		out = append(out, CFRule{
			Attrs:    r.head,
			RawInner: r.body,
			Formulas: stored,
			Offsets:  offsets,
		})
	}
	return out, nil
}

// extractFormulas decodes every <formula> body in a rule.
func extractFormulas(body string) ([]string, error) {
	var out []string
	i := 0
	for {
		start := strings.Index(body[i:], "<formula>")
		if start < 0 {
			return out, nil
		}
		start += i
		end := strings.Index(body[start:], "</formula>")
		if end < 0 {
			return nil, fmt.Errorf("<formula> 未闭合")
		}
		end += start
		out = append(out, unescapeXML(body[start+len("<formula>"):end]))
		i = end + len("</formula>")
	}
}

// rebaseRefs rewrites cell references so row numbers become offsets from
// anchorRow. Absolute row references (P$181, $DN$56) are preserved verbatim.
func rebaseRefs(formula string, anchorRow int) (string, []int, error) {
	var sb strings.Builder
	var deltas []int
	runes := []rune(formula)
	for i := 0; i < len(runes); {
		if runes[i] == '"' {
			// Copy string literals verbatim so their contents are never
			// mistaken for references.
			j := i + 1
			for j < len(runes) && runes[j] != '"' {
				j++
			}
			if j < len(runes) {
				j++
			}
			sb.WriteString(string(runes[i:j]))
			i = j
			continue
		}
		ref, next, ok := scanRef(runes, i)
		if !ok {
			sb.WriteRune(runes[i])
			i++
			continue
		}
		if ref.rowAbsolute {
			// An absolute row must travel with the sheet, not stay put: the
			// export drops the rows above the header, so $DN$56 has to become
			// $DN$2. Store the row as a token and resolve it at render time.
			sb.WriteString(ref.colAbs + ref.col + "$" + absRowToken + strconv.Itoa(ref.row))
		} else {
			d := ref.row - anchorRow
			deltas = append(deltas, d)
			sb.WriteString(ref.colAbs + ref.col + offsetToken + strconv.Itoa(d))
		}
		i = next
	}
	return sb.String(), deltas, nil
}

// applyOffsets is the inverse of rebaseRefs: it materialises a stored formula
// for a concrete destination row.
func applyOffsets(stored string, newRow int) string {
	return resolveRowRefs(stored, newRow)
}

// resolveRowRefs turns the stored row tokens back into real row numbers:
// "#o<delta>" is the anchor row plus a delta, "#a<row>" is a source row that
// shifts up with the export.
func resolveRowRefs(stored string, newRow int) string {
	var sb strings.Builder
	runes := []rune(stored)
	for i := 0; i < len(runes); {
		if runes[i] == '#' && i+1 < len(runes) && (runes[i+1] == 'o' || runes[i+1] == 'a') {
			kind := runes[i+1]
			j := i + 2
			neg := false
			if j < len(runes) && (runes[j] == '-' || runes[j] == '+') {
				neg = runes[j] == '-'
				j++
			}
			start := j
			for j < len(runes) && runes[j] >= '0' && runes[j] <= '9' {
				j++
			}
			if start < j {
				n, err := strconv.Atoi(string(runes[start:j]))
				if err == nil {
					if neg {
						n = -n
					}
					if kind == 'a' {
						sb.WriteString(strconv.Itoa(n - DropRows))
					} else {
						sb.WriteString(strconv.Itoa(newRow + n))
					}
					i = j
					continue
				}
			}
		}
		sb.WriteRune(runes[i])
		i++
	}
	return sb.String()
}

// shiftLiteralRows moves absolute row references that are still written out in
// full. Stored programs written before the export started dropping the rows
// above the header hold "$DN$56" rather than a token; without this they would
// point into the data block once the sheet is re-based.
func shiftLiteralRows(formula string) string {
	if !strings.Contains(formula, "$") {
		return formula
	}
	var sb strings.Builder
	runes := []rune(formula)
	for i := 0; i < len(runes); {
		ref, next, ok := scanRef(runes, i)
		if ok && ref.rowAbsolute && ref.row >= HeaderRow {
			sb.WriteString(ref.colAbs + ref.col + "$" + strconv.Itoa(ref.row-DropRows))
			i = next
			continue
		}
		sb.WriteRune(runes[i])
		i++
	}
	return sb.String()
}

type cellRefToken struct {
	col         string
	colAbs      string
	row         int
	rowAbsolute bool
}

// scanRef tries to read a cell reference starting at i. It rejects things that
// only look like one, such as the LOG10 in LOG10(A1) or a name ending in
// digits.
func scanRef(runes []rune, i int) (cellRefToken, int, bool) {
	start := i
	if runes[i] == '$' {
		i++
	}
	colStart := i
	for i < len(runes) && runes[i] >= 'A' && runes[i] <= 'Z' {
		i++
	}
	colLen := i - colStart
	if colLen < 1 || colLen > 3 {
		return cellRefToken{}, 0, false
	}
	col := string(runes[colStart:i])
	colAbs := ""
	if start < colStart {
		colAbs = "$"
	}
	if i < len(runes) && runes[i] == '$' {
		i++
	}
	rowStart := i
	for i < len(runes) && runes[i] >= '0' && runes[i] <= '9' {
		i++
	}
	if rowStart == i {
		return cellRefToken{}, 0, false
	}
	// Reject LOG10( and similar: a reference is never followed by a letter,
	// digit, underscore or an opening parenthesis.
	if i < len(runes) {
		c := runes[i]
		if c == '_' || c == '(' || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') {
			return cellRefToken{}, 0, false
		}
	}
	// Reject a preceding identifier character (e.g. the tail of a name).
	if start > 0 {
		p := runes[start-1]
		if p == '_' || p == '.' || (p >= 'A' && p <= 'Z') || (p >= 'a' && p <= 'z') || (p >= '0' && p <= '9') {
			return cellRefToken{}, 0, false
		}
	}
	row, err := strconv.Atoi(string(runes[rowStart:i]))
	if err != nil {
		return cellRefToken{}, 0, false
	}
	rowAbs := i-rowStart > 0 && runes[rowStart-1] == '$'
	return cellRefToken{col: col, colAbs: colAbs, row: row, rowAbsolute: rowAbs}, i, true
}

// RenderCFRows turns row programs into <conditionalFormatting> XML for the
// destination sheet, renumbering priorities so they do not collide with the
// blocks that were kept from the header area.
func RenderCFRows(rows map[int]CFRow, firstPriority int) (string, int, error) {
	keys := make([]int, 0, len(rows))
	for r := range rows {
		keys = append(keys, r)
	}
	sortInts(keys)

	var sb strings.Builder
	priority := firstPriority
	for _, row := range keys {
		for _, blk := range rows[row].Blocks {
			c1, c2 := blk.C1, blk.C2
			if c1 < FirstWeekCol {
				c1 = FirstWeekCol
			}
			if c2 < c1 {
				c2 = c1
			}
			ref, err := rangeRef(c1, row, c2, row)
			if err != nil {
				return "", 0, err
			}
			sb.WriteString(cfOpenTag + ` sqref="` + ref + `">`)
			for _, rule := range blk.Rules {
				sb.WriteString(ruleOpen + spaceIf(rewritePriority(rule.Attrs, priority)) + ">")
				sb.WriteString(renderRuleBody(rule, row))
				sb.WriteString("</cfRule>")
				priority++
			}
			sb.WriteString(cfCloseTag)
		}
	}
	return sb.String(), priority, nil
}

func renderRuleBody(rule CFRule, row int) string {
	if len(rule.Formulas) == 0 {
		return rule.RawInner
	}
	body := rule.RawInner
	idx := 0
	var sb strings.Builder
	for {
		start := strings.Index(body, "<formula>")
		if start < 0 {
			sb.WriteString(body)
			break
		}
		end := strings.Index(body[start:], "</formula>")
		if end < 0 {
			sb.WriteString(body)
			break
		}
		end += start
		sb.WriteString(body[:start])
		if idx < len(rule.Formulas) {
			sb.WriteString("<formula>")
			sb.WriteString(escapeXML(resolveRowRefs(shiftLiteralRows(rule.Formulas[idx]), row)))
			sb.WriteString("</formula>")
			idx++
		}
		body = body[end+len("</formula>"):]
	}
	return sb.String()
}

// spaceIf restores the separator between the tag name and its first attribute.
func spaceIf(attrs string) string {
	if attrs == "" || strings.HasPrefix(attrs, " ") {
		return attrs
	}
	return " " + attrs
}

func rewritePriority(attrs string, priority int) string {
	replaced := false
	out := replaceAttr(attrs, "priority", strconv.Itoa(priority), &replaced)
	if !replaced {
		return attrs + ` priority="` + strconv.Itoa(priority) + `"`
	}
	return out
}

// MaxCFPriority returns the highest priority in use by a document, so injected
// rules can be numbered above it.
func MaxCFPriority(sheetXML []byte) int {
	maxP := 0
	for _, blk := range extractElements(string(sheetXML), cfOpenTag, cfCloseTag) {
		for _, rule := range extractElements(blk.inner, ruleOpen, "</cfRule>") {
			head, _, _, err := splitElement(rule.raw)
			if err != nil {
				continue
			}
			v, err := attrValue(head, "priority")
			if err != nil {
				continue
			}
			if n, err := strconv.Atoi(v); err == nil && n > maxP {
				maxP = n
			}
		}
	}
	return maxP
}

// StripDataAreaCF removes every conditional format that touches the data rows
// while leaving the header block untouched.
func StripDataAreaCF(sheetXML []byte) []byte {
	raw := extractElements(string(sheetXML), cfOpenTag, cfCloseTag)
	rest := string(sheetXML)
	for _, blk := range raw {
		sqref, err := attrValue(blk.attrs, "sqref")
		if err != nil {
			continue
		}
		touchesData := false
		for _, part := range strings.Fields(sqref) {
			_, _, _, r2, err := parseRangeRef(part)
			if err == nil && r2 >= FirstDataRow {
				touchesData = true
				break
			}
		}
		if !touchesData {
			continue
		}
		rest = strings.Replace(rest, blk.raw, "", 1)
	}
	return []byte(rest)
}

// StripAllCF removes every conditional-format block from a worksheet. The
// export rebuilds the whole set — data rows and header alike — so anything the
// template still carries would either duplicate it or point at rows that moved
// when the sheet was re-based.
func StripAllCF(sheetXML []byte) []byte {
	rest := string(sheetXML)
	for _, blk := range extractElements(rest, cfOpenTag, cfCloseTag) {
		rest = strings.Replace(rest, blk.raw, "", 1)
	}
	return []byte(rest)
}

// CloneCFRow deep-copies a row program so callers can rebase it safely.
func CloneCFRow(in CFRow) CFRow {
	out := CFRow{Blocks: make([]CFBlock, len(in.Blocks))}
	for i, b := range in.Blocks {
		out.Blocks[i] = CFBlock{C1: b.C1, C2: b.C2, Rules: make([]CFRule, len(b.Rules))}
		copy(out.Blocks[i].Rules, b.Rules)
	}
	return out
}

// MarshalRow and UnmarshalRow move a row program through the database.
func MarshalRow(r CFRow) string {
	b, err := json.Marshal(r)
	if err != nil {
		return ""
	}
	return string(b)
}

// UnmarshalRow restores a row program; corrupt payloads degrade to "no
// conditional formatting" rather than failing the whole export.
func UnmarshalRow(s string) CFRow {
	var r CFRow
	if s == "" {
		return r
	}
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return CFRow{}
	}
	return r
}
