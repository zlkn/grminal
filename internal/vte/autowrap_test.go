package vte_test

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

func TestAutowrapDefersAtMargin(t *testing.T) {
	g := feed(3, 2, "\x1b[2;1Habc")
	if row(g, 0) != "   " || row(g, 1) != "abc" {
		t.Errorf("rows = %q / %q, want blank / \"abc\"", row(g, 0), row(g, 1))
	}
	if cx, cy := g.Cursor(); cx != 2 || cy != 1 {
		t.Errorf("cursor = (%d,%d), want (2,1)", cx, cy)
	}
}

func TestAutowrapCancelledByCursorMove(t *testing.T) {
	g := feed(3, 2, "\x1b[2;1Habc\x1b[1;1HX")
	if row(g, 0) != "X  " || row(g, 1) != "abc" {
		t.Errorf("rows = %q / %q, want \"X  \" / \"abc\"", row(g, 0), row(g, 1))
	}
}

func TestAutowrapFiresOnOverflowGlyph(t *testing.T) {
	g := feed(3, 2, "\x1b[2;1Habcd")
	if row(g, 0) != "abc" || row(g, 1) != "d  " {
		t.Errorf("rows = %q / %q, want \"abc\" / \"d  \"", row(g, 0), row(g, 1))
	}
}

func TestAutowrapNoDoubleAdvanceOnNewline(t *testing.T) {
	g := feed(4, 3, "abcd\r\ne")
	if row(g, 0) != "abcd" || row(g, 1) != "e   " || row(g, 2) != "    " {
		t.Errorf("rows = %q / %q / %q, want \"abcd\" / \"e   \" / blank",
			row(g, 0), row(g, 1), row(g, 2))
	}
}

func TestDECAWMOffClampsAtMargin(t *testing.T) {
	g := feed(3, 1, "\x1b[?7labcd")
	if row(g, 0) != "abd" {
		t.Errorf("row0 = %q, want \"abd\"", row(g, 0))
	}
	if cx, _ := g.Cursor(); cx != 2 {
		t.Errorf("cursor col = %d, want 2", cx)
	}
}

func TestDECAWMReenabled(t *testing.T) {
	g := feed(3, 2, "\x1b[?7l\x1b[?7habcd")
	if row(g, 0) != "abc" || row(g, 1) != "d  " {
		t.Errorf("rows = %q / %q, want wrap restored", row(g, 0), row(g, 1))
	}
}

func TestDECCKMTracksApplicationCursorKeys(t *testing.T) {
	g := vte.NewGrid(4, 2)
	if g.AppCursorKeys() {
		t.Fatal("DECCKM should default to off")
	}
	p := parser.NewParser(g)
	p.Write([]byte("\x1b[?1h"))
	if !g.AppCursorKeys() {
		t.Error("ESC[?1h did not enable application cursor keys")
	}
	p.Write([]byte("\x1b[?1l"))
	if g.AppCursorKeys() {
		t.Error("ESC[?1l did not disable application cursor keys")
	}
	p.Write([]byte("\x1b[?1h\x1bc"))
	if g.AppCursorKeys() {
		t.Error("RIS (ESC c) did not reset application cursor keys")
	}
}
