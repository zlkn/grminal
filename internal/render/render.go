// Package render is the isolated Ebitengine renderer. It draws a Grid into a
// GPU texture using block/run tokenization to preserve ligatures and colors.
//
// The pure tokenization logic (SplitRuns) lives here and is unit-tested without
// a GPU; the actual Ebiten draw calls are kept thin on top of it.
package render

import (
	"strings"

	"github.com/yzolkin/go-vte/internal/vte"
)

// Run is a maximal horizontal block of cells sharing one style. Text is drawn
// as a single unit so OpenType shaping (ligatures like != and ->) is preserved.
type Run struct {
	Col   int       // starting column of the run within the row
	Text  string    // the run's runes
	Style vte.Style // shared style of every cell in the run
}

// SplitRuns tokenizes a cell row into style-contiguous runs, ready for drawing.
//
// Leading and trailing default-style blanks are trimmed (nothing to draw);
// interior blanks are kept because they share the surrounding style, so a whole
// segment shapes as one unit. Runs break wherever the style changes. A row with
// nothing to draw returns nil.
func SplitRuns(row []vte.Cell) []Run {
	start := 0
	for start < len(row) && row[start].IsBlank() {
		start++
	}
	end := len(row)
	for end > start && row[end-1].IsBlank() {
		end--
	}
	if start >= end {
		return nil
	}

	var runs []Run
	runStart := start
	for i := start + 1; i <= end; i++ {
		if i == end || row[i].Style != row[runStart].Style {
			runs = append(runs, makeRun(row, runStart, i))
			runStart = i
		}
	}
	return runs
}

// makeRun builds a Run from the cells in row[lo:hi].
func makeRun(row []vte.Cell, lo, hi int) Run {
	var b strings.Builder
	b.Grow(hi - lo)
	for i := lo; i < hi; i++ {
		b.WriteRune(row[i].Rune)
	}
	return Run{Col: lo, Text: b.String(), Style: row[lo].Style}
}
