package vte

import "testing"

// These pin DECOM (origin mode, ?6): CUP/VPA row addressing becomes relative to
// the scroll region top, and the cursor is confined to the region.

// With origin mode set, CUP row 1 lands on the region's top margin, not row 0.
func TestOriginModeCUPRelativeToRegion(t *testing.T) {
	// Region rows 3..6 (1-based) => scrollTop=2. Enable DECOM, then home via CUP.
	g := feed(10, 8, "\x1b[3;6r\x1b[?6hX")
	// After DECSTBM+DECOM the cursor homes to (0, scrollTop); 'X' lands there.
	if row(g, 2) != "X         " {
		t.Errorf("row2 = %q, want 'X' at region top", row(g, 2))
	}
	if row(g, 0) != "          " {
		t.Errorf("row0 = %q, want blank (row 0 is outside the region origin)", row(g, 0))
	}
}

// CUP past the region bottom is clamped to the region, not the screen.
func TestOriginModeConfinesToRegion(t *testing.T) {
	g := feed(10, 8, "\x1b[3;6r\x1b[?6h\x1b[100;1HX") // request row 100
	if row(g, 5) != "X         " { // scrollBot = 5
		t.Errorf("cursor not confined: want 'X' on row5, got rows: %q / %q", row(g, 5), row(g, 6))
	}
}

// Without origin mode (the default), CUP is absolute.
func TestOriginModeOffIsAbsolute(t *testing.T) {
	g := feed(10, 8, "\x1b[3;6r\x1b[1;1HX")
	if row(g, 0) != "X         " {
		t.Errorf("row0 = %q, want 'X' (absolute addressing)", row(g, 0))
	}
}

// RIS resets origin mode back to off.
func TestOriginModeResetByRIS(t *testing.T) {
	g := feed(10, 8, "\x1b[?6h")
	if !g.originMode {
		t.Fatal("?6h did not set origin mode")
	}
	NewParser(g).Write([]byte("\x1bc"))
	if g.originMode {
		t.Error("RIS did not reset origin mode")
	}
}
