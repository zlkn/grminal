package render

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
)

// rowFrom builds a cell row (default style) from a string.
func rowFrom(s string) []vte.Cell {
	rs := []rune(s)
	cells := make([]vte.Cell, len(rs))
	for i, r := range rs {
		cells[i] = vte.Cell{Rune: r}
	}
	return cells
}

// seg is a run of text sharing one style, for building mixed-style rows.
type seg struct {
	text  string
	style vte.Style
}

// styledRow concatenates segments and pads to width with default blanks.
func styledRow(width int, segs ...seg) []vte.Cell {
	row := make([]vte.Cell, 0, width)
	for _, s := range segs {
		for _, r := range s.text {
			row = append(row, vte.Cell{Rune: r, Style: s.style})
		}
	}
	for len(row) < width {
		row = append(row, vte.Cell{Rune: ' '})
	}
	return row
}

var (
	redFG   = vte.Style{FG: vte.Color{Kind: vte.ColorIndexed, Idx: 1}}
	greenFG = vte.Style{FG: vte.Color{Kind: vte.ColorIndexed, Idx: 2}}
	boldS   = vte.Style{Attrs: vte.AttrBold}
	redBG   = vte.Style{BG: vte.Color{Kind: vte.ColorIndexed, Idx: 1}}
)

func TestSplitRunsEmptyRow(t *testing.T) {
	// A row of default-style blanks has nothing to draw.
	if got := SplitRuns(rowFrom("     ")); got != nil {
		t.Errorf("SplitRuns(blanks) = %v, want nil", got)
	}
	if got := SplitRuns(rowFrom("")); got != nil {
		t.Errorf("SplitRuns(empty) = %v, want nil", got)
	}
}

func TestSplitRunsSingle(t *testing.T) {
	runs := SplitRuns(rowFrom("hello     "))
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1: %+v", len(runs), runs)
	}
	if runs[0].Col != 0 || runs[0].Text != "hello" {
		t.Errorf("run = {Col:%d Text:%q}, want {0 %q}", runs[0].Col, runs[0].Text, "hello")
	}
}

func TestSplitRunsLeadingBlanksTrimmed(t *testing.T) {
	// Leading default-style blanks are skipped; Col reflects the true column.
	runs := SplitRuns(rowFrom("  hi   "))
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1: %+v", len(runs), runs)
	}
	if runs[0].Col != 2 || runs[0].Text != "hi" {
		t.Errorf("run = {Col:%d Text:%q}, want {2 %q}", runs[0].Col, runs[0].Text, "hi")
	}
}

func TestSplitRunsInteriorSpacesKept(t *testing.T) {
	// Interior spaces share the surrounding style, so they stay in one run
	// (preserves shaping/ligatures across the whole segment).
	runs := SplitRuns(rowFrom("a b c    "))
	if len(runs) != 1 {
		t.Fatalf("got %d runs, want 1: %+v", len(runs), runs)
	}
	if runs[0].Col != 0 || runs[0].Text != "a b c" {
		t.Errorf("run = {Col:%d Text:%q}, want {0 %q}", runs[0].Col, runs[0].Text, "a b c")
	}
}

func TestSplitRunsFullWidth(t *testing.T) {
	runs := SplitRuns(rowFrom("abc"))
	if len(runs) != 1 || runs[0].Text != "abc" || runs[0].Col != 0 {
		t.Fatalf("got %+v, want single run {0 \"abc\"}", runs)
	}
}

func TestSplitRunsColorBoundary(t *testing.T) {
	runs := SplitRuns(styledRow(10, seg{"ab", redFG}, seg{"cd", greenFG}))
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2: %+v", len(runs), runs)
	}
	if runs[0].Col != 0 || runs[0].Text != "ab" || runs[0].Style != redFG {
		t.Errorf("run0 = %+v, want {0 ab red}", runs[0])
	}
	if runs[1].Col != 2 || runs[1].Text != "cd" || runs[1].Style != greenFG {
		t.Errorf("run1 = %+v, want {2 cd green}", runs[1])
	}
}

func TestSplitRunsAttrBoundary(t *testing.T) {
	// Same color, differing only by the bold attribute, still splits.
	runs := SplitRuns(styledRow(10, seg{"hi", boldS}, seg{"yo", vte.Style{}}))
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2: %+v", len(runs), runs)
	}
	if runs[0].Style != boldS || runs[1].Style != (vte.Style{}) {
		t.Errorf("styles = %+v / %+v, want bold / default", runs[0].Style, runs[1].Style)
	}
}

func TestSplitRunsNonDefaultBlanksKept(t *testing.T) {
	// Trailing spaces with a non-default background are drawable, so they must
	// survive trimming and form their own run.
	runs := SplitRuns(styledRow(6, seg{"ab", vte.Style{}}, seg{"  ", redBG}))
	if len(runs) != 2 {
		t.Fatalf("got %d runs, want 2: %+v", len(runs), runs)
	}
	if runs[1].Col != 2 || runs[1].Text != "  " || runs[1].Style != redBG {
		t.Errorf("run1 = %+v, want {2 \"  \" redBG}", runs[1])
	}
}
