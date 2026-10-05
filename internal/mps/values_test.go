package mps

import "testing"

// Excel writes references in upper case, but a lower-case one is still a valid
// reference. Rejecting it made the streaming reader drop the cell silently:
// inCell was set to false and the value never reached the grid.
func TestColumnOfAcceptsLowerCaseReferences(t *testing.T) {
	cases := map[string]int{
		"A":   1,
		"P":   16,
		"p":   16,
		"ai":  35,
		"AI":  35,
		"$dn": 118,
		"a":   1,
	}
	for ref, want := range cases {
		got, err := columnOf(ref)
		if err != nil {
			t.Errorf("columnOf(%q) returned an error: %v", ref, err)
			continue
		}
		if got != want {
			t.Errorf("columnOf(%q) = %d, want %d", ref, got, want)
		}
	}
}

func TestColumnOfStillRejectsNonReferences(t *testing.T) {
	for _, ref := range []string{"", "   ", "$", "58", "1A"} {
		if got, err := columnOf(ref); err == nil {
			t.Errorf("columnOf(%q) = %d, want an error", ref, got)
		}
	}
}

func TestCellToCoordsAcceptsLowerCaseReferences(t *testing.T) {
	col, row, err := cellToCoords("p58")
	if err != nil {
		t.Fatalf("cellToCoords(p58): %v", err)
	}
	if col != 16 || row != 58 {
		t.Errorf("cellToCoords(p58) = (%d,%d), want (16,58)", col, row)
	}
}

func TestCellToCoordsStillRejectsInvalidReferences(t *testing.T) {
	for _, cell := range []string{"", "$", "58", "A0"} {
		if _, _, err := cellToCoords(cell); err == nil {
			t.Errorf("cellToCoords(%q) succeeded, want an error", cell)
		}
	}
}
