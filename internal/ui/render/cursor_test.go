package render

import (
	"image/color"
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
)

// cursorRect places each shape within its cell: a block fills it, an underline
// sits on the bottom edge, a bar on the left edge. All three must stay inside the
// cell so a cursor never bleeds into its neighbours.
func TestCursorRect(t *testing.T) {
	const cellX, cellY, cellW, cellH = 100, 50, 10, 20

	cases := []struct {
		shape                      vte.CursorShape
		wantX, wantY, wantW, wantH float64
	}{
		{vte.CursorShapeBlock, 100, 50, 10, 20},
		{vte.CursorShapeUnderline, 100, 68, 10, 2}, // round(20*0.12) = 2, flush to the bottom
		{vte.CursorShapeBar, 100, 50, 2, 20},       // round(10*0.15) = 2, flush to the left
	}
	for _, tc := range cases {
		x, y, w, h := cursorRect(tc.shape, cellX, cellY, cellW, cellH)
		if x != tc.wantX || y != tc.wantY || w != tc.wantW || h != tc.wantH {
			t.Errorf("cursorRect(%v) = (%v,%v,%v,%v), want (%v,%v,%v,%v)",
				tc.shape, x, y, w, h, tc.wantX, tc.wantY, tc.wantW, tc.wantH)
		}
		if x < cellX || y < cellY || x+w > cellX+cellW || y+h > cellY+cellH {
			t.Errorf("cursorRect(%v) = (%v,%v,%v,%v) escapes its cell", tc.shape, x, y, w, h)
		}
	}
}

// Thin shapes must survive tiny cells: a rounded-to-zero thickness would draw
// nothing at all.
func TestCursorRectThinShapesStayVisible(t *testing.T) {
	for _, shape := range []vte.CursorShape{vte.CursorShapeUnderline, vte.CursorShapeBar} {
		_, _, w, h := cursorRect(shape, 0, 0, 3, 4)
		if w < 1 || h < 1 {
			t.Errorf("cursorRect(%v) in a 3x4 cell = %vx%v, want at least 1px each way", shape, w, h)
		}
	}
}

func TestShapeFromStyle(t *testing.T) {
	cases := map[string]vte.CursorShape{
		"block":     vte.CursorShapeBlock,
		"beam":      vte.CursorShapeBar,
		"underline": vte.CursorShapeUnderline,
		"nonsense":  vte.CursorShapeBlock, // never drawn as "nothing"
	}
	for name, want := range cases {
		if got := shapeFromStyle(name); got != want {
			t.Errorf("shapeFromStyle(%q) = %v, want %v", name, got, want)
		}
	}
}

// The app's DECSCUSR request overrides the configured default in both directions;
// only CursorShapeDefault/CursorBlinkDefault fall back to the config.
func TestCursorOverridesFromApp(t *testing.T) {
	blinkCfg := &Renderer{cursorShape: vte.CursorShapeBlock, cursorBlink: true}
	steadyCfg := &Renderer{cursorShape: vte.CursorShapeBar, cursorBlink: false}

	blinkCases := []struct {
		r    *Renderer
		in   vte.CursorBlink
		want bool
	}{
		{blinkCfg, vte.CursorBlinkDefault, true},
		{blinkCfg, vte.CursorBlinkOff, false},
		{steadyCfg, vte.CursorBlinkDefault, false},
		{steadyCfg, vte.CursorBlinkOn, true},
	}
	for _, tc := range blinkCases {
		if got := tc.r.CursorBlinking(tc.in); got != tc.want {
			t.Errorf("CursorBlinking(%v) with config %t = %t, want %t",
				tc.in, tc.r.cursorBlink, got, tc.want)
		}
	}

	if got := steadyCfg.shapeFor(vte.CursorShapeDefault); got != vte.CursorShapeBar {
		t.Errorf("shapeFor(default) = %v, want the configured bar", got)
	}
	if got := steadyCfg.shapeFor(vte.CursorShapeUnderline); got != vte.CursorShapeUnderline {
		t.Errorf("shapeFor(underline) = %v, want the app's underline", got)
	}
}

// color.RGBA is alpha-premultiplied, so the components must scale with the alpha:
// leaving them at full strength draws an over-bright cursor.
func TestPremultiply(t *testing.T) {
	cases := []struct {
		alpha float64
		want  color.RGBA
	}{
		{1, color.RGBA{0x20, 0xc0, 0xff, 0xff}},
		{0.5, color.RGBA{0x10, 0x60, 0x80, 0x80}},
		{0, color.RGBA{0, 0, 0, 0}},
		{2, color.RGBA{0x20, 0xc0, 0xff, 0xff}}, // clamped
		{-1, color.RGBA{0, 0, 0, 0}},            // clamped
	}
	in := color.RGBA{0x20, 0xc0, 0xff, 0xff}
	for _, tc := range cases {
		got := premultiply(in, tc.alpha)
		if got != tc.want {
			t.Errorf("premultiply(%v, %v) = %v, want %v", in, tc.alpha, got, tc.want)
		}
		if got.R > got.A || got.G > got.A || got.B > got.A {
			t.Errorf("premultiply(%v, %v) = %v: a component exceeds alpha", in, tc.alpha, got)
		}
	}
}
