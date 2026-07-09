package scrollback

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
)

// line builds a slice of cells from a string, for terse test fixtures.
func line(s string) []vte.Cell {
	cells := make([]vte.Cell, len(s))
	for i, r := range []rune(s) {
		cells[i] = vte.Cell{Rune: r}
	}
	return cells
}

// text renders a ring line back to a string for comparison.
func text(cells []vte.Cell) string {
	rs := make([]rune, len(cells))
	for i, c := range cells {
		rs[i] = c.Rune
	}
	return string(rs)
}

func TestPushBelowCap(t *testing.T) {
	r := NewRing(5, 8)
	r.Push(line("a"))
	r.Push(line("b"))
	r.Push(line("c"))

	if r.Len() != 3 {
		t.Fatalf("Len = %d, want 3", r.Len())
	}
	if r.Cap() != 5 {
		t.Fatalf("Cap = %d, want 5", r.Cap())
	}
	for i, want := range []string{"a", "b", "c"} {
		if got := text(r.At(i)); got != want {
			t.Errorf("At(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestPushExactlyCap(t *testing.T) {
	r := NewRing(3, 8)
	for _, s := range []string{"1", "2", "3"} {
		r.Push(line(s))
	}
	if r.Len() != 3 {
		t.Fatalf("Len = %d, want 3", r.Len())
	}
	for i, want := range []string{"1", "2", "3"} {
		if got := text(r.At(i)); got != want {
			t.Errorf("At(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestPushEvictsOldest(t *testing.T) {
	r := NewRing(3, 8)
	for _, s := range []string{"1", "2", "3", "4", "5"} {
		r.Push(line(s))
	}
	// Capacity is 3, so only the last three survive, oldest→newest.
	if r.Len() != 3 {
		t.Fatalf("Len = %d, want 3", r.Len())
	}
	for i, want := range []string{"3", "4", "5"} {
		if got := text(r.At(i)); got != want {
			t.Errorf("At(%d) = %q, want %q", i, got, want)
		}
	}
}

func TestAtOutOfRange(t *testing.T) {
	r := NewRing(4, 8)
	r.Push(line("x"))
	for _, i := range []int{-1, 1, 99} {
		if got := r.At(i); got != nil {
			t.Errorf("At(%d) = %v, want nil", i, got)
		}
	}
}

// TestPushZeroAlloc enforces the headline design goal: steady-state Push
// (buffer full, lines no wider than the pre-allocated width) allocates nothing.
func TestPushZeroAlloc(t *testing.T) {
	r := NewRing(64, 80)
	src := line("the quick brown fox jumps over the lazy dog") // <= width 80
	// Fill to capacity so every subsequent Push reuses an evicted backing slice.
	for i := 0; i < r.Cap(); i++ {
		r.Push(src)
	}

	allocs := testing.AllocsPerRun(1000, func() {
		r.Push(src)
	})
	if allocs != 0 {
		t.Errorf("Push allocated %.0f times per run, want 0", allocs)
	}
}
