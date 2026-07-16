package vte

import "strconv"

// mouseMode is the active mouse-reporting level, set by DEC private modes
// ?1000/?1002/?1003. The SGR extended encoding (?1006) is tracked separately.
type mouseMode uint8

const (
	mouseOff    mouseMode = iota // no reporting
	mouseX11                     // ?1000: button press and release
	mouseButton                  // ?1002: + motion while a button is held (drag)
	mouseAny                     // ?1003: + all motion
)

// MouseButton identifies the button (or wheel direction) of a MouseEvent. The
// values are the xterm button-code bits: 0/1/2 for left/middle/right, 64/65 for
// wheel up/down, and 3 for "no button" (motion with nothing held).
type MouseButton uint8

const (
	MouseLeft      MouseButton = 0
	MouseMiddle    MouseButton = 1
	MouseRight     MouseButton = 2
	MouseNone      MouseButton = 3
	MouseWheelUp   MouseButton = 64
	MouseWheelDown MouseButton = 65
)

// MouseEvent is a pointer event in 0-based cell coordinates, decoupled from the
// input backend so encoding is testable.
type MouseEvent struct {
	Col, Row         int
	Button           MouseButton
	Motion           bool // pointer moved (drag) rather than a discrete press
	Release          bool // button released (ignored for wheel)
	Shift, Alt, Ctrl bool
}

// setMouseMode enables reporting level m, or disables reporting when on is false
// and m is the level currently active.
func (g *Grid) setMouseMode(m mouseMode, on bool) {
	if on {
		g.mouseMode = m
	} else if g.mouseMode == m {
		g.mouseMode = mouseOff
	}
}

// MouseEnabled reports whether the app has turned on any mouse-reporting mode,
// i.e. it owns the wheel. The input layer reads this (via the pane) to decide
// whether the wheel scrolls local scrollback or is forwarded to the child.
func (g *Grid) MouseEnabled() bool { return g.mouseMode != mouseOff }

// EncodeMouse returns the bytes to send to the PTY for ev under the grid's
// current mouse mode, or nil when the event must not be reported (reporting
// off, or a motion event the active mode does not track).
func (g *Grid) EncodeMouse(ev MouseEvent) []byte {
	if g.mouseMode == mouseOff {
		return nil
	}
	if ev.Motion {
		switch g.mouseMode {
		case mouseAny: // reports all motion
		case mouseButton:
			if ev.Button == MouseNone {
				return nil // ?1002 reports motion only while a button is held
			}
		default:
			return nil // ?1000 does not report motion
		}
	}

	cb := int(ev.Button)
	if ev.Motion {
		cb += 32
	}
	if ev.Shift {
		cb += 4
	}
	if ev.Alt {
		cb += 8
	}
	if ev.Ctrl {
		cb += 16
	}

	x, y := ev.Col+1, ev.Row+1 // reports are 1-based

	if g.mouseSGR {
		final := "M"
		if ev.Release {
			final = "m" // SGR marks release by the final byte, keeping the button
		}
		return []byte("\x1b[<" + strconv.Itoa(cb) + ";" + strconv.Itoa(x) + ";" + strconv.Itoa(y) + final)
	}

	// Legacy X10: release is reported as button 3, and each coordinate is a
	// single byte offset by 32, so it cannot exceed 223.
	if ev.Release {
		cb = cb&^3 | 3
	}
	return []byte{0x1b, '[', 'M', byte(cb + 32), byte(clampCoord(x) + 32), byte(clampCoord(y) + 32)}
}

// clampCoord bounds an X10 mouse coordinate to the 1..223 encodable range.
func clampCoord(v int) int {
	if v < 1 {
		return 1
	}
	if v > 223 {
		return 223
	}
	return v
}
