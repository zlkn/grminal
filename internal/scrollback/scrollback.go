// Package scrollback implements a pre-allocated ring buffer for terminal
// history. Scrolling and appending move head/tail pointers only; no per-line
// allocation happens on the hot path.
package scrollback

// Ring is a fixed-capacity circular buffer of terminal lines.
type Ring struct {
	// TODO(stage-2): lines [][]vte.Cell, head, tail, cap.
}
