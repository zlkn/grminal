package render

import (
	"bytes"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/yzolkin/go-vte/fonts"
	"github.com/yzolkin/go-vte/internal/vte"
)

// Renderer draws terminal grids to an ebiten target. It owns the font face and
// the derived cell metrics. It is deliberately thin: all layout decisions live
// in the tested SplitRuns tokenizer; this file only issues GPU draw calls and is
// verified by running the app, not by unit tests.
type Renderer struct {
	face   *text.GoTextFace
	cellW  float64
	cellH  float64
	ascent float64

	defaultFG color.RGBA
	defaultBG color.RGBA
	cursor    color.RGBA
}

// NewRenderer builds a renderer using the bundled JetBrains Mono font (with
// programming ligatures) at the given pixel size.
func NewRenderer(sizePx float64) (*Renderer, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMono))
	if err != nil {
		return nil, err
	}
	face := &text.GoTextFace{Source: src, Size: sizePx}

	m := face.Metrics()
	cellH := m.HAscent + m.HDescent
	cellW := text.Advance("M", face)

	return &Renderer{
		face:      face,
		cellW:     cellW,
		cellH:     cellH,
		ascent:    m.HAscent,
		defaultFG: color.RGBA{0xcc, 0xcc, 0xcc, 0xff},
		defaultBG: color.RGBA{0x0a, 0x0a, 0x0a, 0xff},
		cursor:    color.RGBA{0xcc, 0xcc, 0xcc, 0x99},
	}, nil
}

// CellSize returns the pixel size of one character cell.
func (r *Renderer) CellSize() (w, h float64) { return r.cellW, r.cellH }

// GridSize returns how many whole cells fit in a w×h pixel area.
func (r *Renderer) GridSize(wPx, hPx int) (cols, rows int) {
	cols = int(float64(wPx) / r.cellW)
	rows = int(float64(hPx) / r.cellH)
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	return cols, rows
}

// Draw paints snap into dst. dst is expected to be a pane-sized (sub)image whose
// origin is the pane's top-left.
func (r *Renderer) Draw(dst *ebiten.Image, snap vte.Snapshot) {
	dst.Fill(r.defaultBG)

	for y := 0; y < snap.Rows; y++ {
		row := snap.Row(y)
		yPx := float32(float64(y) * r.cellH)

		for _, run := range SplitRuns(row) {
			fg, bg, hasBG := r.colors(run.Style)
			xPx := float32(float64(run.Col) * r.cellW)
			wPx := float32(float64(len([]rune(run.Text))) * r.cellW)

			if hasBG {
				vector.DrawFilledRect(dst, xPx, yPx, wPx, float32(r.cellH), bg, false)
			}

			op := &text.DrawOptions{}
			op.GeoM.Translate(float64(xPx), float64(yPx))
			op.ColorScale.ScaleWithColor(fg)
			text.Draw(dst, run.Text, r.face, op)
		}
	}

	r.drawCursor(dst, snap)
}

// drawCursor overlays a translucent block at the cursor cell.
func (r *Renderer) drawCursor(dst *ebiten.Image, snap vte.Snapshot) {
	x := float32(float64(snap.CurX) * r.cellW)
	y := float32(float64(snap.CurY) * r.cellH)
	vector.DrawFilledRect(dst, x, y, float32(r.cellW), float32(r.cellH), r.cursor, false)
}

// colors resolves a cell style into concrete fg/bg, honouring the reverse
// attribute. hasBG is false when the background is the terminal default (so the
// pane fill already covers it).
func (r *Renderer) colors(s vte.Style) (fg, bg color.RGBA, hasBG bool) {
	fg = r.resolve(s.FG, r.defaultFG)
	bg = r.defaultBG
	hasBG = s.BG.Kind != vte.ColorDefault
	if hasBG {
		bg = r.resolve(s.BG, r.defaultBG)
	}
	if s.Attrs&vte.AttrReverse != 0 {
		fg, bg = bg, fg
		hasBG = true
	}
	return fg, bg, hasBG
}

// resolve turns a vte.Color into an RGBA, using def for the default kind.
func (r *Renderer) resolve(c vte.Color, def color.RGBA) color.RGBA {
	switch c.Kind {
	case vte.ColorRGB:
		return color.RGBA{c.R, c.G, c.B, 0xff}
	case vte.ColorIndexed:
		return palette256(c.Idx)
	default:
		return def
	}
}
