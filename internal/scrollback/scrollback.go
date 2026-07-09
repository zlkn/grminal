// Package scrollback implements a pre-allocated ring buffer for terminal
// history. Scrolling and appending move head/tail indices only; on the steady
// state (buffer full, lines no wider than the pre-allocated width) no per-line
// allocation happens on the hot path.
package scrollback

import "github.com/yzolkin/go-vte/internal/vte"

// Ring is a fixed-capacity circular buffer of terminal lines. Once full, the
// oldest line is evicted on each Push and its backing array is reused.
type Ring struct {
	lines [][]vte.Cell // backing storage, len == capacity
	start int          // index of the oldest live line
	count int          // number of live lines, 0..capacity
}

// NewRing returns a ring holding up to capacity lines, each backed by a slice
// pre-allocated to width cells. capacity and width are clamped to a minimum
// of 1. width is only a hint: lines wider than it still store correctly, at
// the cost of a one-off allocation.
func NewRing(capacity, width int) *Ring {
	if capacity < 1 {
		capacity = 1
	}
	if width < 0 {
		width = 0
	}
	lines := make([][]vte.Cell, capacity)
	for i := range lines {
		lines[i] = make([]vte.Cell, 0, width)
	}
	return &Ring{lines: lines}
}

// Cap returns the maximum number of lines the ring can hold.
func (r *Ring) Cap() int { return len(r.lines) }

// Len returns the number of live lines currently stored.
func (r *Ring) Len() int { return r.count }

// Push appends a copy of src as the newest line, evicting the oldest line when
// the ring is full. The evicted slot's backing array is reused, so pushing
// lines no wider than the pre-allocated width does not allocate.
func (r *Ring) Push(src []vte.Cell) {
	var slot int
	if r.count < len(r.lines) {
		slot = (r.start + r.count) % len(r.lines)
		r.count++
	} else {
		slot = r.start
		r.start = (r.start + 1) % len(r.lines)
	}

	dst := r.lines[slot]
	if cap(dst) < len(src) {
		dst = make([]vte.Cell, len(src)) // rare: src wider than the width hint
	} else {
		dst = dst[:len(src)]
	}
	copy(dst, src)
	r.lines[slot] = dst
}

// At returns the line at logical index i, where 0 is the oldest live line and
// Len()-1 is the newest. It returns nil for out-of-range indices. The returned
// slice aliases internal storage and may be overwritten by a later Push.
func (r *Ring) At(i int) []vte.Cell {
	if i < 0 || i >= r.count {
		return nil
	}
	return r.lines[(r.start+i)%len(r.lines)]
}
