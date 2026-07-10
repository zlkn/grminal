package render

import (
	"bytes"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/yzolkin/go-vte/fonts"
	"github.com/yzolkin/go-vte/internal/vte"
)

// iconFillRatio is the fraction of the cell height a Nerd Font icon is scaled to
// fill. >1-cell-wide icons are allowed to overflow horizontally (centered),
// which matches how kitty/wezterm render symbols. Tune to taste.
const iconFillRatio = 0.85

// Renderer draws terminal grids to an ebiten target. It owns the font face and
// the derived cell metrics. It is deliberately thin: all layout decisions live
// in the tested SplitRuns tokenizer; this file only issues GPU draw calls and is
// verified by running the app, not by unit tests.
type Renderer struct {
	face   *text.GoTextFace
	sf     *sfnt.Font    // same font parsed for glyph geometry (icon scaling)
	buf    sfnt.Buffer   // scratch for sf lookups (single-goroutine use)
	icons  map[rune]iconGeom
	size   float64
	cellW  float64
	cellH  float64
	ascent float64

	defaultFG color.RGBA
	defaultBG color.RGBA
	cursor    color.RGBA
}

// iconGeom is the cached transform for centering and scaling one icon glyph.
type iconGeom struct {
	scale        float64
	inkCX, inkCY float64 // ink centre in the base-face draw coordinate space
}

// NewRenderer builds a renderer using the bundled JetBrains Mono Nerd Font at
// the given pixel size.
func NewRenderer(sizePx float64) (*Renderer, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMono))
	if err != nil {
		return nil, err
	}
	sf, err := sfnt.Parse(fonts.JetBrainsMono)
	if err != nil {
		return nil, err
	}
	face := &text.GoTextFace{Source: src, Size: sizePx}

	m := face.Metrics()
	return &Renderer{
		face:      face,
		sf:        sf,
		icons:     make(map[rune]iconGeom),
		size:      sizePx,
		cellW:     text.Advance("M", face),
		cellH:     m.HAscent + m.HDescent,
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
		topY := float64(y) * r.cellH

		for _, run := range SplitRuns(row) {
			fg, bg, hasBG := r.colors(run.Style)
			xPx := float32(float64(run.Col) * r.cellW)

			if hasBG {
				wPx := float32(float64(len([]rune(run.Text))) * r.cellW)
				vector.DrawFilledRect(dst, xPx, float32(topY), wPx, float32(r.cellH), bg, false)
			}

			if run.Icon {
				col := run.Col
				for _, ru := range run.Text {
					r.drawIcon(dst, ru, col, topY, fg)
					col++
				}
				continue
			}

			op := &text.DrawOptions{}
			op.GeoM.Translate(float64(xPx), topY)
			op.ColorScale.ScaleWithColor(fg)
			text.Draw(dst, run.Text, r.face, op)
		}
	}

	r.drawCursor(dst, snap)
}

// drawIcon draws a single Nerd Font icon scaled to fill the cell height and
// centered in the cell (overflowing horizontally if needed).
func (r *Renderer) drawIcon(dst *ebiten.Image, ru rune, col int, topY float64, fg color.RGBA) {
	g := r.iconMetrics(ru)
	cx := float64(col)*r.cellW + r.cellW/2
	cy := topY + r.cellH/2

	op := &text.DrawOptions{}
	op.GeoM.Translate(-g.inkCX, -g.inkCY) // move ink centre to origin
	op.GeoM.Scale(g.scale, g.scale)       // scale about it
	op.GeoM.Translate(cx, cy)             // place at the cell centre
	op.ColorScale.ScaleWithColor(fg)
	text.Draw(dst, string(ru), r.face, op)
}

// iconMetrics returns (and caches) the scale and ink centre for an icon glyph.
func (r *Renderer) iconMetrics(ru rune) iconGeom {
	if g, ok := r.icons[ru]; ok {
		return g
	}
	g := iconGeom{scale: 1}
	ppem := fixed.Int26_6(r.size * 64)
	if gi, err := r.sf.GlyphIndex(&r.buf, ru); err == nil && gi != 0 {
		if b, _, err := r.sf.GlyphBounds(&r.buf, gi, ppem, font.HintingNone); err == nil {
			inkH := f26(b.Max.Y - b.Min.Y)
			if inkH > 0 {
				g.scale = clampF(r.cellH*iconFillRatio/inkH, 0.5, 3)
			}
			g.inkCX = f26(b.Min.X+b.Max.X) / 2
			g.inkCY = r.ascent + f26(b.Min.Y+b.Max.Y)/2
		}
	}
	r.icons[ru] = g
	return g
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

// f26 converts a 26.6 fixed-point value to float64.
func f26(v fixed.Int26_6) float64 { return float64(v) / 64 }

func clampF(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
