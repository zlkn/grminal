package app

import (
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"

	"github.com/yzolkin/go-vte/internal/pane"
	"github.com/yzolkin/go-vte/internal/vte"
)

// handleInput dispatches this frame's keyboard input: global tab hotkeys first,
// then everything else encoded and forwarded to the focused pane's PTY.
func (g *game) handleInput() {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)

	// Global hotkeys consume the frame's input when they match a binding.
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		name, ok := keyName(k)
		if !ok {
			continue
		}
		if g.app.HandleKey(Key{Name: name, Ctrl: ctrl, Shift: shift, Alt: alt}) {
			g.dirty.Store(true) // tab set/active changed; repaint bar + pane
			return
		}
	}

	p := g.active()
	if p == nil {
		return
	}

	var buf []byte
	// Printable characters (respects keyboard layout and shift).
	for _, r := range ebiten.AppendInputChars(nil) {
		buf = utf8.AppendRune(buf, r)
	}
	buf = appendSpecialKeys(buf, ctrl, p.AppCursorKeys())

	if len(buf) > 0 {
		_, _ = p.Write(buf)
		g.dirty.Store(true) // echo/response will arrive; ensure a redraw
	}

	g.handleMouse(p)
}

// handleMouse forwards pointer events (button press/release, drag, wheel) to the
// pane's PTY, encoded per its mouse-reporting mode. When reporting is off the
// pane returns nil and nothing is sent, so ordinary shells ignore the mouse.
func (g *game) handleMouse(p *pane.Pane) {
	mx, my := ebiten.CursorPosition()
	col, row, inGrid := g.r.CellAt(mx, my)
	col = clampInt(col, 0, g.cols-1)
	row = clampInt(row, 0, g.rows-1)

	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
	send := func(ev vte.MouseEvent) {
		ev.Col, ev.Row = col, row
		ev.Shift, ev.Alt, ev.Ctrl = shift, alt, ctrl
		if b := p.EncodeMouse(ev); b != nil {
			_, _ = p.Write(b)
			g.dirty.Store(true)
		}
	}

	buttons := []struct {
		mb ebiten.MouseButton
		vb vte.MouseButton
	}{
		{ebiten.MouseButtonLeft, vte.MouseLeft},
		{ebiten.MouseButtonMiddle, vte.MouseMiddle},
		{ebiten.MouseButtonRight, vte.MouseRight},
	}
	for _, bt := range buttons {
		// A press only starts inside the grid (not on the tab bar); a release is
		// always reported so the app sees the button go up wherever it lands.
		if inGrid && inpututil.IsMouseButtonJustPressed(bt.mb) {
			g.mouseBtn, g.mouseHeld = bt.vb, true
			g.mouseCol, g.mouseRow = col, row
			send(vte.MouseEvent{Button: bt.vb})
		}
		if inpututil.IsMouseButtonJustReleased(bt.mb) {
			send(vte.MouseEvent{Button: bt.vb, Release: true})
			g.mouseHeld, g.mouseBtn = false, vte.MouseNone
		}
	}

	// Motion: report only when the pointer changes cell (the pane gates by mode,
	// so this is a no-op unless the app enabled drag/any-motion tracking).
	if inGrid && (col != g.mouseCol || row != g.mouseRow) {
		g.mouseCol, g.mouseRow = col, row
		btn := vte.MouseNone
		if g.mouseHeld {
			btn = g.mouseBtn
		}
		send(vte.MouseEvent{Button: btn, Motion: true})
	}

	// Wheel: positive dy scrolls up.
	if _, dy := ebiten.Wheel(); dy != 0 && inGrid {
		btn := vte.MouseWheelDown
		if dy > 0 {
			btn = vte.MouseWheelUp
		}
		send(vte.MouseEvent{Button: btn})
	}
}

// clampInt bounds v to [lo, hi].
func clampInt(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// keyName maps an Ebiten key to the normalized token used in keybindings
// (lowercase letters, digits, brackets, and "tab"). Keys that cannot appear in a
// binding return ok=false.
func keyName(k ebiten.Key) (string, bool) {
	switch {
	case k >= ebiten.KeyA && k <= ebiten.KeyZ:
		return string(rune('a' + (k - ebiten.KeyA))), true
	case k >= ebiten.KeyDigit0 && k <= ebiten.KeyDigit9:
		return string(rune('0' + (k - ebiten.KeyDigit0))), true
	case k == ebiten.KeyBracketLeft:
		return "[", true
	case k == ebiten.KeyBracketRight:
		return "]", true
	case k == ebiten.KeyTab:
		return "tab", true
	}
	return "", false
}

// appendSpecialKeys appends escape sequences / control bytes for the non-
// printable keys pressed this frame. When appCursor is set (DECCKM, e.g. under
// htop), the cursor keys are encoded as SS3 (ESC O x) instead of CSI (ESC [ x).
func appendSpecialKeys(b []byte, ctrl, appCursor bool) []byte {
	// Cursor keys: the introducer switches between CSI and SS3; the final byte
	// is the same in both forms.
	prefix := "\x1b["
	if appCursor {
		prefix = "\x1bO"
	}
	cursor := []struct {
		key   ebiten.Key
		final byte
	}{
		{ebiten.KeyArrowUp, 'A'},
		{ebiten.KeyArrowDown, 'B'},
		{ebiten.KeyArrowRight, 'C'},
		{ebiten.KeyArrowLeft, 'D'},
		{ebiten.KeyHome, 'H'},
		{ebiten.KeyEnd, 'F'},
	}
	for _, c := range cursor {
		if inpututil.IsKeyJustPressed(c.key) {
			b = append(b, prefix...)
			b = append(b, c.final)
		}
	}

	seqs := []struct {
		key ebiten.Key
		out string
	}{
		{ebiten.KeyEnter, "\r"},
		{ebiten.KeyNumpadEnter, "\r"},
		{ebiten.KeyBackspace, "\x7f"},
		{ebiten.KeyTab, "\t"},
		{ebiten.KeyEscape, "\x1b"},
		{ebiten.KeyDelete, "\x1b[3~"},
		{ebiten.KeyPageUp, "\x1b[5~"},
		{ebiten.KeyPageDown, "\x1b[6~"},
	}
	for _, s := range seqs {
		if inpututil.IsKeyJustPressed(s.key) {
			b = append(b, s.out...)
		}
	}

	// Ctrl+A..Z -> 0x01..0x1a (Ctrl+letter control codes).
	if ctrl {
		for k := ebiten.KeyA; k <= ebiten.KeyZ; k++ {
			if inpututil.IsKeyJustPressed(k) {
				b = append(b, byte(k-ebiten.KeyA)+1)
			}
		}
	}
	return b
}
