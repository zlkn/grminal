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

	switch {
	case len(buf) > 0:
		// A fresh press this frame: send it and arm autorepeat on the physical
		// key that produced it, so holding re-sends the same bytes. Typing also
		// snaps the view back to the live screen.
		g.repeat.arm(primaryPressedKey(), buf)
		g.scrollOff, g.scrollAccum = 0, 0
		_, _ = p.Write(buf)
		g.dirty.Store(true) // echo/response will arrive; ensure a redraw
	default:
		// No fresh press: re-emit the armed key's bytes if it is still held past
		// the delay. Ebiten does not surface OS autorepeat, so we synthesize it.
		if rep := g.repeat.repeatBytes(g.repeatDelay, g.repeatInterval); rep != nil {
			_, _ = p.Write(rep)
			g.dirty.Store(true)
		}
	}

	g.handleMouse(p)
}

// keyNone is the sentinel for "no key" in keyRepeat.
const keyNone = ebiten.Key(-1)

// keyRepeat holds the state needed to synthesize OS-style key autorepeat: the
// physical key currently held and the exact bytes to re-emit for it.
type keyRepeat struct {
	key     ebiten.Key
	armed   bool
	payload []byte
}

// arm starts (or restarts) autorepeat for key with the given payload. A keyNone
// key disarms — the fresh input had no repeatable physical key behind it.
func (r *keyRepeat) arm(key ebiten.Key, payload []byte) {
	if key == keyNone {
		r.armed = false
		return
	}
	r.key = key
	r.payload = append(r.payload[:0], payload...)
	r.armed = true
}

// repeatBytes returns the payload to re-send this tick, or nil. It fires once the
// armed key has been held past delay, then every interval ticks, and disarms as
// soon as the key is released. delay <= 0 disables autorepeat.
func (r *keyRepeat) repeatBytes(delay, interval int) []byte {
	if !r.armed {
		return nil
	}
	d := inpututil.KeyPressDuration(r.key)
	if d == 0 { // key released
		r.armed = false
		return nil
	}
	if shouldRepeat(d, delay, interval) {
		return r.payload
	}
	return nil
}

// shouldRepeat reports whether a key held for d ticks emits a repeat this tick.
// The first repeat lands at delay ticks, then every interval ticks after. A
// non-positive delay disables repeat; interval is clamped to at least one tick.
func shouldRepeat(d, delay, interval int) bool {
	if delay <= 0 || d < delay {
		return false
	}
	if interval < 1 {
		interval = 1
	}
	return (d-delay)%interval == 0
}

// primaryPressedKey returns the last non-modifier key pressed this frame, i.e.
// the one that should drive autorepeat, or keyNone if none qualifies.
func primaryPressedKey() ebiten.Key {
	key := keyNone
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		if isModifierKey(k) {
			continue
		}
		key = k
	}
	return key
}

// isModifierKey reports whether k is a bare modifier (Ctrl/Shift/Alt/Meta), which
// never drives autorepeat on its own.
func isModifierKey(k ebiten.Key) bool {
	switch k {
	case ebiten.KeyControl, ebiten.KeyControlLeft, ebiten.KeyControlRight,
		ebiten.KeyShift, ebiten.KeyShiftLeft, ebiten.KeyShiftRight,
		ebiten.KeyAlt, ebiten.KeyAltLeft, ebiten.KeyAltRight,
		ebiten.KeyMeta, ebiten.KeyMetaLeft, ebiten.KeyMetaRight:
		return true
	}
	return false
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

	// Wheel: positive dy scrolls up. When the app owns the mouse, forward the
	// event as a report; otherwise (plain shell, primary screen) scroll local
	// scrollback history instead.
	if _, dy := ebiten.Wheel(); dy != 0 && inGrid {
		switch {
		case p.MouseEnabled():
			btn := vte.MouseWheelDown
			if dy > 0 {
				btn = vte.MouseWheelUp
			}
			send(vte.MouseEvent{Button: btn})
		case !p.OnAltScreen():
			g.scrollHistory(p, dy)
		}
	}
}

// wheelScrollLines is how many scrollback lines one wheel notch moves the view.
const wheelScrollLines = 3

// scrollHistory moves the scrollback view by the wheel delta dy (positive = up =
// further into history), accumulating fractional trackpad deltas into whole
// notches and clamping to the available history. It marks the frame dirty when
// the offset actually changes.
func (g *game) scrollHistory(p *pane.Pane, dy float64) {
	g.scrollAccum += dy
	notches := int(g.scrollAccum)
	if notches == 0 {
		return
	}
	g.scrollAccum -= float64(notches)

	off := clampInt(g.scrollOff+notches*wheelScrollLines, 0, p.ScrollbackLen())
	if off != g.scrollOff {
		g.scrollOff = off
		g.dirty.Store(true)
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
