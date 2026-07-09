// Package layout implements the LayoutManager: a flat Flexbox-style geometry
// manager (Columns -> Rows) that distributes screen space between panes. It
// deliberately avoids BSP trees — the screen is an ordered list of columns, and
// each column is an ordered list of stacked panes (rows).
//
// It is pure geometry: no GPU, no pane contents, just integer rectangles.
package layout

import "math"

// Rect is an integer rectangle in cell (or pixel) coordinates.
type Rect struct {
	X, Y, W, H int
}

// Contains reports whether (x, y) lies inside r.
func (r Rect) Contains(x, y int) bool {
	return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H
}

// Area returns the rectangle's area.
func (r Rect) Area() int { return r.W * r.H }

// minSize is the smallest width or height a pane may occupy.
const minSize = 1

// column is an ordered stack of pane ids (top to bottom).
type column struct {
	rows []int
}

// Layout is the flat Columns→Rows arrangement of panes within a bounds.
type Layout struct {
	bounds Rect
	cols   []column
	nextID int
}

// New returns a layout with a single pane (id 0) filling bounds.
func New(bounds Rect) *Layout {
	return &Layout{
		bounds: bounds,
		cols:   []column{{rows: []int{0}}},
		nextID: 1,
	}
}

// Count returns the number of panes.
func (l *Layout) Count() int {
	n := 0
	for _, c := range l.cols {
		n += len(c.rows)
	}
	return n
}

// find locates the column/row indices of pane id.
func (l *Layout) find(id int) (ci, ri int, ok bool) {
	for ci, c := range l.cols {
		for ri, pid := range c.rows {
			if pid == id {
				return ci, ri, true
			}
		}
	}
	return 0, 0, false
}

// SplitVertical inserts a new full-height column immediately to the right of the
// column containing id, returning the new pane id. It fails (ok == false) if id
// is unknown or the columns would fall below the minimum width.
func (l *Layout) SplitVertical(id int) (newID int, ok bool) {
	ci, _, found := l.find(id)
	if !found {
		return 0, false
	}
	if l.bounds.W < (len(l.cols)+1)*minSize {
		return 0, false
	}
	newID = l.nextID
	l.nextID++
	col := column{rows: []int{newID}}
	l.cols = append(l.cols, column{})
	copy(l.cols[ci+2:], l.cols[ci+1:])
	l.cols[ci+1] = col
	return newID, true
}

// SplitHorizontal inserts a new pane directly below id within its column,
// returning the new pane id. It fails (ok == false) if id is unknown or the
// column's rows would fall below the minimum height.
func (l *Layout) SplitHorizontal(id int) (newID int, ok bool) {
	ci, ri, found := l.find(id)
	if !found {
		return 0, false
	}
	if l.bounds.H < (len(l.cols[ci].rows)+1)*minSize {
		return 0, false
	}
	newID = l.nextID
	l.nextID++
	rows := l.cols[ci].rows
	rows = append(rows, 0)
	copy(rows[ri+2:], rows[ri+1:])
	rows[ri+1] = newID
	l.cols[ci].rows = rows
	return newID, true
}

// Resize changes the bounds; the next Rects call re-tiles proportionally.
func (l *Layout) Resize(bounds Rect) { l.bounds = bounds }

// Rects computes the current rectangle for every pane, keyed by pane id. The
// result exactly tiles the bounds (disjoint, gap-free).
func (l *Layout) Rects() map[int]Rect {
	res := make(map[int]Rect, l.Count())
	colW := distribute(l.bounds.W, len(l.cols))
	x := l.bounds.X
	for ci, c := range l.cols {
		w := colW[ci]
		rowH := distribute(l.bounds.H, len(c.rows))
		y := l.bounds.Y
		for ri, id := range c.rows {
			h := rowH[ri]
			res[id] = Rect{X: x, Y: y, W: w, H: h}
			y += h
		}
		x += w
	}
	return res
}

// PaneAt returns the pane whose rectangle contains (x, y).
func (l *Layout) PaneAt(x, y int) (id int, ok bool) {
	for id, r := range l.Rects() {
		if r.Contains(x, y) {
			return id, true
		}
	}
	return 0, false
}

// distribute splits total into n integer parts as evenly as possible, using
// cumulative rounding so the parts always sum exactly to total (no gaps).
func distribute(total, n int) []int {
	sizes := make([]int, n)
	prev := 0
	for i := 0; i < n; i++ {
		pos := int(math.Round(float64(i+1) / float64(n) * float64(total)))
		sizes[i] = pos - prev
		prev = pos
	}
	return sizes
}
