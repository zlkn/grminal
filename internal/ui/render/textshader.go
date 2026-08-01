package render

import (
	"log"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/yzolkin/go-vte/internal/config"
)

// textShaderSrc reshapes a glyph's antialiasing coverage before it is blended.
//
// text/v2 hands back glyph images as premultiplied greyscale (r = g = b = a =
// coverage), and the plain DrawImage path multiplies that by the ColorScale and
// blends it straight into an sRGB destination. Coverage is linear, sRGB is not,
// so a half-covered pixel lands visually far short of half a stroke: it reads as
// haze around the glyph instead of as part of it. Ligatures suffer most — their
// thin horizontal bars are ~2px thick and straddle the pixel grid, so *every*
// row of the bar is partial coverage and the whole shape turns grey.
//
// Applying cov^(1/gamma) pulls those partial pixels toward ink. The tint still
// arrives in the vertex colour (`color`), exactly as ColorScale delivers it on
// the DrawImage path, so at InvGamma = 1 this shader is a no-op reimplementation
// of that path — which is what TestTextGammaIdentity pins down. Keeping the
// colour in the vertices rather than a uniform also preserves batching: the only
// uniform is constant for the whole frame.
const textShaderSrc = `//kage:unit pixels

package main

var InvGamma float

func Fragment(dstPos vec4, srcPos vec2, color vec4) vec4 {
	cov := imageSrc0At(srcPos).a
	return color * pow(cov, InvGamma)
}
`

// newTextShader compiles the coverage-gamma shader for the given gamma, or
// returns nil when the correction is disabled (gamma 1) — callers then use the
// plain DrawImage path. A compile failure is not fatal: it degrades to that same
// path, because uncorrected text is a great deal better than no text.
func newTextShader(gamma float64) (*ebiten.Shader, float32) {
	invGamma := invGammaFor(gamma)
	if invGamma == 1 {
		return nil, 1
	}
	sh, err := ebiten.NewShader([]byte(textShaderSrc))
	if err != nil {
		log.Printf("[render] text gamma disabled: shader failed to compile: %v", err)
		return nil, 1
	}
	if config.Debug {
		log.Printf("[render] text gamma %.2f (coverage exponent %.4f)", gamma, invGamma)
	}
	return sh, invGamma
}

// invGammaFor turns a configured gamma into the exponent the shader applies. A
// non-positive or non-finite value (only reachable from a caller-built Config;
// config validates the parsed range) means "no correction" rather than a NaN
// exponent that would erase every glyph.
func invGammaFor(gamma float64) float32 {
	if math.IsNaN(gamma) || math.IsInf(gamma, 0) || gamma <= 0 {
		return 1
	}
	return float32(1 / gamma)
}
