package mps

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"unicode"
)

// The MPS sheets paint their signal colours with conditional formatting rather
// than with a cell fill: every "CalcOH" / "CalcOH2" row carries four rules
// comparing the cell with itself, with the row two below (the SS row) and with
// a tolerance parameter stored in an absolute cell such as $DN$56. The rules
// only use a handful of shapes — this file evaluates exactly those, so the grid
// can show the colour Excel would show.
//
// The exporter never uses this: it keeps the rules themselves, and Excel
// re-evaluates them. This is display only.

// cellLookup returns the raw text of a cell. row and col are 1-based.
type cellLookup func(row, col int) string

// EvalCFColors returns, for one row program, the fill colour a cell gets from
// its conditional formats, keyed by 1-based column. Rules are tried in
// priority order and the first match wins (every rule in these sheets sets
// stopIfTrue).
func EvalCFColors(prog CFRow, anchorRow int, dxfColors map[int]string, lookup cellLookup) map[int]string {
	if prog.Empty() {
		return nil
	}
	type rule struct {
		col1, col2 int
		priority   int
		dxfID      int
		hasDxf     bool
		formulas   []string
		operator   string
		kind       string
	}
	var rules []rule
	for _, block := range prog.Blocks {
		for _, r := range block.Rules {
			prio := 1 << 30
			if raw, err := attrValue(r.Attrs, "priority"); err == nil {
				if n, err := strconv.Atoi(strings.TrimSpace(raw)); err == nil {
					prio = n
				}
			}
			kind, _ := attrValue(r.Attrs, "type")
			op, _ := attrValue(r.Attrs, "operator")
			id, has := r.DxfID()
			rules = append(rules, rule{
				col1: block.C1, col2: block.C2, priority: prio,
				dxfID: id, hasDxf: has, formulas: r.Formulas,
				operator: strings.TrimSpace(op), kind: strings.TrimSpace(kind),
			})
		}
	}
	// Lower priority number wins; stable so document order breaks ties.
	for i := 1; i < len(rules); i++ {
		for j := i; j > 0 && rules[j].priority < rules[j-1].priority; j-- {
			rules[j], rules[j-1] = rules[j-1], rules[j]
		}
	}

	out := map[int]string{}
	for _, r := range rules {
		if !r.hasDxf {
			continue
		}
		color := dxfColors[r.dxfID]
		if color == "" {
			continue
		}
		for c := r.col1; c <= r.col2; c++ {
			if _, done := out[c]; done {
				continue
			}
			hit, err := evalRule(r.kind, r.operator, r.formulas, anchorRow, c, lookup)
			if err != nil || !hit {
				continue
			}
			out[c] = color
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func evalRule(kind, operator string, formulas []string, row, col int, lookup cellLookup) (bool, error) {
	switch kind {
	case "expression":
		if len(formulas) == 0 {
			return false, fmt.Errorf("表达式规则没有公式")
		}
		v, err := evalFormula(formulas[0], row, col, lookup)
		if err != nil {
			return false, err
		}
		return truthy(v), nil
	case "cellIs":
		if len(formulas) < 1 {
			return false, fmt.Errorf("cellIs 规则没有公式")
		}
		left, err := evalFormula("REF", row, col, lookup)
		if err != nil {
			return false, err
		}
		right, err := evalFormula(formulas[0], row, col, lookup)
		if err != nil {
			return false, err
		}
		return compare(operator, left, right), nil
	}
	return false, fmt.Errorf("暂不支持的条件格式类型 %q", kind)
}

// --- expression evaluation -------------------------------------------------

type cfValue struct {
	num      float64
	str      string
	text     bool
	bl       bool
	rangeCol int
	kind     byte // 'n' number, 't' text, 'b' bool, 'r' column range
}

func numValue(f float64) cfValue    { return cfValue{num: f, kind: 'n'} }
func textValue(s string) cfValue    { return cfValue{str: s, text: true, kind: 't'} }
func boolValue(b bool) cfValue      { return cfValue{bl: b, kind: 'b'} }
func rangeValue(col int) cfValue    { return cfValue{rangeCol: col, kind: 'r'} }
func (v cfValue) asNumber() float64 { return v.num }
func (v cfValue) isText() bool      { return v.kind == 't' }
func (v cfValue) asText() string    { return v.str }
func truthy(v cfValue) bool {
	switch v.kind {
	case 'b':
		return v.bl
	case 't':
		s := strings.TrimSpace(v.str)
		return strings.EqualFold(s, "true")
	default:
		return v.num != 0
	}
}

func compare(op string, a, b cfValue) bool {
	if a.kind == 't' || b.kind == 't' {
		x, y := a.asText(), b.asText()
		switch op {
		case "=":
			return x == y
		case "<>":
			return x != y
		case "<":
			return x < y
		case "<=":
			return x <= y
		case ">":
			return x > y
		case ">=":
			return x >= y
		}
		return false
	}
	x, y := a.num, b.num
	switch op {
	case "=":
		return x == y
	case "<>":
		return x != y
	case "<":
		return x < y
	case "<=":
		return x <= y
	case ">":
		return x > y
	case ">=":
		return x >= y
	}
	return false
}

func evalFormula(formula string, row, col int, lookup cellLookup) (cfValue, error) {
	p := &cfParser{src: []rune(strings.TrimSpace(formula)), row: row, col: col, lookup: lookup}
	v, err := p.parseExpr()
	if err != nil {
		return cfValue{}, err
	}
	p.skipSpace()
	if p.pos != len(p.src) {
		return cfValue{}, fmt.Errorf("公式 %q 有多余内容", formula)
	}
	return v, nil
}

type cfParser struct {
	src    []rune
	pos    int
	row    int
	col    int
	lookup cellLookup
}

func (p *cfParser) skipSpace() {
	for p.pos < len(p.src) && unicode.IsSpace(p.src[p.pos]) {
		p.pos++
	}
}

func (p *cfParser) peek() rune {
	if p.pos < len(p.src) {
		return p.src[p.pos]
	}
	return 0
}

func (p *cfParser) has(prefix string) bool {
	p.skipSpace()
	return strings.HasPrefix(string(p.src[p.pos:]), prefix)
}

func (p *cfParser) take(prefix string) bool {
	if !p.has(prefix) {
		return false
	}
	p.pos += len([]rune(prefix))
	return true
}

// expr := cmp
func (p *cfParser) parseExpr() (cfValue, error) {
	left, err := p.parseSum()
	if err != nil {
		return cfValue{}, err
	}
	for _, op := range []string{"<=", ">=", "<>", "<", ">", "="} {
		if p.take(op) {
			right, err := p.parseSum()
			if err != nil {
				return cfValue{}, err
			}
			return boolValue(compare(op, left, right)), nil
		}
	}
	return left, nil
}

func (p *cfParser) parseSum() (cfValue, error) {
	left, err := p.parseTerm()
	if err != nil {
		return cfValue{}, err
	}
	for {
		switch {
		case p.take("+"):
			right, err := p.parseTerm()
			if err != nil {
				return cfValue{}, err
			}
			left = numValue(left.asNumber() + right.asNumber())
		case p.take("-"):
			right, err := p.parseTerm()
			if err != nil {
				return cfValue{}, err
			}
			left = numValue(left.asNumber() - right.asNumber())
		default:
			return left, nil
		}
	}
}

func (p *cfParser) parseTerm() (cfValue, error) {
	left, err := p.parseUnary()
	if err != nil {
		return cfValue{}, err
	}
	for {
		switch {
		case p.take("*"):
			right, err := p.parseUnary()
			if err != nil {
				return cfValue{}, err
			}
			left = numValue(left.asNumber() * right.asNumber())
		case p.take("/"):
			right, err := p.parseUnary()
			if err != nil {
				return cfValue{}, err
			}
			if right.asNumber() == 0 {
				return cfValue{}, fmt.Errorf("除以零")
			}
			left = numValue(left.asNumber() / right.asNumber())
		default:
			return left, nil
		}
	}
}

func (p *cfParser) parseUnary() (cfValue, error) {
	if p.take("-") {
		v, err := p.parseUnary()
		if err != nil {
			return cfValue{}, err
		}
		return numValue(-v.asNumber()), nil
	}
	if p.take("+") {
		return p.parseUnary()
	}
	return p.parsePrimary()
}

func (p *cfParser) parsePrimary() (cfValue, error) {
	p.skipSpace()
	if p.pos >= len(p.src) {
		return cfValue{}, fmt.Errorf("公式意外结束")
	}
	ch := p.peek()
	switch {
	case ch == '(':
		p.pos++
		v, err := p.parseExpr()
		if err != nil {
			return cfValue{}, err
		}
		if !p.take(")") {
			return cfValue{}, fmt.Errorf("缺少右括号")
		}
		return v, nil
	case ch == '"':
		p.pos++
		var sb strings.Builder
		for p.pos < len(p.src) && p.src[p.pos] != '"' {
			sb.WriteRune(p.src[p.pos])
			p.pos++
		}
		if p.pos >= len(p.src) {
			return cfValue{}, fmt.Errorf("字符串未闭合")
		}
		p.pos++ // closing quote
		return textValue(sb.String()), nil
	case unicode.IsDigit(ch) || ch == '.':
		start := p.pos
		for p.pos < len(p.src) && (unicode.IsDigit(p.src[p.pos]) || p.src[p.pos] == '.') {
			p.pos++
		}
		f, err := strconv.ParseFloat(string(p.src[start:p.pos]), 64)
		if err != nil {
			return cfValue{}, err
		}
		return numValue(f), nil
	case unicode.IsLetter(ch) || ch == '$':
		return p.parseName()
	}
	return cfValue{}, fmt.Errorf("无法解析的公式片段 %q", string(p.src[p.pos:]))
}

// parseName handles TRUE/FALSE, function calls and cell references.
func (p *cfParser) parseName() (cfValue, error) {
	start := p.pos
	// A reference may start with '$' and contain letters, digits and the row
	// tokens "#o12" / "$#a181" the rebasing step writes.
	for p.pos < len(p.src) {
		ch := p.src[p.pos]
		if unicode.IsLetter(ch) || unicode.IsDigit(ch) || ch == '$' || ch == '#' || ch == '_' {
			p.pos++
			continue
		}
		break
	}
	token := string(p.src[start:p.pos])
	// A column range such as $DE:$DE (used by INDEX(...,ROW())).
	if p.peek() == ':' {
		p.pos++
		second := p.pos
		for p.pos < len(p.src) && (unicode.IsLetter(p.src[p.pos]) || p.src[p.pos] == '$') {
			p.pos++
		}
		first, ok1 := columnOfToken(token)
		other, ok2 := columnOfToken(string(p.src[second:p.pos]))
		if ok1 && ok2 && first == other {
			return rangeValue(first), nil
		}
		return cfValue{}, fmt.Errorf("不支持的区间 %q", token+":"+string(p.src[second:p.pos]))
	}
	if p.take("(") {
		var args []cfValue
		if !p.take(")") {
			for {
				v, err := p.parseExpr()
				if err != nil {
					return cfValue{}, err
				}
				args = append(args, v)
				if p.take(",") {
					continue
				}
				if !p.take(")") {
					return cfValue{}, fmt.Errorf("函数 %s 缺少右括号", token)
				}
				break
			}
		}
		return p.callFunc(strings.ToUpper(token), args)
	}
	upper := strings.ToUpper(token)
	if upper == "TRUE" {
		return boolValue(true), nil
	}
	if upper == "FALSE" {
		return boolValue(false), nil
	}
	row, col, ok := resolveRef(token, p.row, p.col)
	if !ok {
		return cfValue{}, fmt.Errorf("无法解析引用 %q", token)
	}
	raw := p.lookup(row, col)
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return numValue(0), nil
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return numValue(f), nil
	}
	return textValue(raw), nil
}

func (p *cfParser) callFunc(name string, args []cfValue) (cfValue, error) {
	switch name {
	case "ROUND":
		if len(args) < 1 {
			return cfValue{}, fmt.Errorf("ROUND 缺少参数")
		}
		digits := 0
		if len(args) > 1 {
			digits = int(args[1].asNumber())
		}
		return numValue(roundTo(args[0].asNumber(), digits)), nil
	case "AND":
		for _, a := range args {
			if !truthy(a) {
				return boolValue(false), nil
			}
		}
		return boolValue(true), nil
	case "OR":
		for _, a := range args {
			if truthy(a) {
				return boolValue(true), nil
			}
		}
		return boolValue(false), nil
	case "IF":
		if len(args) < 2 {
			return cfValue{}, fmt.Errorf("IF 缺少参数")
		}
		if truthy(args[0]) {
			return args[1], nil
		}
		if len(args) > 2 {
			return args[2], nil
		}
		return boolValue(false), nil
	case "ROW":
		return numValue(float64(p.row)), nil
	case "COLUMN":
		return numValue(float64(p.col)), nil
	case "INDEX":
		// INDEX($DE:$DE, ROW()) — a single column, one row chosen by ROW().
		if len(args) < 2 || args[0].kind != 'r' {
			return cfValue{}, fmt.Errorf("INDEX 缺少参数")
		}
		row := int(args[1].asNumber())
		return p.cellAt(row, args[0].rangeCol), nil
	case "ABS":
		if len(args) < 1 {
			return cfValue{}, fmt.Errorf("ABS 缺少参数")
		}
		return numValue(math.Abs(args[0].asNumber())), nil
	}
	return cfValue{}, fmt.Errorf("暂不支持的函数 %s", name)
}

// cellAt reads one cell of the sheet through the lookup.
func (p *cfParser) cellAt(row, col int) cfValue {
	raw := strings.TrimSpace(p.lookup(row, col))
	if raw == "" {
		return numValue(0)
	}
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return numValue(f)
	}
	return textValue(raw)
}

// columnOfToken turns "$DE" into the 1-based column number.
func columnOfToken(token string) (int, bool) {
	name := strings.ToUpper(strings.TrimPrefix(strings.TrimSpace(token), "$"))
	if name == "" {
		return 0, false
	}
	for _, r := range name {
		if !unicode.IsLetter(r) {
			return 0, false
		}
	}
	c, err := ColumnNumber(name)
	if err != nil {
		return 0, false
	}
	return c, true
}

// roundTo mirrors Excel's ROUND: half away from zero at the given digit.
func roundTo(v float64, digits int) float64 {
	factor := math.Pow(10, float64(digits))
	scaled := v * factor
	if scaled < 0 {
		return -math.Floor(-scaled+0.5) / factor
	}
	return math.Floor(scaled+0.5) / factor
}

// resolveRef turns a stored reference such as "P#o2", "P$#a181" or "$DN$56"
// into a concrete (row, col). anchorRow is the row the formula was captured on.
func resolveRef(token string, anchorRow, anchorCol int) (row, col int, ok bool) {
	colAbs := strings.HasPrefix(token, "$")
	t := strings.TrimPrefix(token, "$")
	i := 0
	for i < len(t) && unicode.IsLetter(rune(t[i])) {
		i++
	}
	if i == 0 {
		return 0, 0, false
	}
	colName := t[:i]
	rest := t[i:]
	c, err := ColumnNumber(strings.ToUpper(colName))
	if err != nil {
		return 0, 0, false
	}
	_ = colAbs
	switch {
	case strings.HasPrefix(rest, offsetToken):
		d, err := strconv.Atoi(rest[len(offsetToken):])
		if err != nil {
			return 0, 0, false
		}
		return anchorRow + d, c, true
	case strings.HasPrefix(rest, "$"+absRowToken):
		r, err := strconv.Atoi(rest[len("$"+absRowToken):])
		if err != nil {
			return 0, 0, false
		}
		return r, c, true
	case strings.HasPrefix(rest, absRowToken):
		r, err := strconv.Atoi(rest[len(absRowToken):])
		if err != nil {
			return 0, 0, false
		}
		return r, c, true
	case strings.HasPrefix(rest, "$"):
		// $DN$56 style: column absolute, row written out in full.
		r, err := strconv.Atoi(strings.TrimPrefix(rest, "$"))
		if err != nil {
			return 0, 0, false
		}
		return r, c, true
	case rest == "":
		return anchorRow, c, true
	}
	if r, err := strconv.Atoi(rest); err == nil {
		return r, c, true
	}
	return 0, 0, false
}
