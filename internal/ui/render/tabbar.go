package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// The bar background matches the terminal background (inactive tabs blend in).
// The active tab is drawn as a filled rounded "pill" tinted toward the
// foreground (tabActiveTint), so it reads as a darker selected chip.
const (
	tabActiveTint = 0.15 // fraction of foreground blended over background for the pill
	tabPillGapX   = 0.45 // horizontal gap from each segment edge, in cells
	tabPillPadY   = 0.22 // vertical padding around the label, fraction of cell height
	tabPillRadius = 0.30 // corner radius, fraction of the pill height
)

// tabSegments splits width into n as-even-as-possible integer segments that sum
// exactly to width (cumulative rounding, no gaps). Returns nil for n <= 0.
func tabSegments(width, n int) []int {
	if n <= 0 {
		return nil
	}
	segs := make([]int, n)
	prev := 0
	for i := 0; i < n; i++ {
		pos := int(math.Round(float64(i+1) / float64(n) * float64(width)))
		segs[i] = pos - prev
		prev = pos
	}
	return segs
}

// TabBarHeight is the reserved height of the tab bar in physical pixels.
func (r *Renderer) TabBarHeight() float64 { return r.barH }

// DrawTabBar paints the top tab strip: the terminal background, equal-width
// segments with centered labels, and a darker filled rounded pill behind the
// active tab. It is drawn over the top strip that Draw left as background. Pixel
// glue — verified by running.
func (r *Renderer) DrawTabBar(dst *ebiten.Image, labels []string, active int) {
	if len(labels) == 0 || r.barH <= 0 {
		return
	}
	width := dst.Bounds().Dx()
	segs := tabSegments(width, len(labels))

	// Clear the strip with the terminal background so inactive tabs blend in.
	vector.DrawFilledRect(dst, 0, 0, float32(width), float32(r.barH), r.defaultBG, false)

	labelTop := (r.barH - r.cellH) / 2

	x := 0
	for i, label := range labels {
		w := segs[i]
		fg := r.tabMutedFG
		if i == active {
			fg = r.defaultFG
			// Filled pill behind the active tab (drawn first, under the label).
			// Skipped when there is only one tab — then just the label shows.
			if len(labels) > 1 {
				r.drawActivePill(dst, x, w, labelTop)
			}
		}

		// Centered label within the segment, on top of any pill.
		lw := text.Advance(label, r.face)
		lx := float64(x) + (float64(w)-lw)/2
		if lx < float64(x) {
			lx = float64(x)
		}
		r.drawString(dst, label, lx, labelTop, fg)

		x += w
	}
}

// drawActivePill paints the rounded "selected" background of the active tab in
// segment [x, x+w), sized around the label row at labelTop.
func (r *Renderer) drawActivePill(dst *ebiten.Image, x, w int, labelTop float64) {
	gapX := tabPillGapX * r.cellW
	padY := tabPillPadY * r.cellH
	px := float32(float64(x) + gapX)
	pw := float32(float64(w) - 2*gapX)
	py := float32(labelTop - padY)
	ph := float32(r.cellH + 2*padY)
	if pw <= 0 || ph <= 0 {
		return
	}
	drawRoundedRect(dst, px, py, pw, ph, ph*float32(tabPillRadius), r.tabActiveBG)
}

// drawRoundedRect fills a rectangle with rounded corners using axis-aligned rects
// for the straight regions and antialiased circles for the corners. Drawn with
// immediate primitives so it reliably lands under anything drawn afterwards.
func drawRoundedRect(dst *ebiten.Image, x, y, w, h, radius float32, clr color.Color) {
	if radius > w/2 {
		radius = w / 2
	}
	if radius > h/2 {
		radius = h / 2
	}
	// A full-height centre band plus the left/right edges between the corners.
	vector.DrawFilledRect(dst, x+radius, y, w-2*radius, h, clr, false)
	vector.DrawFilledRect(dst, x, y+radius, radius, h-2*radius, clr, false)
	vector.DrawFilledRect(dst, x+w-radius, y+radius, radius, h-2*radius, clr, false)
	// Rounded corners.
	vector.DrawFilledCircle(dst, x+radius, y+radius, radius, clr, true)
	vector.DrawFilledCircle(dst, x+w-radius, y+radius, radius, clr, true)
	vector.DrawFilledCircle(dst, x+radius, y+h-radius, radius, clr, true)
	vector.DrawFilledCircle(dst, x+w-radius, y+h-radius, radius, clr, true)
}

// blendRGBA linearly mixes a and b by t in [0,1], returning an opaque colour.
func blendRGBA(a, b color.RGBA, t float64) color.RGBA {
	if t < 0 {
		t = 0
	} else if t > 1 {
		t = 1
	}
	lerp := func(x, y uint8) uint8 { return uint8(float64(x)*(1-t) + float64(y)*t + 0.5) }
	return color.RGBA{lerp(a.R, b.R), lerp(a.G, b.G), lerp(a.B, b.B), 0xff}
}
