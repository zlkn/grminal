package vte

import "strings"

// Grid is the virtual screen: a rectangular matrix of Cells plus a cursor.
// Cells are stored row-major in a single backing slice to keep the memory
// contiguous and the hot paths allocation-free.
type Grid struct {
	cols, rows int
	cells      []Cell // len == cols*rows, row-major
	curX, curY int
}

// NewGrid returns a cols×rows grid filled with blank cells and the cursor at
// the origin. cols and rows are clamped to a minimum of 1.
func NewGrid(cols, rows int) *Grid {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	g := &Grid{cols: cols, rows: rows, cells: make([]Cell, cols*rows)}
	g.fill(0, blank)
	return g
}

// Cols returns the number of columns.
func (g *Grid) Cols() int { return g.cols }

// Rows returns the number of rows.
func (g *Grid) Rows() int { return g.rows }

// Cursor returns the current cursor position (x, y).
func (g *Grid) Cursor() (x, y int) { return g.curX, g.curY }

// inBounds reports whether (x, y) is a valid cell coordinate.
func (g *Grid) inBounds(x, y int) bool {
	return x >= 0 && x < g.cols && y >= 0 && y < g.rows
}

// CellAt returns the cell at (x, y). Out-of-bounds coordinates return blank.
func (g *Grid) CellAt(x, y int) Cell {
	if !g.inBounds(x, y) {
		return blank
	}
	return g.cells[y*g.cols+x]
}

// SetCell writes c at (x, y). Out-of-bounds writes are silently ignored.
func (g *Grid) SetCell(x, y int, c Cell) {
	if !g.inBounds(x, y) {
		return
	}
	g.cells[y*g.cols+x] = c
}

// MoveCursor sets the cursor position, clamping to the grid bounds.
func (g *Grid) MoveCursor(x, y int) {
	g.curX = clamp(x, 0, g.cols-1)
	g.curY = clamp(y, 0, g.rows-1)
}

// Clear blanks every cell and returns the cursor to the origin.
func (g *Grid) Clear() {
	g.fill(0, blank)
	g.curX, g.curY = 0, 0
}

// Resize changes the grid dimensions, preserving overlapping content. Growing
// fills the new area with blanks; shrinking drops out-of-range cells and clamps
// the cursor into the new bounds.
func (g *Grid) Resize(cols, rows int) {
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	if cols == g.cols && rows == g.rows {
		return
	}

	next := make([]Cell, cols*rows)
	for i := range next {
		next[i] = blank
	}
	copyCols := min(cols, g.cols)
	copyRows := min(rows, g.rows)
	for y := 0; y < copyRows; y++ {
		for x := 0; x < copyCols; x++ {
			next[y*cols+x] = g.cells[y*g.cols+x]
		}
	}

	g.cols, g.rows, g.cells = cols, rows, next
	g.MoveCursor(g.curX, g.curY) // re-clamp into new bounds
}

// Dump renders the grid as plain text, one row per line terminated by '\n'.
// It is the human-readable form used for golden-file snapshots.
func (g *Grid) Dump() string {
	var b strings.Builder
	b.Grow(g.rows * (g.cols + 1))
	for y := 0; y < g.rows; y++ {
		for x := 0; x < g.cols; x++ {
			b.WriteRune(g.cells[y*g.cols+x].Rune)
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// fill sets every cell from index start onward to c.
func (g *Grid) fill(start int, c Cell) {
	for i := start; i < len(g.cells); i++ {
		g.cells[i] = c
	}
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
