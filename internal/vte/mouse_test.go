package vte

import "testing"

// setModes feeds DEC private-mode escapes so tests exercise the real parser path
// (e.g. "?1000h?1006h" enables X11 tracking + SGR encoding).
func mouseGrid(t *testing.T, enable string) *Grid {
	t.Helper()
	g := NewGrid(80, 24)
	NewParser(g).Write([]byte(enable))
	return g
}

func TestEncodeMouseOffReportsNothing(t *testing.T) {
	g := NewGrid(80, 24)
	if b := g.EncodeMouse(MouseEvent{Button: MouseLeft}); b != nil {
		t.Errorf("reporting off should yield nil, got %q", b)
	}
}

func TestEncodeMouseSGR(t *testing.T) {
	g := mouseGrid(t, "\x1b[?1000h\x1b[?1006h")
	cases := []struct {
		name string
		ev   MouseEvent
		want string
	}{
		{"left press at origin", MouseEvent{Button: MouseLeft}, "\x1b[<0;1;1M"},
		{"left release at origin", MouseEvent{Button: MouseLeft, Release: true}, "\x1b[<0;1;1m"},
		{"right press mid-screen", MouseEvent{Button: MouseRight, Col: 9, Row: 4}, "\x1b[<2;10;5M"},
		{"wheel up", MouseEvent{Button: MouseWheelUp, Col: 2, Row: 2}, "\x1b[<64;3;3M"},
		{"ctrl+left", MouseEvent{Button: MouseLeft, Ctrl: true}, "\x1b[<16;1;1M"},
	}
	for _, tc := range cases {
		if got := string(g.EncodeMouse(tc.ev)); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestEncodeMouseX10(t *testing.T) {
	g := mouseGrid(t, "\x1b[?1000h") // no ?1006 -> legacy X10 encoding
	// left press at (0,0): Cb=0+32=' ', Cx=1+32='!', Cy=1+32='!'.
	if got := string(g.EncodeMouse(MouseEvent{Button: MouseLeft})); got != "\x1b[M !!" {
		t.Errorf("X10 press: got %q, want %q", got, "\x1b[M !!")
	}
	// release reports button 3: Cb=3+32='#'.
	if got := string(g.EncodeMouse(MouseEvent{Button: MouseLeft, Release: true})); got != "\x1b[M#!!" {
		t.Errorf("X10 release: got %q, want %q", got, "\x1b[M#!!")
	}
}

func TestEncodeMouseMotionGatedByMode(t *testing.T) {
	drag := MouseEvent{Button: MouseLeft, Motion: true, Col: 1, Row: 1}
	hover := MouseEvent{Button: MouseNone, Motion: true, Col: 1, Row: 1}

	x11 := mouseGrid(t, "\x1b[?1000h\x1b[?1006h")
	if b := x11.EncodeMouse(drag); b != nil {
		t.Errorf("?1000 must not report motion, got %q", b)
	}

	btn := mouseGrid(t, "\x1b[?1002h\x1b[?1006h")
	if b := btn.EncodeMouse(hover); b != nil {
		t.Errorf("?1002 must not report motion with no button, got %q", b)
	}
	if b := btn.EncodeMouse(drag); b == nil {
		t.Error("?1002 should report drag while a button is held")
	}

	any := mouseGrid(t, "\x1b[?1003h\x1b[?1006h")
	if b := any.EncodeMouse(hover); b == nil {
		t.Error("?1003 should report buttonless motion")
	}
}

func TestMouseModeResetByRIS(t *testing.T) {
	g := mouseGrid(t, "\x1b[?1000h\x1b[?1006h\x1bc") // enable, then RIS
	if b := g.EncodeMouse(MouseEvent{Button: MouseLeft}); b != nil {
		t.Errorf("RIS should disable mouse reporting, got %q", b)
	}
}
