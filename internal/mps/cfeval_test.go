package mps

import (
	"testing"
)

// lookupFromValues builds a cell lookup from a sparse map, the way the reader
// feeds the evaluator.
func lookupFromValues(values map[[2]int]string) cellLookup {
	return func(row, col int) string { return values[[2]int{row, col}] }
}

// CalcOH 行的四条规则：红(小于 0)、黄(低于 SS 行)、蓝(超过 SS 行*(1+容差))、
// 绿(其余)。这里逐条验证求值结果。
func TestEvalCFColorsPicksTheFirstMatchingRule(t *testing.T) {
	// $DN$56 = 0.1（容差 10%），P69 = SS 行的值。
	values := map[[2]int]string{
		{56, 118}: "0.1",
		{69, 16}:  "1000", // P69
		{69, 17}:  "1000", // Q69
		{69, 18}:  "0",    // R69
		{69, 19}:  "1000", // S69
	}
	lookup := lookupFromValues(values)

	prog := CFRow{Blocks: []CFBlock{{
		C1: 16, C2: 16 + 3,
		Rules: []CFRule{
			{Attrs: ` type="expression" dxfId="254" priority="303" stopIfTrue="1"`, Formulas: []string{"(ROUND(P#o0,0)<0)"}},
			{Attrs: ` type="expression" dxfId="253" priority="304" stopIfTrue="1"`, Formulas: []string{"(ROUND(P#o0,0)<P#o2)"}},
			{Attrs: ` type="expression" dxfId="252" priority="305" stopIfTrue="1"`, Formulas: []string{"(ROUND(P#o0,0)>=P#o2*(1+$DN$56))"}},
			{Attrs: ` type="expression" dxfId="251" priority="306" stopIfTrue="1"`, Formulas: []string{"AND(P#o0>=0,P#o0>=P#o2,P#o0<P#o2*(1+$DN$56))"}},
		},
	}}}
	colors := map[int]string{254: "#FF0000", 253: "#FFFF00", 252: "#00B0F0", 251: "#9BBB59"}

	cases := []struct {
		name string
		row  int
		col  int
		val  string
		want string
	}{
		{"负库存显示红色", 67, 16, "-5", "#FF0000"},
		{"低于 SS 行显示黄色", 67, 16, "900", "#FFFF00"},
		{"高于 SS 行 10% 显示蓝色", 67, 16, "1200", "#00B0F0"},
		{"其余显示绿色", 67, 16, "1000", "#9BBB59"},
	}
	for _, c := range cases {
		values[[2]int{c.row, c.col}] = c.val
		got := EvalCFColors(prog, c.row, colors, lookup)
		if got[c.col] != c.want {
			t.Errorf("%s: %s 得到 %q, want %q", c.name, c.val, got[c.col], c.want)
		}
	}
}

func TestEvalCFColorsHandlesFormulaShapes(t *testing.T) {
	values := map[[2]int]string{
		{100, 16}:  "118", // Ctrl+Z
		{101, 109}: "X",   // $DE 列（INDEX(...,ROW())）
	}
	lookup := lookupFromValues(values)
	colors := map[int]string{1: "#112233", 2: "#445566"}

	// INDEX($DE:$DE,ROW())="X" → 命中
	prog := CFRow{Blocks: []CFBlock{{C1: 16, C2: 16, Rules: []CFRule{
		{Attrs: ` type="expression" dxfId="1" priority="10" stopIfTrue="1"`, Formulas: []string{`INDEX($DE:$DE,ROW())="X"`}},
	}}}}
	if got := EvalCFColors(prog, 101, colors, lookup); got[16] != "#112233" {
		t.Errorf("INDEX/ROW 规则 = %q, want #112233", got[16])
	}

	// 不满足时不染色
	prog = CFRow{Blocks: []CFBlock{{C1: 16, C2: 16, Rules: []CFRule{
		{Attrs: ` type="expression" dxfId="1" priority="10" stopIfTrue="1"`, Formulas: []string{`(ROUND(P#o0,0)>1000)`}},
	}}}}
	if got := EvalCFColors(prog, 100, colors, lookup); len(got) != 0 {
		t.Errorf("不命中的规则不应染色，得到 %v", got)
	}
}

func TestDxfFillColorsReadsBackgroundColour(t *testing.T) {
	colors := DxfFillColors([]string{
		`<dxf><fill><patternFill><bgColor rgb="FFFF0000"/></patternFill></fill></dxf>`,
		`<dxf><font><color rgb="FF000000"/></font></dxf>`,
		`<dxf><fill><patternFill><fgColor rgb="FF00B0F0"/></patternFill></fill></dxf>`,
	})
	if colors[0] != "#FF0000" {
		t.Errorf("bgColor 颜色 = %q, want #FF0000", colors[0])
	}
	if _, ok := colors[1]; ok {
		t.Errorf("只有字体的 dxf 不该给出填充色")
	}
	if colors[2] != "#00B0F0" {
		t.Errorf("fgColor 回退 = %q, want #00B0F0", colors[2])
	}
}
