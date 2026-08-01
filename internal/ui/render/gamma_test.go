package render

import (
	"math"
	"testing"
)

// invGammaFor must never hand the shader an exponent that would erase text. A
// bad gamma can only reach it from a caller-built Config (the config loader
// validates the parsed range), so the contract is "degrade to no correction",
// not "panic".
func TestInvGammaFor(t *testing.T) {
	cases := []struct {
		name  string
		gamma float64
		want  float32
	}{
		{"neutral", 1, 1},
		{"default", 1.4, 1 / 1.4},
		{"lightening", 0.5, 2},
		{"zero disables", 0, 1},
		{"negative disables", -2, 1},
		{"NaN disables", math.NaN(), 1},
		{"Inf disables", math.Inf(1), 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := invGammaFor(c.gamma); got != c.want {
				t.Errorf("invGammaFor(%v) = %v, want %v", c.gamma, got, c.want)
			}
		})
	}
}

// A gamma of 1 must skip the shader entirely rather than compile one that
// happens to be a no-op: the plain DrawImage path stays the default, so an
// untouched config cannot regress on a driver that dislikes the shader.
func TestNewTextShaderNeutralIsNil(t *testing.T) {
	sh, inv := newTextShader(1)
	if sh != nil {
		t.Error("gamma 1 compiled a shader, want the plain DrawImage path")
	}
	if inv != 1 {
		t.Errorf("gamma 1 exponent = %v, want 1", inv)
	}
}
