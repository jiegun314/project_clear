package mps

import (
	"strings"
	"testing"
)

// A row-absolute reference into the block the export drops has nowhere to go.
// Storing it as a token made resolveRowRefs subtract DropRows from the row
// number, which produced something like "$A$-53" — not a legal reference, so
// Excel reported the workbook as damaged and offered to repair it.
func TestRebaseRefsKeepsAbsoluteRowsAboveTheHeaderLiteral(t *testing.T) {
	stored, _, err := rebaseRefs("(P58>$A$1)", 58)
	if err != nil {
		t.Fatalf("rebaseRefs: %v", err)
	}
	if strings.Contains(stored, absRowToken) {
		t.Errorf("a row above the header was tokenised: %q", stored)
	}
	if want := "(P#o0>$A$1)"; stored != want {
		t.Errorf("rebaseRefs = %q, want %q", stored, want)
	}

	resolved := resolveRowRefs(stored, 58)
	if strings.Contains(resolved, "$A$-") || strings.Contains(resolved, "$A$0") {
		t.Errorf("resolveRowRefs produced a row that cannot exist: %q", resolved)
	}
}

// A row-absolute reference inside the header block still has to travel with the
// sheet: this is the $DN$56 parameter cell the real workbooks use.
func TestRebaseRefsStillTokenisesHeaderRows(t *testing.T) {
	stored, _, err := rebaseRefs("(ROUND(P58,0)>=P59*(1+$DN$56))", 58)
	if err != nil {
		t.Fatalf("rebaseRefs: %v", err)
	}
	if want := "(ROUND(P#o0,0)>=P#o1*(1+$DN$#a56))"; stored != want {
		t.Errorf("rebaseRefs = %q, want %q", stored, want)
	}
	if got, want := resolveRowRefs(stored, 1000), "(ROUND(P1000,0)>=P1001*(1+$DN$2))"; got != want {
		t.Errorf("resolveRowRefs = %q, want %q", got, want)
	}
}

// The text inside a formula string is data. An absolute row written there is not
// a reference, so rebasing must copy it through untouched.
func TestRebaseRefsLeavesStringLiteralsAlone(t *testing.T) {
	stored, _, err := rebaseRefs(`IF(P58="x$DN$56",P59,0)`, 58)
	if err != nil {
		t.Fatalf("rebaseRefs: %v", err)
	}
	if want := `IF(P#o0="x$DN$56",P#o1,0)`; stored != want {
		t.Errorf("rebaseRefs = %q, want %q", stored, want)
	}
}

// A stored token that appears inside a string literal is text, not a row.
func TestResolveRowRefsLeavesStringLiteralsAlone(t *testing.T) {
	stored := `IF(P#o0="a#o9b",1,0)`
	if got, want := resolveRowRefs(stored, 100), `IF(P100="a#o9b",1,0)`; got != want {
		t.Errorf("resolveRowRefs = %q, want %q", got, want)
	}
}

// shiftLiteralRows migrates programs stored before the token scheme existed. It
// has to shift a real absolute row while leaving one inside a string alone.
func TestShiftLiteralRowsLeavesStringLiteralsAlone(t *testing.T) {
	formula := `IF(P58="$DN$56",$DN$56,0)`
	if got, want := shiftLiteralRows(formula), `IF(P58="$DN$56",$DN$2,0)`; got != want {
		t.Errorf("shiftLiteralRows = %q, want %q", got, want)
	}
}

// A formula with no absolute reference is handed back untouched.
func TestShiftLiteralRowsIgnoresFormulasWithoutAbsoluteRows(t *testing.T) {
	formula := "(ROUND(P180,0)<P181)"
	if got := shiftLiteralRows(formula); got != formula {
		t.Errorf("shiftLiteralRows = %q, want %q", got, formula)
	}
}

// CloneCFRow promises a deep copy. Copying the rule structs alone shares the
// Formulas and Offsets backing arrays, so a change made through the clone
// reached the original and corrupted the stored program.
func TestCloneCFRowIsADeepCopy(t *testing.T) {
	orig := CFRow{Blocks: []CFBlock{{
		C1: 16, C2: 20,
		Rules: []CFRule{{Formulas: []string{"a", "b"}, Offsets: []int{0, 1}}},
	}}}

	clone := CloneCFRow(orig)
	clone.Blocks[0].Rules[0].Formulas[0] = "MUTATED"
	clone.Blocks[0].Rules[0].Offsets[0] = 99

	if got := orig.Blocks[0].Rules[0].Formulas[0]; got != "a" {
		t.Errorf("Formulas was shared with the clone: original[0] = %q", got)
	}
	if got := orig.Blocks[0].Rules[0].Offsets[0]; got != 0 {
		t.Errorf("Offsets was shared with the clone: original[0] = %d", got)
	}
	// The clone is otherwise an equal but independent program.
	if clone.Blocks[0].C1 != 16 || clone.Blocks[0].Rules[0].Formulas[1] != "b" {
		t.Errorf("clone does not mirror the original: %+v", clone)
	}
}
