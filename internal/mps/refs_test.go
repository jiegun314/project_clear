package mps

import "testing"

// Cell references are parsed by several helpers for different purposes:
// ColumnName/ColumnNumber are the canonical pair, columnOf and cellToCoords read
// the sheet, parseRangeRef splits a range, and scanRef rewrites the formulas
// inside conditional-format rules. They overlap on the letters-to-number
// mapping, and a divergence there is silent — an export would simply write a
// rule against the wrong column.
//
// This pins the agreement, so a later unification of those helpers cannot change
// the mapping by accident.
func TestReferenceHelpersAgreeOnTheColumnMapping(t *testing.T) {
	names := []string{"A", "B", "O", "P", "Z", "AA", "AB", "AI", "BA", "DN", "ZZ", "AAA"}
	for _, name := range names {
		col, err := ColumnNumber(name)
		if err != nil {
			t.Fatalf("ColumnNumber(%q): %v", name, err)
		}

		// columnOf reads the letters the same way.
		got, err := columnOf(name)
		if err != nil {
			t.Fatalf("columnOf(%q): %v", name, err)
		}
		if got != col {
			t.Errorf("columnOf(%q) = %d, ColumnNumber = %d", name, got, col)
		}

		// cellToCoords splits the same reference into both halves.
		c, r, err := cellToCoords(name + "12")
		if err != nil {
			t.Fatalf("cellToCoords(%q12): %v", name, err)
		}
		if c != col || r != 12 {
			t.Errorf("cellToCoords(%s12) = (%d,%d), want (%d,12)", name, c, r, col)
		}

		// And the name comes back from the number.
		back, err := ColumnName(col)
		if err != nil {
			t.Fatalf("ColumnName(%d): %v", col, err)
		}
		if back != name {
			t.Errorf("ColumnName(%d) = %q, want %q", col, back, name)
		}

		// CellRef renders the same reference again.
		ref := CellRef(col, 12)
		if ref != name+"12" {
			t.Errorf("CellRef(%d,12) = %q, want %q", col, ref, name+"12")
		}

		// scanRef, which walks formulas, sees the same column and row.
		runes := []rune(name + "12")
		tok, next, ok := scanRef(runes, 0)
		if !ok || next != len(runes) {
			t.Fatalf("scanRef(%s12) = ok %v, consumed %d of %d", name, ok, next, len(runes))
		}
		if tok.col != name || tok.row != 12 {
			t.Errorf("scanRef(%s12) = col %q row %d", name, tok.col, tok.row)
		}
	}
}

// The range splitter is the same mapping applied to both ends.
func TestParseRangeRefAgreesWithTheColumnMapping(t *testing.T) {
	c1, r1, c2, r2, err := parseRangeRef("P57:DN435")
	if err != nil {
		t.Fatalf("parseRangeRef: %v", err)
	}
	wantC1, _ := ColumnNumber("P")
	wantC2, _ := ColumnNumber("DN")
	if c1 != wantC1 || r1 != 57 || c2 != wantC2 || r2 != 435 {
		t.Errorf("parseRangeRef = (%d,%d,%d,%d), want (%d,57,%d,435)", c1, r1, c2, r2, wantC1, wantC2)
	}

	// A single cell is a range whose ends coincide.
	c1, r1, c2, r2, err = parseRangeRef("A1")
	if err != nil {
		t.Fatalf("parseRangeRef single: %v", err)
	}
	if c1 != 1 || r1 != 1 || c2 != 1 || r2 != 1 {
		t.Errorf("parseRangeRef(A1) = (%d,%d,%d,%d), want (1,1,1,1)", c1, r1, c2, r2)
	}
}

// A lower-case reference is still a reference: Excel writes upper case, but the
// readers must not drop a cell that arrives otherwise.
func TestReferenceHelpersAcceptLowerCase(t *testing.T) {
	col, err := ColumnNumber("dn")
	if err != nil {
		// ColumnNumber is the canonical helper; if it rejects lower case the
		// others still have to agree with each other.
		t.Logf("ColumnNumber rejects lower case (%v); checking the readers only", err)
	}
	if upper, err2 := ColumnNumber("DN"); err2 == nil && col != upper {
		t.Errorf("ColumnNumber(dn) = %d, ColumnNumber(DN) = %d", col, upper)
	}

	got, err := columnOf("dn")
	if err != nil {
		t.Fatalf("columnOf(dn): %v", err)
	}
	want, _ := ColumnNumber("DN")
	if got != want {
		t.Errorf("columnOf(dn) = %d, want %d", got, want)
	}

	c, r, err := cellToCoords("dn435")
	if err != nil {
		t.Fatalf("cellToCoords(dn435): %v", err)
	}
	if c != want || r != 435 {
		t.Errorf("cellToCoords(dn435) = (%d,%d), want (%d,435)", c, r, want)
	}
}
