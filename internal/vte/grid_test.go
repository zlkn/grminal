package vte

import "testing"

func TestNewGrid(t *testing.T) {
	g := NewGrid(80, 24)

	if g.Cols() != 80 || g.Rows() != 24 {
		t.Fatalf("dims = %dx%d, want 80x24", g.Cols(), g.Rows())
	}
	// A fresh grid is entirely blank spaces with the cursor at the origin.
	for y := 0; y < g.Rows(); y++ {
		for x := 0; x < g.Cols(); x++ {
			if got := g.CellAt(x, y).Rune; got != ' ' {
				t.Fatalf("CellAt(%d,%d).Rune = %q, want space", x, y, got)
			}
		}
	}
	if cx, cy := g.Cursor(); cx != 0 || cy != 0 {
		t.Fatalf("cursor = (%d,%d), want (0,0)", cx, cy)
	}
}

func TestSetCell(t *testing.T) {
	g := NewGrid(10, 3)
	g.SetCell(2, 1, Cell{Rune: 'X'})

	if got := g.CellAt(2, 1).Rune; got != 'X' {
		t.Fatalf("CellAt(2,1) = %q, want 'X'", got)
	}
	// Neighbours untouched.
	if got := g.CellAt(3, 1).Rune; got != ' ' {
		t.Fatalf("neighbour changed: %q", got)
	}
}

func TestSetCellOutOfBounds(t *testing.T) {
	g := NewGrid(4, 2)
	// Writes outside the grid are silently ignored, not panics.
	g.SetCell(-1, 0, Cell{Rune: 'A'})
	g.SetCell(4, 0, Cell{Rune: 'B'})
	g.SetCell(0, 2, Cell{Rune: 'C'})
	if got := g.CellAt(0, 0).Rune; got != ' ' {
		t.Fatalf("out-of-bounds write leaked: %q", got)
	}
}

func TestMoveCursorClamps(t *testing.T) {
	g := NewGrid(10, 5)
	tests := []struct {
		inX, inY   int
		wantX, wantY int
	}{
		{3, 2, 3, 2},
		{-5, -5, 0, 0},
		{100, 100, 9, 4},
		{9, 4, 9, 4},
	}
	for _, tc := range tests {
		g.MoveCursor(tc.inX, tc.inY)
		if cx, cy := g.Cursor(); cx != tc.wantX || cy != tc.wantY {
			t.Errorf("MoveCursor(%d,%d) = (%d,%d), want (%d,%d)",
				tc.inX, tc.inY, cx, cy, tc.wantX, tc.wantY)
		}
	}
}

func TestClear(t *testing.T) {
	g := NewGrid(3, 2)
	g.SetCell(0, 0, Cell{Rune: 'a'})
	g.SetCell(2, 1, Cell{Rune: 'b'})
	g.MoveCursor(2, 1)

	g.Clear()

	for y := 0; y < g.Rows(); y++ {
		for x := 0; x < g.Cols(); x++ {
			if got := g.CellAt(x, y).Rune; got != ' ' {
				t.Fatalf("CellAt(%d,%d) = %q after Clear, want space", x, y, got)
			}
		}
	}
	if cx, cy := g.Cursor(); cx != 0 || cy != 0 {
		t.Fatalf("cursor = (%d,%d) after Clear, want (0,0)", cx, cy)
	}
}

func TestResizePreservesContent(t *testing.T) {
	g := NewGrid(4, 2)
	g.SetCell(1, 0, Cell{Rune: 'p'})
	g.SetCell(3, 1, Cell{Rune: 'q'})

	g.Resize(6, 3)
	if g.Cols() != 6 || g.Rows() != 3 {
		t.Fatalf("dims after resize = %dx%d, want 6x3", g.Cols(), g.Rows())
	}
	if got := g.CellAt(1, 0).Rune; got != 'p' {
		t.Errorf("content lost on grow: (1,0) = %q, want 'p'", got)
	}
	if got := g.CellAt(3, 1).Rune; got != 'q' {
		t.Errorf("content lost on grow: (3,1) = %q, want 'q'", got)
	}
	// New area is blank.
	if got := g.CellAt(5, 2).Rune; got != ' ' {
		t.Errorf("new cell not blank: (5,2) = %q", got)
	}

	// Shrinking keeps content by re-wrapping rather than clipping: the surviving
	// cells stay reachable and the cursor is clamped into bounds.
	g.Resize(3, 3)
	if g.Cols() != 3 || g.Rows() != 3 {
		t.Fatalf("dims after shrink = %dx%d, want 3x3", g.Cols(), g.Rows())
	}
	if got := g.CellAt(1, 0).Rune; got != 'p' {
		t.Errorf("surviving cell lost on shrink: (1,0) = %q, want 'p'", got)
	}
	if cx, cy := g.Cursor(); cx < 0 || cx >= 3 || cy < 0 || cy >= 3 {
		t.Errorf("cursor out of bounds after shrink: (%d,%d)", cx, cy)
	}
}

func TestDump(t *testing.T) {
	g := NewGrid(3, 2)
	g.SetCell(0, 0, Cell{Rune: 'h'})
	g.SetCell(1, 0, Cell{Rune: 'i'})

	want := "hi \n   \n"
	if got := g.Dump(); got != want {
		t.Errorf("Dump() = %q, want %q", got, want)
	}
}
