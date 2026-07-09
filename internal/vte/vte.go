// Package vte holds the ANSI/VTE state machine that consumes bytes from the
// PTY and updates the virtual character Grid. It knows nothing about the GPU.
package vte

// Cell is a single terminal cell: a rune plus (later) style attributes.
type Cell struct {
	Rune rune
	// TODO(stage-2): FG, BG color, attrs bitmask (bold/italic/underline).
}

// blank is the value of an empty cell.
var blank = Cell{Rune: ' '}
