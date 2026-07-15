package vte

import "testing"

// These pin background-color erase (bce): cells cleared by EL/ED/ECH/scroll take
// the current SGR background, so a highlighted row (e.g. an htop selection) fills
// to the edge instead of stopping at the text.

var blueBG = Color{Kind: ColorIndexed, Idx: 4}

// EL (erase to end of line) after setting a background fills the tail with it.
func TestBCEEraseLineCarriesBackground(t *testing.T) {
	g := feed(6, 1, "\x1b[44mab\x1b[K") // blue bg, "ab", erase to EOL
	for x := 2; x < 6; x++ {
		c := g.CellAt(x, 0)
		if c.Rune != ' ' || c.Style.BG != blueBG {
			t.Errorf("cell %d = %+v, want space with blue bg", x, c)
		}
	}
}

// ED mode 2 (clear screen) fills every cell with the current background.
func TestBCEEraseDisplayCarriesBackground(t *testing.T) {
	g := feed(4, 2, "\x1b[44m\x1b[2J")
	for y := 0; y < 2; y++ {
		for x := 0; x < 4; x++ {
			if g.CellAt(x, y).Style.BG != blueBG {
				t.Fatalf("cell (%d,%d) bg not blue after ED2", x, y)
			}
		}
	}
}

// Resetting SGR (or 49) returns erase to the default background.
func TestBCEResetBySGRDefault(t *testing.T) {
	g := feed(6, 1, "\x1b[44m\x1b[0m\x1b[K") // blue, reset, erase
	for x := 0; x < 6; x++ {
		if g.CellAt(x, 0).Style.BG != (Color{}) {
			t.Errorf("cell %d bg = %+v, want default after SGR reset", x, g.CellAt(x, 0).Style.BG)
		}
	}
}

// RIS clears the bce background so a later erase is default-colored.
func TestBCEResetByRIS(t *testing.T) {
	g := feed(6, 1, "\x1b[44m\x1bc\x1b[K")
	if g.CellAt(0, 0).Style.BG != (Color{}) {
		t.Errorf("bce background survived RIS: %+v", g.CellAt(0, 0).Style.BG)
	}
}
