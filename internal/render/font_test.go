package render

import (
	"bytes"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"

	"github.com/yzolkin/go-vte/fonts"
)

func testFace(t *testing.T) *text.GoTextFace {
	t.Helper()
	src, err := text.NewGoTextFaceSource(bytes.NewReader(fonts.JetBrainsMono))
	if err != nil {
		t.Fatalf("load face source: %v", err)
	}
	return &text.GoTextFace{Source: src, Size: 16}
}

func gids(face *text.GoTextFace, s string) []uint32 {
	glyphs := text.AppendGlyphs(nil, s, face, nil)
	ids := make([]uint32, len(glyphs))
	for i, g := range glyphs {
		ids[i] = g.GID
	}
	return ids
}

func equalIDs(a, b []uint32) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestLigaturesApplied guards that the font + shaper substitute programming
// ligatures: the glyphs for "!=" must differ from the plain glyphs of "!"
// followed by "=". If ligatures regress (wrong font, shaping off), this fails.
func TestLigaturesApplied(t *testing.T) {
	face := testFace(t)
	for _, seq := range []string{"!=", "->", "=>"} {
		lig := gids(face, seq)
		plain := append(gids(face, seq[:1]), gids(face, seq[1:])...)
		if equalIDs(lig, plain) {
			t.Errorf("no ligature for %q: glyphs %v == plain %v", seq, lig, plain)
		}
	}
}

// maxInkHeight shapes s and returns the tallest glyph ink height (font units).
func maxInkHeight(t *testing.T, face *text.GoTextFace, f *sfnt.Font, s string) fixed.Int26_6 {
	t.Helper()
	var b sfnt.Buffer
	ppem := fixed.I(1000)
	var maxH fixed.Int26_6
	for _, g := range text.AppendGlyphs(nil, s, face, nil) {
		bnd, _, err := f.GlyphBounds(&b, sfnt.GlyphIndex(g.GID), ppem, 0)
		if err != nil {
			continue
		}
		if h := bnd.Max.Y - bnd.Min.Y; h > maxH {
			maxH = h
		}
	}
	return maxH
}

// TestLigatureNotShrunk guards against the exact symptom of a mis-built font:
// ligatures rendered smaller than the characters they replace. Each ligature's
// tallest glyph must be at least 80% as tall as the tallest of its component
// characters. (The font is currently fine; this keeps a bad swap out of CI.)
func TestLigatureNotShrunk(t *testing.T) {
	face := testFace(t)
	f, err := sfnt.Parse(fonts.JetBrainsMono)
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ seq, a, b string }{
		{"!=", "!", "="},
		{"->", "-", ">"},
		{"=>", "=", ">"},
		{">=", ">", "="},
	}
	for _, c := range cases {
		lig := maxInkHeight(t, face, f, c.seq)
		plain := maxInkHeight(t, face, f, c.a)
		if h := maxInkHeight(t, face, f, c.b); h > plain {
			plain = h
		}
		if lig*100 < plain*80 {
			t.Errorf("ligature %q ink height %v < 80%% of component height %v (shrunken ligature)", c.seq, lig, plain)
		}
	}
}

// TestRendererCellMetrics checks the renderer builds and derives a sane,
// positive cell size from the font.
func TestRendererCellMetrics(t *testing.T) {
	r, err := NewRenderer(16)
	if err != nil {
		t.Fatalf("NewRenderer: %v", err)
	}
	w, h := r.CellSize()
	if w <= 0 || h <= 0 {
		t.Errorf("cell size = %vx%v, want positive", w, h)
	}
	cols, rows := r.GridSize(800, 600)
	if cols < 1 || rows < 1 {
		t.Errorf("grid size = %dx%d, want >= 1x1", cols, rows)
	}
}
