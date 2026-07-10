package render

import "image/color"

// base16 is the ANSI palette for indices 0..15 (a light theme).
var base16 = [16]color.RGBA{
	{0xd1, 0xd1, 0xd1, 0xff}, // 0  black
	{0xb8, 0x1a, 0x6b, 0xff}, // 1  red
	{0x1e, 0x76, 0x3c, 0xff}, // 2  green
	{0x8d, 0x5b, 0x00, 0xff}, // 3  yellow
	{0x01, 0x54, 0x93, 0xff}, // 4  blue
	{0x75, 0x22, 0x8e, 0xff}, // 5  magenta
	{0x00, 0x74, 0x74, 0xff}, // 6  cyan
	{0x42, 0x42, 0x42, 0xff}, // 7  white
	{0x57, 0x60, 0x6a, 0xff}, // 8  bright black
	{0xb8, 0x1a, 0x6b, 0xff}, // 9  bright red
	{0x1e, 0x76, 0x3c, 0xff}, // 10 bright green
	{0x8d, 0x5b, 0x00, 0xff}, // 11 bright yellow
	{0x01, 0x54, 0x93, 0xff}, // 12 bright blue
	{0x75, 0x22, 0x8e, 0xff}, // 13 bright magenta
	{0x00, 0x74, 0x74, 0xff}, // 14 bright cyan
	{0x08, 0x51, 0x57, 0xff}, // 15 bright white
}

// palette256 maps a 256-color palette index to an RGBA value: 0..15 are the base
// ANSI colors, 16..231 the 6×6×6 color cube, 232..255 the grayscale ramp.
func palette256(idx uint8) color.RGBA {
	switch {
	case idx < 16:
		return base16[idx]
	case idx < 232:
		n := int(idx) - 16
		r := n / 36
		g := (n / 6) % 6
		b := n % 6
		return color.RGBA{cubeStep(r), cubeStep(g), cubeStep(b), 0xff}
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
