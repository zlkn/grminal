package render

import (
	"bytes"
	"image/color"
	"log"
	"math"
	"unicode/utf8"

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
	scale         float64

	// Cursor presentation from the config. cursorShape/cursorBlink are the
	// fallbacks used when the running app has not selected them via DECSCUSR.
	cursorShape vte.CursorShape
	cursorBlink bool

	padL, padR, padT, padB float64 // inset in physical px

	barH        float64    // reserved tab-bar height in physical px
	tabActiveBG color.RGBA // filled "selected" pill behind the active tab (darker than bg)
	tabMutedFG  color.RGBA

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
	if cfg.FontSnap {
		if snapped := snapToWholeAdvance(src, sizePx); snapped != sizePx {
			if config.Debug {
				log.Printf("[render] font size %.2fpx -> %.2fpx so the advance is a whole pixel (font_snap)",
					sizePx, snapped)
			}
			sizePx = snapped
		}
	}
	face := &text.GoTextFace{Source: src, Size: sizePx}

	m := face.Metrics()
	// Snap cell size to whole pixels so every column/row lands on the pixel grid
	// — sub-pixel cell origins make glyphs rasterize blurry.
	cellW := math.Ceil(text.Advance("M", face))
	cellH := math.Ceil(m.HAscent + m.HDescent)

	// The cursor is drawn translucent so the glyph beneath stays readable. Ebiten
	// blends with premultiplied alpha, so the components have to be scaled too —
	// setting A alone would draw an over-bright cursor.
	cursor := premultiply(cfg.Cursor, cfg.CursorOpacity)

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
		cursorShape:   shapeFromStyle(cfg.CursorStyle),
		cursorBlink:   cfg.CursorBlink,
		scale:         scale,
		padL:          padPx(cfg.PaddingLeft, scale),
		padR:          padPx(cfg.PaddingRight, scale),
		padT:          padPx(cfg.PaddingTop, scale),
		padB:          padPx(cfg.PaddingBottom, scale),
		barH:          math.Ceil(cellH * 2.0),
		tabActiveBG:   blendRGBA(cfg.Background, cfg.Foreground, tabActiveTint),
		tabMutedFG:    cfg.Palette[8],
		defaultFG:     cfg.Foreground,
		defaultBG:     cfg.Background,
		cursor:        cursor,
	}, nil
}

// snapToWholeAdvance returns a pixel size near want whose character advance is a
// whole number of pixels, or want itself when there is none within snapMaxDelta.
//
// Why: the cell grid is whole pixels, so a fractional advance leaves every column
// starting at a different sub-pixel offset (JetBrains Mono advances 0.6 em: at 16px
// that is 9.594 in a 10px cell, drifting 0.406 per column). text/v2 bakes that
// offset into the glyph bitmap it rasterizes, so the *same* character is drawn from
// a different, softer bitmap depending on which column it lands in — measurably so:
// `!=` rasterizes identically in only 2 of 8 consecutive columns at 16px, against
// 8 of 8 once the advance is whole. Ligatures suffer most, being a single wide
// glyph of thin diagonal strokes. A whole-pixel advance also makes the cell exactly
// the advance, so a ligature's ink spans precisely the cells it belongs to.
//
// Only integer sizes are tested: the rasterizer works at whole-pixel ppem (the
// advance changes only when ceil(size) does), so they are the canonical
// representatives — within a plateau the bitmap is identical and only the
// line metrics creep.
func snapToWholeAdvance(src *text.GoTextFaceSource, want float64) float64 {
	// How far the size may move: a tenth of it, capped at 2px. Whole-pixel advances
	// repeat every 5px of size for a 0.6 em advance, so a size landing midway
	// between two of them has no candidate close enough — better to honour what was
	// asked for and accept the softer rendering than to resize text by 15%.
	limit := math.Min(2, want*0.1)
	best, bestDelta := want, math.Inf(1)
	for s := math.Ceil(want - limit); s <= want+limit; s++ {
		if s < 4 { // below this text is unreadable anyway
			continue
		}
		a := text.Advance("M", &text.GoTextFace{Source: src, Size: s})
		if a < 1 || a != math.Trunc(a) {
			continue
		}
		if d := math.Abs(s - want); d < bestDelta {
			best, bestDelta = s, d
		}
	}
	return best
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

// CellAt maps a physical-pixel point to the 0-based grid cell under it. ok is
// false when the point is left of or above the content area (i.e. in the left/
// top padding or the tab bar), so callers can ignore clicks outside the grid.
// The returned col/row are not clamped to the grid's width/height.
func (r *Renderer) CellAt(px, py int) (col, row int, ok bool) {
	x := float64(px) - r.padL
	y := float64(py) - r.barH - r.padT
	if x < 0 || y < 0 {
		return 0, 0, false
	}
	return int(x / r.cellW), int(y / r.cellH), true
}

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
// origin is the pane's top-left. cur carries the window-level cursor state (focus
// and blink phase), which the grid does not know about.
func (r *Renderer) Draw(dst *ebiten.Image, snap vte.Snapshot, cur CursorState) {
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

			r.drawRunText(dst, run, topY, fg)
		}
	}

	r.drawCursor(dst, snap, cur)
}

// drawRunText draws a text run one shaped glyph at a time, snapping each glyph
// cluster to its cell origin. Shaping still runs over the whole run so OpenType
// ligatures (!=, ->) form, but horizontal placement comes from the cell grid,
// not the font's advances. A monospace face whose true advance is not a whole
// number of pixels (cellW is math.Ceil'd) otherwise drifts within the run, so
// text visibly jumps left/right when a background change (e.g. neovim's
// cursor-word highlight) splits a row into differently-anchored runs.
func (r *Renderer) drawRunText(dst *ebiten.Image, run Run, topY float64, fg color.RGBA) {
	r.eachGlyph(run, topY, func(img *ebiten.Image, x, y float64) {
		op := &ebiten.DrawImageOptions{}
		op.GeoM.Translate(x, y)
		op.ColorScale.ScaleWithColor(fg)
		dst.DrawImage(img, op)
	})
}

// eachGlyph shapes run and calls fn for every glyph that has ink, with the pixel
// position to blit it at. It is the placement pass of drawRunText, split out so
// tests can assert where the ink lands without a GPU (drawing needs a live Ebiten
// loop, layout does not) — and so they exercise the same code that draws.
func (r *Renderer) eachGlyph(run Run, topY float64, fn func(img *ebiten.Image, x, y float64)) {
	base := r.padL + float64(run.Col)*r.cellW
	col := 0
	prevByte := -1
	for _, g := range text.AppendGlyphs(nil, run.Text, r.face, nil) {
		// Advance the column counter to this glyph's starting rune. A ligature
		// cluster spans several runes but yields one glyph anchored at its first
		// cell; combining glyphs share a start index and stack in one cell.
		if g.StartIndexInBytes != prevByte {
			if prevByte >= 0 {
				col += utf8.RuneCountInString(run.Text[prevByte:g.StartIndexInBytes])
			}
			prevByte = g.StartIndexInBytes
		}
		if g.Image == nil {
			continue
		}
		fn(g.Image, r.glyphX(base, col, g), math.Round(topY+g.Y))
	}
}

// glyphX is the horizontal blit position for glyph g whose cluster starts at
// column col of a run based at base px. It anchors the cluster to the cell grid
// (base + col*cellW) and adds the glyph image's offset from its own pen origin,
// which is what keeps a ligature's ink — one wide glyph hung off the cluster's
// last cell by a large negative bearing — spanning the cells it belongs to.
//
// The result is rounded to a whole pixel. text/v2 bakes a glyph's sub-pixel phase
// into the rasterized image and hands back an integral g.X to blit it at, so a
// fractional destination re-samples the texture off-grid and smears the thin
// diagonal strokes ligatures are built from. OriginX is the fractional pen
// position, so without rounding every glyph past the run's first column lands
// off-grid.
func (r *Renderer) glyphX(base float64, col int, g text.Glyph) float64 {
	return math.Round(base + float64(col)*r.cellW + (g.X - g.OriginX))
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

// drawCursor overlays the cursor on its cell, unless it is hidden, blinked off,
// or the snapshot is a scrollback view (which reports it invisible).
//
// An unfocused window draws a hollow outline whatever the shape is, so which
// window owns the keyboard is obvious at a glance (xterm/kitty behaviour).
func (r *Renderer) drawCursor(dst *ebiten.Image, snap vte.Snapshot, cur CursorState) {
	if !snap.CursorVisible {
		return
	}
	cellX := r.padL + float64(snap.CurX)*r.cellW
	cellY := r.barH + r.padT + float64(snap.CurY)*r.cellH

	if !cur.Focused {
		// A hollow cursor never blinks: nothing is typing into it.
		w := math.Max(1, math.Round(r.scale))
		vector.StrokeRect(dst, float32(cellX+w/2), float32(cellY+w/2),
			float32(r.cellW-w), float32(r.cellH-w), float32(w), r.cursor, false)
		return
	}
	if r.CursorBlinking(snap.CursorBlink) && !cur.BlinkOn {
		return
	}
	x, y, w, h := cursorRect(r.shapeFor(snap.CursorShape), cellX, cellY, r.cellW, r.cellH)
	vector.DrawFilledRect(dst, float32(x), float32(y), float32(w), float32(h), r.cursor, false)
}

// CursorState is the part of the cursor's presentation that lives outside the
// grid: whether the window has keyboard focus, and where the blink cycle is. The
// app owns both (they are per-window, not per-terminal) and passes them in.
type CursorState struct {
	Focused bool
	BlinkOn bool
}

// CursorBlinking resolves whether the cursor should blink: the app's DECSCUSR
// request wins, otherwise the configured default applies.
func (r *Renderer) CursorBlinking(b vte.CursorBlink) bool {
	switch b {
	case vte.CursorBlinkOn:
		return true
	case vte.CursorBlinkOff:
		return false
	default:
		return r.cursorBlink
	}
}

// shapeFor resolves the shape to draw: the app's DECSCUSR request wins, otherwise
// the configured cursor_style applies.
func (r *Renderer) shapeFor(s vte.CursorShape) vte.CursorShape {
	if s == vte.CursorShapeDefault {
		return r.cursorShape
	}
	return s
}

// cursorRect returns the rectangle to fill for a cursor of the given shape over a
// cell of cellW×cellH at (cellX, cellY): a block covers the cell, an underline
// sits on its bottom edge, a bar on its left edge. Thicknesses scale with the cell
// so the thin shapes stay visible on HiDPI, and are at least one pixel so they
// never vanish. Pure, so the geometry is unit-tested without a GPU.
func cursorRect(shape vte.CursorShape, cellX, cellY, cellW, cellH float64) (x, y, w, h float64) {
	switch shape {
	case vte.CursorShapeUnderline:
		t := math.Max(1, math.Round(cellH*underlineRatio))
		return cellX, cellY + cellH - t, cellW, t
	case vte.CursorShapeBar:
		t := math.Max(1, math.Round(cellW*barRatio))
		return cellX, cellY, t, cellH
	default: // block
		return cellX, cellY, cellW, cellH
	}
}

// Thickness of the thin cursor shapes as a fraction of the cell.
const (
	underlineRatio = 0.12
	barRatio       = 0.15
)

// shapeFromStyle maps a configured cursor_style name to a shape. config validates
// the name, so an unknown one can only come from a caller-built Config; it falls
// back to a block rather than drawing nothing.
func shapeFromStyle(name string) vte.CursorShape {
	switch name {
	case "beam":
		return vte.CursorShapeBar
	case "underline":
		return vte.CursorShapeUnderline
	default:
		return vte.CursorShapeBlock
	}
}

// premultiply applies an alpha in [0,1] to c, scaling the colour components with
// it. Go's color.RGBA (and hence Ebiten) is alpha-premultiplied, so setting the
// alpha channel alone would leave the components too bright to blend correctly.
func premultiply(c color.RGBA, alpha float64) color.RGBA {
	alpha = clampF(alpha, 0, 1)
	scale := func(v uint8) uint8 { return uint8(float64(v)*alpha + 0.5) }
	return color.RGBA{R: scale(c.R), G: scale(c.G), B: scale(c.B), A: scale(0xff)}
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
