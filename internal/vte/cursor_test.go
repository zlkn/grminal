package vte_test

import (
	"bytes"
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

func TestRISResetsCursorState(t *testing.T) {
	g := vte.NewGrid(10, 5)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b[?25l"))   // hide the cursor
	p.Write([]byte("\x1b[5 q"))    // blinking bar
	p.Write([]byte("\x1b[2;3r"))   // scrolling region rows 2..3
	p.Write([]byte("\x1b[?1049h")) // alternate screen
	p.Write([]byte("\x1b[3;3H"))   // move somewhere
	p.Write([]byte("\x1b7"))       // leave a save slot behind

	p.Write([]byte("\x1bc")) // RIS

	snap := g.Snapshot()
	switch {
	case !snap.CursorVisible:
		t.Error("RIS left the cursor hidden")
	case snap.CursorShape != vte.CursorShapeDefault:
		t.Errorf("RIS left shape = %v, want default", snap.CursorShape)
	case snap.CursorBlink != vte.CursorBlinkDefault:
		t.Errorf("RIS left blink = %v, want default", snap.CursorBlink)
	}
	if g.ScrollTop() != 0 || g.ScrollBot() != g.Rows()-1 {
		t.Errorf("RIS left region = [%d,%d], want [0,%d]", g.ScrollTop(), g.ScrollBot(), g.Rows()-1)
	}
	if g.OnAltScreen() {
		t.Error("RIS left the alternate screen active")
	}
	if cx, cy := g.Cursor(); cx != 0 || cy != 0 {
		t.Errorf("RIS left cursor at (%d,%d), want (0,0)", cx, cy)
	}
}

func TestSavedCursorIsPerScreenBuffer(t *testing.T) {
	g := vte.NewGrid(10, 6)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b[3;3H"))   // primary cursor -> (2,2)
	p.Write([]byte("\x1b[?1049h")) // save primary cursor + enter alt
	p.Write([]byte("\x1b[5;5H"))   // move within alt
	p.Write([]byte("\x1b7"))       // app saves the cursor while in alt
	p.Write([]byte("\x1b[?1049l")) // leave alt, restore the primary cursor

	if cx, cy := g.Cursor(); cx != 2 || cy != 2 {
		t.Errorf("cursor after alt exit = (%d,%d), want (2,2)", cx, cy)
	}
}

func TestDECSCSavesPenAndOriginMode(t *testing.T) {
	g := vte.NewGrid(10, 5)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b[?6h"))    // origin mode on
	p.Write([]byte("\x1b[31;44m")) // red on blue
	p.Write([]byte("\x1b7"))       // DECSC
	p.Write([]byte("\x1b[0m"))     // reset the pen
	p.Write([]byte("\x1b[?6l"))    // origin mode off
	p.Write([]byte("\x1b8"))       // DECRC
	p.Write([]byte("X"))

	got := g.CellAt(0, 0).Style
	want := vte.Style{FG: vte.Color{Kind: vte.ColorIndexed, Idx: 1}, BG: vte.Color{Kind: vte.ColorIndexed, Idx: 4}}
	if got != want {
		t.Errorf("style after DECRC = %+v, want %+v", got, want)
	}
}

func TestDECRCWithoutSaveHomesCursor(t *testing.T) {
	g := vte.NewGrid(10, 5)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b[3;4H\x1b[31m"))
	p.Write([]byte("\x1b8X"))

	if got := g.CellAt(0, 0); got.Rune != 'X' || got.Style != (vte.Style{}) {
		t.Errorf("CellAt(0,0) = %+v, want 'X' in the default style", got)
	}
}

func TestDECSCUSR(t *testing.T) {
	cases := []struct {
		seq       string
		wantShape vte.CursorShape
		wantBlink vte.CursorBlink
	}{
		{"\x1b[ q", vte.CursorShapeDefault, vte.CursorBlinkDefault},
		{"\x1b[0 q", vte.CursorShapeDefault, vte.CursorBlinkDefault},
		{"\x1b[1 q", vte.CursorShapeBlock, vte.CursorBlinkOn},
		{"\x1b[2 q", vte.CursorShapeBlock, vte.CursorBlinkOff},
		{"\x1b[3 q", vte.CursorShapeUnderline, vte.CursorBlinkOn},
		{"\x1b[4 q", vte.CursorShapeUnderline, vte.CursorBlinkOff},
		{"\x1b[5 q", vte.CursorShapeBar, vte.CursorBlinkOn},
		{"\x1b[6 q", vte.CursorShapeBar, vte.CursorBlinkOff},
		{"\x1b[9 q", vte.CursorShapeUnderline, vte.CursorBlinkOff},
		{"\x1b[2 q\x1b[5 q", vte.CursorShapeBar, vte.CursorBlinkOn},
		{"\x1b[2 \x1b[5 q", vte.CursorShapeBar, vte.CursorBlinkOn},
	}
	for _, tc := range cases {
		g := vte.NewGrid(10, 2)
		p := parser.NewParser(g)
		p.Write([]byte("\x1b[4 q"))
		p.Write([]byte(tc.seq))
		if g.CursorShape() != tc.wantShape || g.CursorBlink() != tc.wantBlink {
			t.Errorf("%q -> shape %v blink %v, want shape %v blink %v",
				tc.seq, g.CursorShape(), g.CursorBlink(), tc.wantShape, tc.wantBlink)
		}
	}
}

func TestDECSCUSRDoesNotEatText(t *testing.T) {
	g := vte.NewGrid(10, 2)
	p := parser.NewParser(g)

	p.Write([]byte("\x1b[5 qAB"))
	if got := row(g, 0); got != "AB        " {
		t.Errorf("row0 = %q, want %q", got, "AB        ")
	}

	p.Write([]byte("\x1b[1\"qC"))
	if g.CursorShape() != vte.CursorShapeBar {
		t.Errorf("DECSCA changed the cursor shape to %v", g.CursorShape())
	}
}

func TestCursorPositionReportUnderOriginMode(t *testing.T) {
	g := vte.NewGrid(10, 6)
	p := parser.NewParser(g)
	var reply bytes.Buffer
	p.SetReply(&reply)

	p.Write([]byte("\x1b[2;5r"))
	p.Write([]byte("\x1b[?6h"))
	p.Write([]byte("\x1b[2;3H"))
	p.Write([]byte("\x1b[6n"))

	if got := reply.String(); got != "\x1b[2;3R" {
		t.Errorf("CPR under origin mode = %q, want %q", got, "\x1b[2;3R")
	}
}

func TestCursorUpDownRespectScrollRegion(t *testing.T) {
	cases := []struct {
		name  string
		setup string
		seq   string
		wantY int
	}{
		{"CUD stops at the bottom margin", "\x1b[2;2H", "\x1b[10B", 3},
		{"CUU stops at the top margin", "\x1b[4;2H", "\x1b[10A", 1},
		{"CUD below the region stops at the screen bottom", "\x1b[6;2H", "\x1b[10B", 5},
		{"CUU above the region stops at the screen top", "\x1b[1;2H", "\x1b[10A", 0},
	}
	for _, tc := range cases {
		g := vte.NewGrid(10, 6)
		p := parser.NewParser(g)
		p.Write([]byte("\x1b[2;4r"))
		p.Write([]byte(tc.setup))
		p.Write([]byte(tc.seq))
		if _, cy := g.Cursor(); cy != tc.wantY {
			t.Errorf("%s: cursor row = %d, want %d", tc.name, cy, tc.wantY)
		}
		if got := row(g, 0); got != "          " {
			t.Errorf("%s: row0 = %q, cursor movement must not scroll", tc.name, got)
		}
	}
}

func TestCursorVisibleAccessor(t *testing.T) {
	g := vte.NewGrid(4, 2)
	p := parser.NewParser(g)

	if !g.CursorVisible() {
		t.Error("cursor should be visible by default")
	}
	p.Write([]byte("\x1b[?25l"))
	if g.CursorVisible() {
		t.Error("cursor should be hidden after ?25l")
	}
}
