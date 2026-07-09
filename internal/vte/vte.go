// Package vte holds the ANSI/VTE state machine that consumes bytes from the
// PTY and updates the virtual character Grid. It knows nothing about the GPU.
package vte

// Style holds the visual attributes of a cell. It is a placeholder for now;
// foreground/background colors and an attribute bitmask (bold/italic/underline)
// arrive with the SGR handling at step 5/6. The zero value is the terminal
// default style. It must stay a comparable value type so run tokenization can
// group cells with ==.
type Style struct {
	// TODO(stage-2): FG, BG color, attrs bitmask.
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
