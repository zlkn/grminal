package render

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
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

// DrawTabBar paints the top tab strip: equal-width segments with centered
// labels, a thin baseline separator, and a colored underline under the active
// tab (GitHub style). It is drawn over the top strip that Draw left as
// background. Pixel glue — verified by running.
func (r *Renderer) DrawTabBar(dst *ebiten.Image, labels []string, active int) {
	if len(labels) == 0 || r.barH <= 0 {
		return
	}
	width := dst.Bounds().Dx()
	segs := tabSegments(width, len(labels))

	// Clear the strip and draw the baseline separator across the full width.
	vector.DrawFilledRect(dst, 0, 0, float32(width), float32(r.barH), r.defaultBG, false)
	sep := float32(math.Max(1, math.Round(r.scale)))
	vector.DrawFilledRect(dst, 0, float32(r.barH)-sep, float32(width), sep, r.tabMutedFG, false)

	underline := float32(math.Max(2, math.Round(2*r.scale)))
	labelTop := (r.barH - r.cellH) / 2

	x := 0
	for i, label := range labels {
		w := segs[i]
		fg := r.tabMutedFG
		if i == active {
			fg = r.defaultFG
		}

		// Centered label within the segment.
		lw := text.Advance(label, r.face)
		lx := float64(x) + (float64(w)-lw)/2
		if lx < float64(x) {
			lx = float64(x)
		}
		op := &text.DrawOptions{}
		op.GeoM.Translate(lx, labelTop)
		op.ColorScale.ScaleWithColor(fg)
		text.Draw(dst, label, r.face, op)

		// Colored underline under the active tab.
		if i == active {
			vector.DrawFilledRect(dst, float32(x), float32(r.barH)-underline, float32(w), underline, r.tabUnderline, false)
		}
		x += w
	}
}

// opaque returns c with full alpha.
func opaque(c color.RGBA) color.RGBA { c.A = 0xff; return c }
