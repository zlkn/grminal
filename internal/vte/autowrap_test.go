package vte

import "testing"

// These tests pin VT100 deferred autowrap (xterm's do_wrap flag) and DECAWM
// (mode ?7). Each case would fail under eager wrapping (advancing the row the
// instant the last column is written), which mis-scrolled apps like neovim that
// fill the rightmost cell — the duplicated/dropped-line artifact.

// Filling the last column of the bottom row must NOT scroll on its own; the wrap
// is deferred until the next glyph. Eager wrap would scroll "abc" up to row 0.
func TestAutowrapDefersAtMargin(t *testing.T) {
	g := feed(3, 2, "\x1b[2;1Habc") // home to bottom row, fill it exactly
	if row(g, 0) != "   " || row(g, 1) != "abc" {
		t.Errorf("rows = %q / %q, want blank / \"abc\" (no premature scroll)", row(g, 0), row(g, 1))
	}
	if cx, cy := g.Cursor(); cx != 2 || cy != 1 {
		t.Errorf("cursor = (%d,%d), want (2,1) parked at the margin", cx, cy)
	}
}

// Moving the cursor after a margin fill cancels the pending wrap: no scroll, and
// the next write lands where addressed rather than on a wrapped line.
func TestAutowrapCancelledByCursorMove(t *testing.T) {
	g := feed(3, 2, "\x1b[2;1Habc\x1b[1;1HX") // fill bottom row, then home + write
	if row(g, 0) != "X  " || row(g, 1) != "abc" {
		t.Errorf("rows = %q / %q, want \"X  \" / \"abc\"", row(g, 0), row(g, 1))
	}
}

// The deferred wrap DOES fire when an overflowing glyph actually arrives, and
// only then scrolls the region.
func TestAutowrapFiresOnOverflowGlyph(t *testing.T) {
	g := feed(3, 2, "\x1b[2;1Habcd") // one glyph past the bottom-row margin
	if row(g, 0) != "abc" || row(g, 1) != "d  " {
		t.Errorf("rows = %q / %q, want \"abc\" / \"d  \" (wrap+scroll on overflow)", row(g, 0), row(g, 1))
	}
}

// CR/LF after a margin fill must move down exactly one row, not two. Eager wrap
// left a spurious blank line ("e" on row 2 instead of row 1).
func TestAutowrapNoDoubleAdvanceOnNewline(t *testing.T) {
	g := feed(4, 3, "abcd\r\ne")
	if row(g, 0) != "abcd" || row(g, 1) != "e   " || row(g, 2) != "    " {
		t.Errorf("rows = %q / %q / %q, want \"abcd\" / \"e   \" / blank",
			row(g, 0), row(g, 1), row(g, 2))
	}
}

// DECAWM off (?7l): text at the right margin overwrites the last column instead
// of wrapping to a new line.
func TestDECAWMOffClampsAtMargin(t *testing.T) {
	g := feed(3, 1, "\x1b[?7labcd") // disable autowrap, then overflow
	if row(g, 0) != "abd" {
		t.Errorf("row0 = %q, want \"abd\" (last column overwritten, no wrap)", row(g, 0))
	}
	if cx, _ := g.Cursor(); cx != 2 {
		t.Errorf("cursor col = %d, want 2 (clamped at margin)", cx)
	}
}

// Re-enabling DECAWM (?7h) restores deferred wrapping.
func TestDECAWMReenabled(t *testing.T) {
	g := feed(3, 2, "\x1b[?7l\x1b[?7habcd") // off then on, then overflow
	if row(g, 0) != "abc" || row(g, 1) != "d  " {
		t.Errorf("rows = %q / %q, want wrap restored", row(g, 0), row(g, 1))
	}
}

// DECCKM (?1) tracks application cursor key mode, which the input layer reads to
// send arrows as SS3 (ESC O x). ncurses apps like htop enable it via smkx; the
// default and RIS reset is off.
func TestDECCKMTracksApplicationCursorKeys(t *testing.T) {
	g := NewGrid(4, 2)
	if g.AppCursorKeys() {
		t.Fatal("DECCKM should default to off (CSI cursor keys)")
	}
	p := NewParser(g)
	p.Write([]byte("\x1b[?1h")) // smkx
	if !g.AppCursorKeys() {
		t.Error("ESC[?1h did not enable application cursor keys")
	}
	p.Write([]byte("\x1b[?1l")) // rmkx
	if g.AppCursorKeys() {
		t.Error("ESC[?1l did not disable application cursor keys")
	}
	p.Write([]byte("\x1b[?1h\x1bc")) // enable, then RIS must reset it
	if g.AppCursorKeys() {
		t.Error("RIS (ESC c) did not reset application cursor keys")
	}
}
