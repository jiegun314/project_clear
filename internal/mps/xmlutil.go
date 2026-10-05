package mps

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Small hand-rolled helpers for the machine-generated XML that Excel and
// excelize emit. A full XML round-trip is unnecessary here: the only edits are
// element removal and element insertion, and every element we touch is a leaf
// whose attributes we copy verbatim.

type xmlElement struct {
	raw   string // the complete element, tags included
	inner string // everything between the tags
	name  string // the element name, e.g. "cfRule"
	attrs string // the opening tag's attributes, without the name
}

// extractElements finds every non-nested element with the given tag names.
func extractElements(s, openTag, closeTag string) []xmlElement {
	var out []xmlElement
	i := 0
	for {
		start := strings.Index(s[i:], openTag)
		if start < 0 {
			return out
		}
		start += i
		// Guard against a longer tag name that merely starts the same way.
		if start+len(openTag) < len(s) {
			c := s[start+len(openTag)]
			if c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '>' && c != '/' {
				i = start + len(openTag)
				continue
			}
		}
		name, head, bodyStart, selfClosing, err := openTagEnd(s, start)
		if err != nil {
			i = start + len(openTag)
			continue
		}
		if selfClosing {
			raw := s[start:bodyStart]
			out = append(out, xmlElement{raw: raw, inner: "", name: name, attrs: head})
			i = bodyStart
			continue
		}
		closeIdx := strings.Index(s[bodyStart:], closeTag)
		if closeIdx < 0 {
			return out
		}
		closeIdx += bodyStart
		raw := s[start : closeIdx+len(closeTag)]
		out = append(out, xmlElement{raw: raw, inner: s[bodyStart:closeIdx], name: name, attrs: head})
		i = closeIdx + len(closeTag)
	}
}

// openTagEnd walks to the '>' that ends an opening tag, honouring quoted
// attribute values that may themselves contain '>'. It returns the element
// name and its attributes separately, so callers never have to re-split a head
// that still carries the tag name.
func openTagEnd(s string, start int) (name, attrs string, bodyStart int, selfClosing bool, err error) {
	// Skip '<' and the element name.
	i := start + 1
	nameStart := i
	for i < len(s) && !isSpace(s[i]) && s[i] != '>' && s[i] != '/' {
		i++
	}
	name = s[nameStart:i]
	attrStart := i
	inQuote := byte(0)
	for ; i < len(s); i++ {
		c := s[i]
		switch {
		case inQuote != 0:
			if c == inQuote {
				inQuote = 0
			}
		case c == '"' || c == '\'':
			inQuote = c
		case c == '>':
			raw := strings.TrimSpace(s[attrStart:i])
			if strings.HasSuffix(raw, "/") {
				return name, strings.TrimSpace(strings.TrimSuffix(raw, "/")), i + 1, true, nil
			}
			return name, raw, i + 1, false, nil
		}
	}
	return "", "", 0, false, fmt.Errorf("标签未闭合")
}

func splitElement(raw string) (attrs, inner string, selfClosing bool, err error) {
	var bodyStart int
	_, attrs, bodyStart, selfClosing, err = openTagEnd(raw, 0)
	if err != nil {
		return "", "", false, err
	}
	if selfClosing {
		return attrs, "", true, nil
	}
	name, _, _, _, err2 := openTagEnd(raw, 0)
	if err2 != nil {
		return attrs, raw[bodyStart:], false, nil
	}
	closeTag := "</" + name + ">"
	closeIdx := strings.LastIndex(raw, closeTag)
	if closeIdx < 0 || closeIdx < bodyStart {
		return attrs, raw[bodyStart:], false, nil
	}
	return attrs, raw[bodyStart:closeIdx], false, nil
}

func attrValue(head, name string) (string, error) {
	for _, field := range splitAttrs(head) {
		k, v, ok := strings.Cut(field, "=")
		if !ok {
			continue
		}
		if strings.TrimSpace(k) == name {
			return unescapeXML(strings.Trim(strings.TrimSpace(v), `"'`)), nil
		}
	}
	return "", fmt.Errorf("缺少属性 %s", name)
}

func replaceAttr(head, name, value string, replaced *bool) string {
	fields := splitAttrs(head)
	for i, field := range fields {
		k, _, ok := strings.Cut(field, "=")
		if ok && strings.TrimSpace(k) == name {
			fields[i] = fmt.Sprintf(`%s="%s"`, name, value)
			*replaced = true
			return strings.Join(fields, " ")
		}
	}
	return head
}

// splitAttrs breaks an attribute string on whitespace while keeping quoted
// values intact.
func splitAttrs(head string) []string {
	var out []string
	i := 0
	for i < len(head) {
		for i < len(head) && isSpace(head[i]) {
			i++
		}
		if i >= len(head) {
			break
		}
		start := i
		inQuote := byte(0)
		for i < len(head) {
			c := head[i]
			if inQuote != 0 {
				if c == inQuote {
					inQuote = 0
				}
			} else if c == '"' || c == '\'' {
				inQuote = c
			} else if isSpace(c) {
				break
			}
			i++
		}
		out = append(out, head[start:i])
	}
	return out
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func parseRangeRef(ref string) (c1, r1, c2, r2 int, err error) {
	parts := strings.SplitN(ref, ":", 2)
	if c1, r1, err = cellToCoords(parts[0]); err != nil {
		return
	}
	if len(parts) == 2 {
		c2, r2, err = cellToCoords(parts[1])
	} else {
		c2, r2 = c1, r1
	}
	return
}

func cellToCoords(cell string) (col, row int, err error) {
	// Excel writes upper case, but a lower-case reference is still valid and
	// must resolve rather than be silently skipped.
	cell = strings.ToUpper(strings.TrimSpace(strings.TrimPrefix(cell, "$")))
	i := 0
	for i < len(cell) && cell[i] >= 'A' && cell[i] <= 'Z' {
		col = col*26 + int(cell[i]-'A'+1)
		i++
	}
	if col == 0 || i >= len(cell) {
		return 0, 0, fmt.Errorf("非法单元格 %q", cell)
	}
	row, err = strconv.Atoi(cell[i:])
	if err != nil || row < 1 {
		return 0, 0, fmt.Errorf("非法单元格 %q", cell)
	}
	return col, row, nil
}

func rangeRef(c1, r1, c2, r2 int) (string, error) {
	a := CellRef(c1, r1)
	if a == "" {
		return "", fmt.Errorf("非法范围起点 %d,%d", c1, r1)
	}
	b := CellRef(c2, r2)
	if b == "" {
		return "", fmt.Errorf("非法范围终点 %d,%d", c2, r2)
	}
	return a + ":" + b, nil
}

func unescapeXML(s string) string {
	if !strings.ContainsRune(s, '&') {
		return s
	}
	r := strings.NewReplacer(
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&apos;", "'",
		"&amp;", "&",
	)
	return r.Replace(s)
}

func escapeXML(s string) string {
	r := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		`"`, "&quot;",
	)
	return r.Replace(s)
}

func sortInts(v []int) { sort.Ints(v) }
