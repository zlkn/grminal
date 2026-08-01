package vte_test

import (
	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

// feed returns a cols×rows Grid after writing seq through a fresh parser.
func feed(cols, rows int, seq string) *vte.Grid {
	g := vte.NewGrid(cols, rows)
	p := parser.NewParser(g)
	p.Write([]byte(seq))
	return g
}

// row returns the runes of grid row y as a string.
func row(g *vte.Grid, y int) string {
	rs := make([]rune, g.Cols())
	for x := 0; x < g.Cols(); x++ {
		rs[x] = g.CellAt(x, y).Rune
	}
	return string(rs)
}
