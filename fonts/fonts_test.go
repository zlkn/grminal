package fonts

import (
	"testing"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

// parse loads the embedded font or fails the test.
func parse(t *testing.T) *sfnt.Font {
	t.Helper()
	f, err := sfnt.Parse(JetBrainsMono)
	if err != nil {
		t.Fatalf("parse embedded font: %v", err)
	}
	return f
}

func TestEmbeddedFontParses(t *testing.T) {
	if len(JetBrainsMono) == 0 {
		t.Fatal("embedded font is empty")
	}
	f := parse(t)
	if n := f.NumGlyphs(); n <= 0 {
		t.Fatalf("NumGlyphs = %d, want > 0", n)
	}
}

// TestASCIICoverage guarantees every printable ASCII rune has a real glyph, so
// no ordinary character renders as a .notdef box.
func TestASCIICoverage(t *testing.T) {
	f := parse(t)
	var b sfnt.Buffer
	for r := rune(0x20); r <= 0x7e; r++ {
		gi, err := f.GlyphIndex(&b, r)
		if err != nil {
			t.Errorf("GlyphIndex(%q): %v", r, err)
			continue
		}
		if gi == 0 { // 0 == .notdef
			t.Errorf("no glyph for printable ASCII %q (U+%04X)", r, r)
		}
	}
}

// TestMonospaceAdvance enforces the terminal's core assumption: every ASCII
// glyph advances by the same width. A proportional font would break the grid.
func TestMonospaceAdvance(t *testing.T) {
	f := parse(t)
	var b sfnt.Buffer
	ppem := fixed.I(64)

	var want fixed.Int26_6 = -1
	var wantRune rune
	for r := rune(0x20); r <= 0x7e; r++ {
		gi, err := f.GlyphIndex(&b, r)
		if err != nil || gi == 0 {
			continue
		}
		adv, err := f.GlyphAdvance(&b, gi, ppem, font.HintingNone)
		if err != nil {
			t.Errorf("GlyphAdvance(%q): %v", r, err)
			continue
		}
		if want < 0 {
			want, wantRune = adv, r
			continue
		}
		if adv != want {
			t.Errorf("non-monospace: %q advance=%v, but %q advance=%v", r, adv, wantRune, want)
		}
	}
	if want < 0 {
		t.Fatal("measured no glyphs")
	}
}
