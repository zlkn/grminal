// Package vte holds the ANSI/VTE state machine that consumes bytes from the
// PTY and updates the virtual character Grid. It knows nothing about the GPU.
package vte

// Cell is a single terminal cell: rune plus style attributes.
type Cell struct {
	// TODO(stage-2): Rune rune, FG, BG color, attrs bitmask.
}

// Grid is the virtual screen: a rectangular matrix of Cells.
type Grid struct {
	// TODO(stage-1): cells, rows, cols, cursor.
}
