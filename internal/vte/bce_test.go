package vte_test

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
)

func TestBackgroundEraseAppliesCurrentBG(t *testing.T) {
	green := vte.Color{Kind: vte.ColorIndexed, Idx: 2}
	g := feed(4, 2, "\x1b[42m\x1b[2J") // green BG, erase display

	for y := 0; y < g.Rows(); y++ {
		for x := 0; x < g.Cols(); x++ {
			if got := g.CellAt(x, y).Style.BG; got != green {
				t.Errorf("cell (%d,%d) BG = %+v, want green", x, y, got)
			}
		}
	}
}

func TestInsertLinesAppliesCurrentBG(t *testing.T) {
	blue := vte.Color{Kind: vte.ColorIndexed, Idx: 4}
	g := feed(3, 3, "\x1b[44m\x1b[1L") // blue BG, insert line

	for x := 0; x < g.Cols(); x++ {
		if got := g.CellAt(x, 0).Style.BG; got != blue {
			t.Errorf("inserted cell (%d,0) BG = %+v, want blue", x, got)
		}
	}
}
