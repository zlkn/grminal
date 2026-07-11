package vte

import "testing"

func TestScrollRegionScrollsOnlyRegion(t *testing.T) {
	g := NewGrid(3, 3)
	p := NewParser(g)
	p.Write([]byte("a\r\nb\r\nc")) // rows: a / b / c

	p.Write([]byte("\x1b[2;3r")) // scroll region = rows 2..3 (0-based 1..2)
	p.Write([]byte("\x1b[3;1H")) // cursor to bottom of region (row 3, col 1)
	p.Write([]byte("\n"))        // LF at region bottom -> scroll region up

	if row(g, 0) != "a  " {
		t.Errorf("row0 = %q, want %q (outside region, untouched)", row(g, 0), "a  ")
	}
	if row(g, 1) != "c  " {
		t.Errorf("row1 = %q, want %q (region scrolled up)", row(g, 1), "c  ")
	}
	if row(g, 2) != "   " {
		t.Errorf("row2 = %q, want blank", row(g, 2))
	}
}

func TestReverseIndex(t *testing.T) {
	g := NewGrid(3, 3)
	p := NewParser(g)
	p.Write([]byte("a\r\nb\r\nc"))
	p.Write([]byte("\x1b[H")) // home (top of default region)
	p.Write([]byte("\x1bM"))  // RI at top -> scroll region down

	if row(g, 0) != "   " || row(g, 1) != "a  " || row(g, 2) != "b  " {
		t.Errorf("RI rows = %q/%q/%q, want blank/a/b", row(g, 0), row(g, 1), row(g, 2))
	}
}

func TestInsertLines(t *testing.T) {
	g := NewGrid(3, 3)
	p := NewParser(g)
	p.Write([]byte("a\r\nb\r\nc"))
	p.Write([]byte("\x1b[H"))  // home
	p.Write([]byte("\x1b[L")) // insert 1 line

	if row(g, 0) != "   " || row(g, 1) != "a  " || row(g, 2) != "b  " {
		t.Errorf("IL rows = %q/%q/%q, want blank/a/b", row(g, 0), row(g, 1), row(g, 2))
	}
}

func TestDeleteLines(t *testing.T) {
	g := NewGrid(3, 3)
	p := NewParser(g)
	p.Write([]byte("a\r\nb\r\nc"))
	p.Write([]byte("\x1b[H"))  // home
	p.Write([]byte("\x1b[M")) // delete 1 line

	if row(g, 0) != "b  " || row(g, 1) != "c  " || row(g, 2) != "   " {
		t.Errorf("DL rows = %q/%q/%q, want b/c/blank", row(g, 0), row(g, 1), row(g, 2))
	}
}

func TestInsertChars(t *testing.T) {
	g := NewGrid(5, 1)
	p := NewParser(g)
	p.Write([]byte("abcd")) // "abcd " (no wrap on a 5-wide row)
	p.Write([]byte("\x1b[H"))
	p.Write([]byte("\x1b[@")) // insert 1 blank at cursor

	if row(g, 0) != " abcd" {
		t.Errorf("ICH row = %q, want %q", row(g, 0), " abcd")
	}
}

func TestDeleteChars(t *testing.T) {
	g := NewGrid(5, 1)
	p := NewParser(g)
	p.Write([]byte("abcd"))
	p.Write([]byte("\x1b[H"))
	p.Write([]byte("\x1b[P")) // delete 1 char at cursor

	if row(g, 0) != "bcd  " {
		t.Errorf("DCH row = %q, want %q", row(g, 0), "bcd  ")
	}
}

func TestEraseChars(t *testing.T) {
	g := NewGrid(5, 1)
	p := NewParser(g)
	p.Write([]byte("abcd"))
	p.Write([]byte("\x1b[H"))
	p.Write([]byte("\x1b[2X")) // erase 2 chars at cursor (no shift)

	if row(g, 0) != "  cd " {
		t.Errorf("ECH row = %q, want %q", row(g, 0), "  cd ")
	}
	if cx, _ := g.Cursor(); cx != 0 {
		t.Errorf("ECH moved cursor to %d, want 0", cx)
	}
}
