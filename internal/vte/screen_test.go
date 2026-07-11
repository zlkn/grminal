package vte

import "testing"

func TestAltScreenIsolatesAndRestores(t *testing.T) {
	g := NewGrid(5, 2)
	p := NewParser(g)

	p.Write([]byte("AB")) // primary: row0 "AB", cursor (2,0)

	p.Write([]byte("\x1b[?1049h")) // save cursor + switch to (cleared) alt screen
	if got := row(g, 0); got != "     " {
		t.Fatalf("alt screen not cleared: row0 = %q", got)
	}
	p.Write([]byte("\x1b[HXY")) // home, write in alt
	if got := row(g, 0); got != "XY   " {
		t.Fatalf("alt write: row0 = %q", got)
	}

	p.Write([]byte("\x1b[?1049l")) // exit alt: restore primary + cursor
	if got := row(g, 0); got != "AB   " {
		t.Errorf("primary not restored: row0 = %q, want %q", got, "AB   ")
	}
	if cx, cy := g.Cursor(); cx != 2 || cy != 0 {
		t.Errorf("cursor after restore = (%d,%d), want (2,0)", cx, cy)
	}
}

func TestSaveRestoreCursorCSI(t *testing.T) {
	g := NewGrid(10, 5)
	p := NewParser(g)

	p.Write([]byte("\x1b[3;4H")) // cursor to row3,col4 -> (3,2)
	p.Write([]byte("\x1b[s"))    // SCOSC save
	p.Write([]byte("\x1b[HX"))   // home + write, cursor moves
	p.Write([]byte("\x1b[u"))    // SCORC restore
	p.Write([]byte("Y"))         // write at restored position

	if got := g.CellAt(3, 2).Rune; got != 'Y' {
		t.Errorf("CellAt(3,2) = %q, want 'Y' (cursor not restored)", got)
	}
}

func TestSaveRestoreCursorDEC(t *testing.T) {
	g := NewGrid(10, 5)
	p := NewParser(g)

	p.Write([]byte("\x1b[2;2H")) // (1,1)
	p.Write([]byte("\x1b7"))     // DECSC
	p.Write([]byte("\x1b[HZ"))   // home + write
	p.Write([]byte("\x1b8"))     // DECRC
	p.Write([]byte("Q"))

	if got := g.CellAt(1, 1).Rune; got != 'Q' {
		t.Errorf("CellAt(1,1) = %q, want 'Q' (DECRC failed)", got)
	}
}

func TestCursorVisibility(t *testing.T) {
	g := NewGrid(3, 1)
	p := NewParser(g)

	if !g.Snapshot().CursorVisible {
		t.Error("cursor should be visible by default")
	}
	p.Write([]byte("\x1b[?25l")) // hide
	if g.Snapshot().CursorVisible {
		t.Error("cursor should be hidden after ?25l")
	}
	p.Write([]byte("\x1b[?25h")) // show
	if !g.Snapshot().CursorVisible {
		t.Error("cursor should be visible after ?25h")
	}
}
