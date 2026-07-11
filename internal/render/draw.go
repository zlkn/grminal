package render

import (
	"bytes"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/yzolkin/go-vte/fonts"
	"github.com/yzolkin/go-vte/internal/config"
	"github.com/yzolkin/go-vte/internal/vte"
)

// Renderer draws terminal grids to an ebiten target. It owns the font face and
// the derived cell metrics. It is deliberately thin: all layout decisions live
// in the tested SplitRuns tokenizer; this file only issues GPU draw calls and is
// verified by running the app, not by unit tests.
type Renderer struct {
	face   *text.GoTextFace
	sf     *sfnt.Font  // same font parsed for glyph geometry (icon scaling)
	buf    sfnt.Buffer // scratch for sf lookups (single-goroutine use)
	icons  map[rune]iconGeom
	size   float64
	cellW  float64
	cellH  float64
	ascent float64

	palette       [16]color.RGBA
	iconFillRatio float64
	cursorStyle   string
	scale         float64

	padL, padR, padT, padB float64 // inset in physical px

	barH         float64 // reserved tab-bar height in physical px
	tabUnderline color.RGBA
	tabMutedFG   color.RGBA

	defaultFG color.RGBA
	defaultBG color.RGBA
	cursor    color.RGBA
}

// iconGeom is the cached transform for centering and scaling one icon glyph.
type iconGeom struct {
	scale        float64
	inkCX, inkCY float64 // ink centre in the base-face draw coordinate space
}

// NewRenderer builds a renderer using the bundled JetBrains Mono Nerd Font,
// themed from cfg. scale is the display device scale factor; the font size and
// padding (both in logical px in cfg) are multiplied by it so everything is
// rasterized at native resolution.
func NewRenderer(cfg config.Config, scale float64) (*Renderer, error) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMono))
	if err != nil {
		return nil, err
	}
	sf, err := sfnt.Parse(fonts.JetBrainsMono)
	if err != nil {
		return nil, err
	}
	sizePx := cfg.FontSize * scale
	face := &text.GoTextFace{Source: src, Size: sizePx}

	m := face.Metrics()
	// Snap cell size to whole pixels so every column/row lands on the pixel grid
	// — sub-pixel cell origins make glyphs rasterize blurry.
	cellW := math.Ceil(text.Advance("M", face))
	cellH := math.Ceil(m.HAscent + m.HDescent)

	// The cursor is drawn as a translucent block so the glyph beneath shows.
	cursor := cfg.Cursor
	cursor.A = 0x99

	return &Renderer{
		face:          face,
		sf:            sf,
		icons:         make(map[rune]iconGeom),
		size:          sizePx,
		cellW:         cellW,
		cellH:         cellH,
		ascent:        m.HAscent,
		palette:       cfg.Palette,
		iconFillRatio: cfg.IconFillRatio,
		cursorStyle:   cfg.CursorStyle,
		scale:         scale,
		padL:          padPx(cfg.PaddingLeft, scale),
		padR:          padPx(cfg.PaddingRight, scale),
		padT:          padPx(cfg.PaddingTop, scale),
		padB:          padPx(cfg.PaddingBottom, scale),
		barH:          math.Ceil(cellH * 1.4),
		tabUnderline:  opaque(cfg.Cursor),
		tabMutedFG:    cfg.Palette[8],
		defaultFG:     cfg.Foreground,
		defaultBG:     cfg.Background,
		cursor:        cursor,
	}, nil
}

// padPx converts a logical-pixel padding to physical px (>= 0).
func padPx(logical int, scale float64) float64 {
	if logical <= 0 {
		return 0
	}
	return math.Round(float64(logical) * scale)
}

// CellSize returns the pixel size of one character cell.
func (r *Renderer) CellSize() (w, h float64) { return r.cellW, r.cellH }

// GridSize returns how many whole cells fit in a w×h pixel area, after removing
// the padding on each side.
func (r *Renderer) GridSize(wPx, hPx int) (cols, rows int) {
	usableW := float64(wPx) - r.padL - r.padR
	usableH := float64(hPx) - r.padT - r.padB - r.barH
	cols = int(usableW / r.cellW)
	rows = int(usableH / r.cellH)
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
		topY := r.barH + r.padT + float64(y)*r.cellH

		for _, run := range SplitRuns(row) {
			fg, bg, hasBG := r.colors(run.Style)
			xPx := float32(r.padL + float64(run.Col)*r.cellW)

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
	cx := r.padL + float64(col)*r.cellW + r.cellW/2
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
				g.scale = clampF(r.cellH*r.iconFillRatio/inkH, 0.5, 3)
			}
			g.inkCX = f26(b.Min.X+b.Max.X) / 2
			g.inkCY = r.ascent + f26(b.Min.Y+b.Max.Y)/2
		}
	}
	r.icons[ru] = g
	return g
}

// drawCursor overlays a translucent block at the cursor cell, unless hidden.
func (r *Renderer) drawCursor(dst *ebiten.Image, snap vte.Snapshot) {
	if !snap.CursorVisible {
		return
	}
	x := float32(r.padL + float64(snap.CurX)*r.cellW)
	y := float32(r.barH + r.padT + float64(snap.CurY)*r.cellH)
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
		return r.palette256(c.Idx)
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
