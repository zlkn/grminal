package render

import "image/color"

// palette256 maps a 256-color palette index to an RGBA value: 0..15 come from
// the configured theme palette, 16..231 from the 6×6×6 color cube, 232..255
// from the grayscale ramp.
func (r *Renderer) palette256(idx uint8) color.RGBA {
	switch {
	case idx < 16:
		return r.palette[idx]
	case idx < 232:
		n := int(idx) - 16
		return color.RGBA{cubeStep(n / 36), cubeStep((n / 6) % 6), cubeStep(n % 6), 0xff}
	default:
		v := uint8(8 + (int(idx)-232)*10)
		return color.RGBA{v, v, v, 0xff}
	}
}

// cubeStep converts a 0..5 color-cube coordinate to an 8-bit intensity.
func cubeStep(v int) uint8 {
	if v == 0 {
		return 0
	}
	return uint8(55 + v*40)
}
