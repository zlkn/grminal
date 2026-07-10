package render

import (
	"bytes"
	"testing"

	"github.com/hajimehoshi/ebiten/v2/text/v2"

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
