package app

import (
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
)

// handleInput dispatches this frame's keyboard input: global tab hotkeys first,
// then everything else encoded and forwarded to the focused pane's PTY.
func (g *game) handleInput() {
	ctrl := ebiten.IsKeyPressed(ebiten.KeyControl)
	shift := ebiten.IsKeyPressed(ebiten.KeyShift)
	alt := ebiten.IsKeyPressed(ebiten.KeyAlt)

	// Global hotkeys consume the frame's input when they match.
	for _, k := range inpututil.AppendJustPressedKeys(nil) {
		if r, ok := hotkeyRune(k); ok {
			if g.app.HandleKey(Key{Rune: r, Ctrl: ctrl, Shift: shift, Alt: alt}) {
				return
			}
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
	buf = appendSpecialKeys(buf, ctrl)

	if len(buf) > 0 {
		_, _ = p.Write(buf)
	}
}

// hotkeyRune maps the keys that can participate in a global hotkey to their rune.
func hotkeyRune(k ebiten.Key) (rune, bool) {
	switch {
	case k == ebiten.KeyT:
		return 't', true
	case k == ebiten.KeyW:
		return 'w', true
	case k == ebiten.KeyBracketLeft:
		return '[', true
	case k == ebiten.KeyBracketRight:
		return ']', true
	case k >= ebiten.KeyDigit1 && k <= ebiten.KeyDigit9:
		return rune('1' + (k - ebiten.KeyDigit1)), true
	}
	return 0, false
}

// appendSpecialKeys appends escape sequences / control bytes for the non-
// printable keys pressed this frame.
func appendSpecialKeys(b []byte, ctrl bool) []byte {
	seqs := []struct {
		key ebiten.Key
		out string
	}{
		{ebiten.KeyEnter, "\r"},
		{ebiten.KeyNumpadEnter, "\r"},
		{ebiten.KeyBackspace, "\x7f"},
		{ebiten.KeyTab, "\t"},
		{ebiten.KeyEscape, "\x1b"},
		{ebiten.KeyArrowUp, "\x1b[A"},
		{ebiten.KeyArrowDown, "\x1b[B"},
		{ebiten.KeyArrowRight, "\x1b[C"},
		{ebiten.KeyArrowLeft, "\x1b[D"},
		{ebiten.KeyHome, "\x1b[H"},
		{ebiten.KeyEnd, "\x1b[F"},
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
