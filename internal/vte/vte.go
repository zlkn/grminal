// Package vte holds the ANSI/VTE state machine that consumes bytes from the
// PTY and updates the virtual character Grid. It knows nothing about the GPU.
package vte

// ColorKind selects how a Color is interpreted.
type ColorKind uint8

const (
	ColorDefault ColorKind = iota // terminal default (zero value)
	ColorIndexed                  // 256-color palette index in Idx
	ColorRGB                      // 24-bit truecolor in R,G,B
)

// Color is a foreground or background color. It is a comparable value type; the
// zero value means the terminal default color.
type Color struct {
	Kind    ColorKind
	Idx     uint8 // palette index when Kind == ColorIndexed
	R, G, B uint8 // components when Kind == ColorRGB
}

// AttrMask is a bitmask of boolean cell attributes.
type AttrMask uint8

const (
	AttrBold AttrMask = 1 << iota
	AttrItalic
	AttrUnderline
	AttrReverse
)

// Style holds the visual attributes of a cell. It must stay a comparable value
// type (no slices/pointers) so run tokenization can group cells with ==. The
// zero value is the terminal default style.
type Style struct {
	FG, BG Color
	Attrs  AttrMask
}

// Cell is a single terminal cell: a rune plus its style.
type Cell struct {
	Rune  rune
	Style Style
}

// blank is the value of an empty (default-style) cell.
var blank = Cell{Rune: ' '}

// IsBlank reports whether c is an empty cell in the default style — i.e. there
// is nothing to draw for it.
func (c Cell) IsBlank() bool { return c.Rune == ' ' && c.Style == Style{} }
