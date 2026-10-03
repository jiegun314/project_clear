package mps

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"strings"
)

// Differential formats (the <dxf> blocks a conditional-format rule paints
// with) are numbered per workbook: id 296 means one thing in one vendor's file
// and something else in the next. Merging rows from several workbooks therefore
// cannot keep the ids as they are — a rule would point at whatever happens to
// sit at that index in the exported file, and an id past the end of the table
// makes Excel refuse the sheet outright.
//
// So ids are interned per batch, exactly like cell styles: reading a workbook
// rewrites every rule's dxfId to a batch-wide id, and exporting appends those
// definitions to the target's styles part and maps each id to its new home.

// DxfDict interns differential formats by content.
type DxfDict struct {
	byHash map[string]int
	list   []string
}

// NewDxfDict creates an empty dictionary.
func NewDxfDict() *DxfDict { return &DxfDict{byHash: map[string]int{}} }

// Intern resolves a <dxf> block to its dictionary id, registering it on first
// sight. Two workbooks that use the same format share one entry.
func (d *DxfDict) Intern(xml string) int {
	sum := sha256.Sum256([]byte(xml))
	key := hex.EncodeToString(sum[:8])
	if id, ok := d.byHash[key]; ok {
		return id
	}
	id := len(d.list)
	d.list = append(d.list, xml)
	d.byHash[key] = id
	return id
}

// Len is the number of distinct formats collected so far.
func (d *DxfDict) Len() int { return len(d.list) }

// At returns the stored block for an id.
func (d *DxfDict) At(id int) (string, bool) {
	if id < 0 || id >= len(d.list) {
		return "", false
	}
	return d.list[id], true
}

// ParseDxfs returns the <dxf> blocks of a styles part, in table order, so a
// rule's dxfId can be resolved against the workbook it came from.
func ParseDxfs(stylesXML []byte) []string {
	elements := extractElements(string(stylesXML), "<dxf", "</dxf>")
	out := make([]string, 0, len(elements))
	for _, el := range elements {
		out = append(out, el.raw)
	}
	return out
}

var (
	dxfBGColorRe = regexp.MustCompile(`<bgColor[^>]*rgb="([0-9A-Fa-f]{6,8})"`)
	dxfFGColorRe = regexp.MustCompile(`<fgColor[^>]*rgb="([0-9A-Fa-f]{6,8})"`)
)

// DxfFillColors maps every differential format onto the fill colour it paints,
// as #RRGGBB. Differential formats used by these sheets put the colour in
// <bgColor>; <fgColor> is the fallback. Formats that only change fonts or
// borders yield "".
func DxfFillColors(dxfs []string) map[int]string {
	out := make(map[int]string, len(dxfs))
	for i, dxf := range dxfs {
		if !strings.Contains(dxf, "<fill>") {
			continue
		}
		match := dxfBGColorRe.FindStringSubmatch(dxf)
		if match == nil {
			match = dxfFGColorRe.FindStringSubmatch(dxf)
		}
		if match == nil {
			continue
		}
		if hex := normaliseHex(match[1]); hex != "" {
			out[i] = hex
		}
	}
	return out
}

// normaliseHex turns the 6- or 8-digit ARGB Excel writes into #RRGGBB.
func normaliseHex(v string) string {
	v = strings.ToUpper(strings.TrimSpace(v))
	switch len(v) {
	case 6:
		return "#" + v
	case 8:
		if v[:2] == "00" {
			return ""
		}
		return "#" + v[2:]
	}
	return ""
}

// remapSourceDxfs turns each rule's file-local dxfId into a batch-wide id. A
// rule whose id is outside the file's own table cannot be resolved, so its
// reference is dropped rather than carried over as a wrong colour.
func remapSourceDxfs(rows map[int]CFRow, dxfs []string, dict *DxfDict) {
	for row, prog := range rows {
		for bi := range prog.Blocks {
			for ri := range prog.Blocks[bi].Rules {
				rule := &prog.Blocks[bi].Rules[ri]
				id, ok := rule.DxfID()
				if !ok {
					continue
				}
				if id < 0 || id >= len(dxfs) {
					rule.SetDxfID(-1)
					continue
				}
				rule.SetDxfID(dict.Intern(dxfs[id]))
			}
		}
		rows[row] = prog
	}
}

// MergeDxfs writes extra differential formats into a styles part and reports
// where each one ended up, so the rules that reference them can be repointed.
//
// A format identical to one the workbook already carries reuses that index:
// re-exporting a template must not duplicate (or renumber) its own formats.
func MergeDxfs(stylesXML []byte, extra []string) ([]byte, []int, error) {
	doc := string(stylesXML)
	merged := ParseDxfs(stylesXML)
	index := make([]int, len(extra))
	at := make(map[string]int, len(merged))
	for i, dxf := range merged {
		if _, seen := at[dxf]; !seen {
			at[dxf] = i
		}
	}
	for i, dxf := range extra {
		if known, ok := at[dxf]; ok {
			index[i] = known
			continue
		}
		index[i] = len(merged)
		at[dxf] = len(merged)
		merged = append(merged, dxf)
	}
	if len(merged) == 0 {
		return stylesXML, index, nil
	}

	block := fmt.Sprintf(`<dxfs count="%d">%s</dxfs>`, len(merged), strings.Join(merged, ""))
	if existing := extractElements(doc, "<dxfs", "</dxfs>"); len(existing) > 0 {
		return []byte(strings.Replace(doc, existing[0].raw, block, 1)), index, nil
	}
	// No table yet: the schema puts dxfs after cellStyles and before
	// tableStyles, so aim for the first of those that exists.
	cut := -1
	for _, anchor := range []string{"<tableStyles", "</styleSheet>"} {
		if i := strings.Index(doc, anchor); i >= 0 && (cut < 0 || i < cut) {
			cut = i
		}
	}
	if cut < 0 {
		return nil, nil, fmt.Errorf("样式表缺少 tableStyles/styleSheet，无法写入条件格式")
	}
	return []byte(doc[:cut] + block + doc[cut:]), index, nil
}
