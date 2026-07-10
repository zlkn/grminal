package vte

import (
	"bytes"
	"testing"
)

func TestPrimaryDeviceAttributes(t *testing.T) {
	g := NewGrid(10, 3)
	p := NewParser(g)
	var reply bytes.Buffer
	p.SetReply(&reply)

	p.Write([]byte("\x1b[c")) // DA1 query
	if got := reply.String(); got != "\x1b[?6c" {
		t.Errorf("DA1 reply = %q, want %q", got, "\x1b[?6c")
	}
}

func TestSecondaryDeviceAttributes(t *testing.T) {
	g := NewGrid(10, 3)
	p := NewParser(g)
	var reply bytes.Buffer
	p.SetReply(&reply)

	p.Write([]byte("\x1b[>c")) // DA2 query
	if got := reply.String(); got != "\x1b[>0;10;0c" {
		t.Errorf("DA2 reply = %q, want %q", got, "\x1b[>0;10;0c")
	}
}

func TestDeviceStatusReport(t *testing.T) {
	g := NewGrid(10, 3)
	p := NewParser(g)
	var reply bytes.Buffer
	p.SetReply(&reply)

	p.Write([]byte("\x1b[5n")) // status query
	if got := reply.String(); got != "\x1b[0n" {
		t.Errorf("DSR reply = %q, want %q", got, "\x1b[0n")
	}
}

func TestCursorPositionReport(t *testing.T) {
	g := NewGrid(10, 5)
	p := NewParser(g)
	var reply bytes.Buffer
	p.SetReply(&reply)

	p.Write([]byte("\x1b[2;5H\x1b[6n")) // move to row2,col5 then query position
	if got := reply.String(); got != "\x1b[2;5R" {
		t.Errorf("CPR reply = %q, want %q", got, "\x1b[2;5R")
	}
}

// row returns the runes of grid row y as a string.
func row(g *Grid, y int) string {
	rs := make([]rune, g.Cols())
	for x := 0; x < g.Cols(); x++ {
		rs[x] = g.CellAt(x, y).Rune
	}
	return string(rs)
}

// feed runs data through a fresh parser over a cols×rows grid and returns it.
func feed(cols, rows int, data string) *Grid {
	g := NewGrid(cols, rows)
	p := NewParser(g)
	p.Write([]byte(data))
	return g
}

// --- printable text and C0 controls (migrated from the minimal writer) -------

func TestPrintable(t *testing.T) {
	g := feed(5, 2, "hi")
	if got := row(g, 0); got != "hi   " {
		t.Errorf("row0 = %q, want %q", got, "hi   ")
	}
	if cx, cy := g.Cursor(); cx != 2 || cy != 0 {
		t.Errorf("cursor = (%d,%d), want (2,0)", cx, cy)
	}
}

func TestNewlineReturn(t *testing.T) {
	g := feed(5, 3, "ab\r\nc")
	if row(g, 0) != "ab   " || row(g, 1) != "c    " {
		t.Errorf("rows = %q / %q", row(g, 0), row(g, 1))
	}
}

func TestWrap(t *testing.T) {
	g := feed(3, 2, "abcd")
	if row(g, 0) != "abc" || row(g, 1) != "d  " {
		t.Errorf("rows = %q / %q", row(g, 0), row(g, 1))
	}
}

func TestScroll(t *testing.T) {
	g := feed(3, 2, "a\r\nb\r\nc")
	if row(g, 0) != "b  " || row(g, 1) != "c  " {
		t.Errorf("rows = %q / %q, want scroll", row(g, 0), row(g, 1))
	}
}

func TestUTF8(t *testing.T) {
	g := feed(4, 1, "héy")
	if row(g, 0) != "héy " {
		t.Errorf("row0 = %q, want %q", row(g, 0), "héy ")
	}
}

func TestBackspace(t *testing.T) {
	g := feed(5, 1, "ab\bc")
	// Backspace moves left; 'c' overwrites 'b'.
	if row(g, 0) != "ac   " {
		t.Errorf("row0 = %q, want %q", row(g, 0), "ac   ")
	}
}

// --- state survives across Write calls (chunk boundary) ----------------------

func TestSplitEscapeAcrossWrites(t *testing.T) {
	g := NewGrid(10, 2)
	p := NewParser(g)
	p.Write([]byte("\x1b[2")) // CSI, partial param
	p.Write([]byte(";3HX"))   // finish CUP then print
	if got := g.CellAt(2, 1).Rune; got != 'X' {
		t.Errorf("CellAt(2,1) = %q, want 'X' (escape spanning writes)", got)
	}
}

// --- CSI cursor movement -----------------------------------------------------

func TestCUP(t *testing.T) {
	g := feed(10, 5, "\x1b[2;3HX") // row 2, col 3 -> 0-based (2,1)
	if got := g.CellAt(2, 1).Rune; got != 'X' {
		t.Errorf("CellAt(2,1) = %q, want 'X'", got)
	}
}

func TestCUPDefaultsToOrigin(t *testing.T) {
	g := feed(10, 5, "\x1b[5;5H\x1b[HX")
	if got := g.CellAt(0, 0).Rune; got != 'X' {
		t.Errorf("CellAt(0,0) = %q, want 'X'", got)
	}
}

func TestCursorMoves(t *testing.T) {
	// Home, down 3, right 4, up 1, left 2, then mark.
	g := feed(10, 6, "\x1b[H\x1b[3B\x1b[4C\x1b[1A\x1b[2DX")
	// (0,0) -> down3 (0,3) -> right4 (4,3) -> up1 (4,2) -> left2 (2,2)
	if got := g.CellAt(2, 2).Rune; got != 'X' {
		t.Errorf("CellAt(2,2) = %q, want 'X'", got)
	}
}

func TestCHA(t *testing.T) {
	g := feed(10, 1, "hello\x1b[3GX") // cursor horizontal absolute to col 3
	if got := g.CellAt(2, 0).Rune; got != 'X' {
		t.Errorf("CellAt(2,0) = %q, want 'X'", got)
	}
}

// --- erase -------------------------------------------------------------------

func TestEraseToEndOfLine(t *testing.T) {
	g := feed(8, 1, "hello\x1b[1;3H\x1b[0K")
	if got := row(g, 0); got != "he      " {
		t.Errorf("row0 = %q, want %q", got, "he      ")
	}
}

func TestEraseWholeLine(t *testing.T) {
	g := feed(8, 1, "hello\x1b[2K")
	if got := row(g, 0); got != "        " {
		t.Errorf("row0 = %q, want all blank", got)
	}
}

func TestEraseDisplay(t *testing.T) {
	g := feed(4, 2, "ab\r\ncd\x1b[2J")
	if row(g, 0) != "    " || row(g, 1) != "    " {
		t.Errorf("rows = %q / %q, want all blank", row(g, 0), row(g, 1))
	}
}

// --- SGR ---------------------------------------------------------------------

func TestSGRBold(t *testing.T) {
	g := feed(3, 1, "\x1b[1mX")
	if a := g.CellAt(0, 0).Style.Attrs; a&AttrBold == 0 {
		t.Errorf("attrs = %b, want bold set", a)
	}
}

func TestSGRIndexedColor(t *testing.T) {
	g := feed(3, 1, "\x1b[31;42mX")
	st := g.CellAt(0, 0).Style
	if st.FG != (Color{Kind: ColorIndexed, Idx: 1}) {
		t.Errorf("FG = %+v, want indexed 1 (red)", st.FG)
	}
	if st.BG != (Color{Kind: ColorIndexed, Idx: 2}) {
		t.Errorf("BG = %+v, want indexed 2 (green)", st.BG)
	}
}

func TestSGRReset(t *testing.T) {
	g := feed(3, 1, "\x1b[1;31mX\x1b[0mY")
	if (g.CellAt(1, 0).Style != Style{}) {
		t.Errorf("second cell style = %+v, want default after reset", g.CellAt(1, 0).Style)
	}
}

func TestSGR256(t *testing.T) {
	g := feed(3, 1, "\x1b[38;5;200mX")
	if fg := g.CellAt(0, 0).Style.FG; fg != (Color{Kind: ColorIndexed, Idx: 200}) {
		t.Errorf("FG = %+v, want indexed 200", fg)
	}
}

func TestSGRTrueColor(t *testing.T) {
	g := feed(3, 1, "\x1b[38;2;10;20;30mX")
	if fg := g.CellAt(0, 0).Style.FG; fg != (Color{Kind: ColorRGB, R: 10, G: 20, B: 30}) {
		t.Errorf("FG = %+v, want rgb(10,20,30)", fg)
	}
}

// --- sequences that must be swallowed, not printed ---------------------------

func TestOSCSwallowed(t *testing.T) {
	// OSC set-title, BEL-terminated, must not leak into the grid.
	g := feed(5, 1, "\x1b]0;my title\x07hi")
	if got := row(g, 0); got != "hi   " {
		t.Errorf("row0 = %q, want %q (OSC leaked)", got, "hi   ")
	}
}

func TestOSCStTerminated(t *testing.T) {
	g := feed(5, 1, "\x1b]0;t\x1b\\hi")
	if got := row(g, 0); got != "hi   " {
		t.Errorf("row0 = %q, want %q (OSC ST leaked)", got, "hi   ")
	}
}

func TestCharsetDesignationSwallowed(t *testing.T) {
	g := feed(5, 1, "\x1b(Bhi")
	if got := row(g, 0); got != "hi   " {
		t.Errorf("row0 = %q, want %q (charset leaked)", got, "hi   ")
	}
}
