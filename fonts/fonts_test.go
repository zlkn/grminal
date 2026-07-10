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

// nerdIcons is a representative sample of Nerd Font glyphs drawn from several of
// its icon ranges. Plain (non-Nerd) fonts lack these, so they render as boxes.
var nerdIcons = []struct {
	r    rune
	name string
}{
	{0xF015, "nf-fa-home"},
	{0xF07B, "nf-fa-folder"},
	{0xF121, "nf-fa-code"},
	{0xE709, "nf-dev-linux"},
	{0xE62B, "nf-seti-config"},
	{0xF300, "nf-linux-tux"},
}

// TestNerdFontIcons is the executable spec for the requirement "the bundled font
// must be a Nerd Font": each sampled icon must resolve to a real glyph. It fails
// against a plain font, so swapping in the true JetBrains Mono Nerd Font is what
// turns it green.
func TestNerdFontIcons(t *testing.T) {
	f := parse(t)
	var b sfnt.Buffer
	for _, ic := range nerdIcons {
		gi, err := f.GlyphIndex(&b, ic.r)
		if err != nil {
			t.Errorf("GlyphIndex(%s U+%04X): %v", ic.name, ic.r, err)
			continue
		}
		if gi == 0 {
			t.Errorf("missing Nerd Font icon %s (U+%04X): bundled font is not a Nerd Font build", ic.name, ic.r)
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
