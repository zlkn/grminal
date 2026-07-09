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
