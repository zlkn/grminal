package render

import (
	"bytes"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/yzolkin/go-vte/fonts"
	"github.com/yzolkin/go-vte/internal/config"
)

// inkPositions returns the blit position of every inked glyph in a run, using the
// renderer's real placement pass. Only layout runs here, no drawing, so it needs
// no Ebiten game loop.
func inkPositions(r *Renderer, run Run) []float64 {
	var xs []float64
	r.eachGlyph(run, 0, func(_ *ebiten.Image, x, _ float64) {
		xs = append(xs, x)
	})
	return xs
}

// Glyph ink must land on whole pixels. text/v2 bakes a glyph's sub-pixel phase
// into the rasterized image and returns an integral offset to blit it at, so a
// fractional destination re-samples the texture off-grid and smears thin strokes —
// worst on ligatures, whose whole shape is one wide glyph of diagonal strokes.
//
// The bug this guards: placement added the glyph's offset from its *fractional*
// pen origin (OriginX = column × the font's true advance) to an integral cell
// origin, so every glyph past a run's first column landed off-grid.
func TestGlyphInkLandsOnWholePixels(t *testing.T) {
	for _, size := range []float64{14, 16, 22, 24, 32} {
		cfg := config.Default()
		cfg.FontSize = size
		r, err := NewRenderer(cfg, 1)
		if err != nil {
			t.Fatal(err)
		}
		for _, text := range []string{"x != y", "if a --> b", "fn(x) => x >= 1", "a===b!==c"} {
			for _, col := range []int{0, 1, 7} {
				for _, x := range inkPositions(r, Run{Col: col, Text: text}) {
					if x != math.Trunc(x) {
						t.Errorf("size=%.0f col=%d %q: glyph ink at x=%.3f, want a whole pixel",
							size, col, text, x)
					}
				}
			}
		}
	}
}

// A ligature must stay pinned to the cells it occupies no matter how far along the
// row it sits. The font's true advance is narrower than the (whole-pixel) cell, so
// placing glyphs at their natural advances would let ink drift left without bound —
// half a cell by column 25 — and ligatures would visibly detach from the grid.
// Each cell is therefore re-anchored, leaving only sub-pixel rounding: at most 1px,
// and never accumulating.
//
// The tolerance cannot be zero: with cells wider than the advance, ink cannot be
// both on whole pixels and byte-identical at every column (see
// TestGlyphInkLandsOnWholePixels for the sharpness half of the trade-off).
func TestLigatureInkStaysAlignedToItsCells(t *testing.T) {
	r, err := NewRenderer(config.Default(), 1)
	if err != nil {
		t.Fatal(err)
	}
	cellW, _ := r.CellSize()

	for _, lig := range []string{"!=", "->", "=>", ">=", "===", "<->"} {
		// The same ligature preceded by a growing run of filler cells. Filler ink is
		// dropped by comparing only the trailing positions.
		base := inkPositions(r, Run{Text: lig})
		for _, shift := range []int{1, 2, 4, 9, 25} {
			padded := inkPositions(r, Run{Text: repeat("x", shift) + lig})
			if len(padded) < len(base) {
				t.Fatalf("%q shifted by %d: got %d inked glyphs, want at least %d",
					lig, shift, len(padded), len(base))
			}
			tail := padded[len(padded)-len(base):]
			for i := range base {
				want := base[i] + float64(shift)*cellW
				if off := tail[i] - want; math.Abs(off) > 1 {
					t.Errorf("%q shifted by %d cells: ink[%d] at %.3f, want %.3f±1 (%.3f px off its cells)",
						lig, shift, i, tail[i], want, off)
				}
			}
		}
	}
}

// With font_snap on, a size that was snapped must have a cell exactly equal to the
// font's advance. That is the property that makes text uniformly crisp: every
// column then starts on a whole pixel, so text/v2 rasterizes each character from
// the same bitmap wherever it lands, instead of a softer one whose sub-pixel phase
// depends on the column. A size with no candidate close enough is left alone, which
// must be visible as an unchanged size rather than a silent large resize.
func TestFontSnapGivesWholePixelAdvance(t *testing.T) {
	snapped := 0
	for _, size := range []float64{12, 13, 14, 15, 16, 17, 18, 20, 22, 24, 28, 32} {
		for _, scale := range []float64{1, 1.25, 1.5, 2} {
			cfg := config.Default()
			cfg.FontSize, cfg.FontSnap = size, true
			r, err := NewRenderer(cfg, scale)
			if err != nil {
				t.Fatal(err)
			}
			cellW, _ := r.CellSize()
			adv := text.Advance("M", r.face)
			want := size * scale

			if r.size == want { // not snapped: the fallback, drift and all
				continue
			}
			snapped++
			if adv != cellW {
				t.Errorf("size=%.0f scale=%.2f: snapped to %.2fpx but advance %.4f != cell %.0f",
					size, scale, r.size, adv, cellW)
			}
			if d := math.Abs(r.size - want); d > 2 || d > want*0.1 {
				t.Errorf("size=%.0f scale=%.2f: snapped %.2fpx -> %.2fpx, moved %.2fpx (limit: 2px and 10%%)",
					size, scale, want, r.size, d)
			}
		}
	}
	if snapped == 0 {
		t.Error("nothing snapped across the whole matrix; the snap is not being applied")
	}
	t.Logf("%d of 48 size/scale combinations snapped", snapped)
}

// The sizes worth being sure about: the configured default, and a HiDPI scale of it.
func TestFontSnapKnownSizes(t *testing.T) {
	cases := []struct {
		size, scale, want float64
	}{
		{16, 1, 15},   // the default: 9.594px advance -> exactly 9
		{16, 1.5, 25}, // HiDPI: 24px -> 25 (15px advance)
		{22, 1, 20},   // 13.203 -> 12
		{15, 1, 15},   // already whole; left alone
		{20, 1, 20},   // already whole
	}
	for _, tc := range cases {
		cfg := config.Default()
		cfg.FontSize, cfg.FontSnap = tc.size, true
		r, err := NewRenderer(cfg, tc.scale)
		if err != nil {
			t.Fatal(err)
		}
		if r.size != tc.want {
			t.Errorf("font_size=%.0f scale=%.2f -> %.2fpx, want %.2fpx", tc.size, tc.scale, r.size, tc.want)
		}
	}
}

// With font_snap off the requested size is honoured exactly, drift and all.
func TestFontSnapOffKeepsRequestedSize(t *testing.T) {
	cfg := config.Default()
	cfg.FontSize, cfg.FontSnap = 16, false
	r, err := NewRenderer(cfg, 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.size != 16 {
		t.Errorf("size = %.2f with font_snap off, want exactly 16", r.size)
	}
	cellW, _ := r.CellSize()
	if adv := text.Advance("M", r.face); adv == cellW {
		t.Skip("this font has no drift at 16px; nothing to distinguish")
	}
}

// snapToWholeAdvance must land on a whole-pixel advance, or leave the size alone.
func TestSnapToWholeAdvance(t *testing.T) {
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMono))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []float64{9, 12, 14.5, 16, 17.5, 21, 24, 31, 48} {
		got := snapToWholeAdvance(src, want)
		adv := text.Advance("M", &text.GoTextFace{Source: src, Size: got})
		snapped := adv == math.Trunc(adv)
		if !snapped && got != want {
			t.Errorf("snapToWholeAdvance(%.1f) = %.2f: neither whole-advance (%.4f) nor the input",
				want, got, adv)
		}
		if d := math.Abs(got - want); d > 2 || d > want*0.1 {
			t.Errorf("snapToWholeAdvance(%.1f) = %.2f: moved %.2fpx (limit: 2px and 10%%)", want, got, d)
		}
		t.Logf("%.1f -> %.2f (advance %.4f, whole=%t)", want, got, adv, snapped)
	}
}

func repeat(s string, n int) string {
	out := ""
	for i := 0; i < n; i++ {
		out += s
	}
	return out
}
