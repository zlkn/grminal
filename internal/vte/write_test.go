package vte

import "testing"

// row returns the runes of grid row y as a string, for terse assertions.
func row(g *Grid, y int) string {
	rs := make([]rune, g.Cols())
	for x := 0; x < g.Cols(); x++ {
		rs[x] = g.CellAt(x, y).Rune
	}
	return string(rs)
}

func TestWritePrintable(t *testing.T) {
	g := NewGrid(5, 2)
	g.Write([]byte("hi"))

	if got := row(g, 0); got != "hi   " {
		t.Errorf("row0 = %q, want %q", got, "hi   ")
	}
	if cx, cy := g.Cursor(); cx != 2 || cy != 0 {
		t.Errorf("cursor = (%d,%d), want (2,0)", cx, cy)
	}
}

func TestWriteNewlineReturn(t *testing.T) {
	g := NewGrid(5, 3)
	// \n moves down (column preserved); \r returns to column 0.
	g.Write([]byte("ab\r\nc"))

	if got := row(g, 0); got != "ab   " {
		t.Errorf("row0 = %q, want %q", got, "ab   ")
	}
	if got := row(g, 1); got != "c    " {
		t.Errorf("row1 = %q, want %q", got, "c    ")
	}
	if cx, cy := g.Cursor(); cx != 1 || cy != 1 {
		t.Errorf("cursor = (%d,%d), want (1,1)", cx, cy)
	}
}

func TestWriteWrap(t *testing.T) {
	g := NewGrid(3, 2)
	// Four printables into a 3-wide grid: the 4th wraps to the next row.
	g.Write([]byte("abcd"))

	if got := row(g, 0); got != "abc" {
		t.Errorf("row0 = %q, want %q", got, "abc")
	}
	if got := row(g, 1); got != "d  " {
		t.Errorf("row1 = %q, want %q", got, "d  ")
	}
	if cx, cy := g.Cursor(); cx != 1 || cy != 1 {
		t.Errorf("cursor = (%d,%d), want (1,1)", cx, cy)
	}
}

func TestWriteScroll(t *testing.T) {
	g := NewGrid(3, 2)
	// Third logical line forces a scroll: the top row ("a") is lost.
	g.Write([]byte("a\r\nb\r\nc"))

	if got := row(g, 0); got != "b  " {
		t.Errorf("row0 = %q, want %q (expected scroll up)", got, "b  ")
	}
	if got := row(g, 1); got != "c  " {
		t.Errorf("row1 = %q, want %q", got, "c  ")
	}
	if cx, cy := g.Cursor(); cx != 1 || cy != 1 {
		t.Errorf("cursor = (%d,%d), want (1,1)", cx, cy)
	}
}

func TestWriteUTF8(t *testing.T) {
	g := NewGrid(4, 1)
	// Multi-byte runes decode to single cells (within one Write call).
	g.Write([]byte("héy"))

	if got := row(g, 0); got != "héy " {
		t.Errorf("row0 = %q, want %q", got, "héy ")
	}
	if cx, _ := g.Cursor(); cx != 3 {
		t.Errorf("cursorX = %d, want 3", cx)
	}
}
