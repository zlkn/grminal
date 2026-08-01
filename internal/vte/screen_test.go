package vte_test

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

func TestAltScreenIsolatesAndRestores(t *testing.T) {
	g := vte.NewGrid(5, 2)
	p := parser.NewParser(g)

	p.Write([]byte("AB"))
	p.Write([]byte("\x1b[?1049h"))
	if got := row(g, 0); got != "     " {
		t.Fatalf("alt screen not cleared: row0 = %q", got)
	}
	p.Write([]byte("\x1b[HXY"))
	if got := row(g, 0); got != "XY   " {
		t.Fatalf("alt write: row0 = %q", got)
	}

	p.Write([]byte("\x1b[?1049l"))
	if got := row(g, 0); got != "AB   " {
		t.Errorf("primary not restored: row0 = %q, want %q", got, "AB   ")
	}
	if cx, cy := g.Cursor(); cx != 2 || cy != 0 {
		t.Errorf("cursor after restore = (%d,%d), want (2,0)", cx, cy)
	}
}

func TestSaveRestoreCursorCSI(t *testing.T) {
	g := vte.NewGrid(10, 5)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b[3;4H"))
	p.Write([]byte("\x1b[s"))
	p.Write([]byte("\x1b[HX"))
	p.Write([]byte("\x1b[u"))
	p.Write([]byte("Y"))

	if got := g.CellAt(3, 2).Rune; got != 'Y' {
		t.Errorf("CellAt(3,2) = %q, want 'Y'", got)
	}
}

func TestSaveRestoreCursorDEC(t *testing.T) {
	g := vte.NewGrid(10, 5)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b[2;2H"))
	p.Write([]byte("\x1b7"))
	p.Write([]byte("\x1b[HZ"))
	p.Write([]byte("\x1b8"))
	p.Write([]byte("Q"))

	if got := g.CellAt(1, 1).Rune; got != 'Q' {
		t.Errorf("CellAt(1,1) = %q, want 'Q'", got)
	}
}

func TestOSCTitle(t *testing.T) {
	g := vte.NewGrid(10, 2)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b]0;hello\x07"))
	if got := g.Snapshot().Title; got != "hello" {
		t.Errorf("title = %q, want %q", got, "hello")
	}
	p.Write([]byte("\x1b]2;world\x1b\\"))
	if got := g.Snapshot().Title; got != "world" {
		t.Errorf("title = %q, want %q", got, "world")
	}
	p.Write([]byte("hi"))
	if row(g, 0) != "hi        " {
		t.Errorf("row0 = %q, want text after OSC", row(g, 0))
	}
}

func TestOSCNonTitleIgnored(t *testing.T) {
	g := vte.NewGrid(10, 2)
	p := parser.NewParser(g)
	p.Write([]byte("\x1b]0;init\x07"))
	p.Write([]byte("\x1b]4;1;rgb:ff/00/00\x07"))
	if got := g.Snapshot().Title; got != "init" {
		t.Errorf("title = %q, want unchanged %q", got, "init")
	}
}

func TestCursorVisibility(t *testing.T) {
	g := vte.NewGrid(3, 1)
	p := parser.NewParser(g)

	if !g.Snapshot().CursorVisible {
		t.Error("cursor should be visible by default")
	}
	p.Write([]byte("\x1b[?25l"))
	if g.Snapshot().CursorVisible {
		t.Error("cursor should be hidden after ?25l")
	}
	p.Write([]byte("\x1b[?25h"))
	if !g.Snapshot().CursorVisible {
		t.Error("cursor should be visible after ?25h")
	}
}
