package vte

import "strings"

// Grid is the virtual screen: a rectangular matrix of Cells plus a cursor.
// Cells are stored row-major in a single backing slice to keep the memory
// contiguous and the hot paths allocation-free.
type Grid struct {
	cols, rows int
	cells      []Cell // len == cols*rows, row-major
	curX, curY int

	// onScroll, if set, receives each row evicted off the top by scrolling. It
	// is a hook (rather than a direct scrollback dependency) so vte stays free of
	// an import cycle with the scrollback package.
	onScroll func(row []Cell)
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

// --- write primitives used by the parser (same package) ----------------------

const tabWidth = 8

// putCell writes c at the cursor and advances, wrapping to the next line (and
// scrolling when needed) once the cursor moves past the right edge.
func (g *Grid) putCell(c Cell) {
	g.cells[g.curY*g.cols+g.curX] = c
	g.curX++
	if g.curX >= g.cols {
		g.curX = 0
		g.lineFeed()
	}
}

// lineFeed moves the cursor down one row, scrolling up when already on the last
// row.
func (g *Grid) lineFeed() {
	if g.curY < g.rows-1 {
		g.curY++
		return
	}
	g.scrollUp()
}

// SetScrollHook registers a callback invoked with each row that scrolls off the
// top of the grid. The slice is only valid for the duration of the call; sinks
// (e.g. the scrollback ring) must copy it.
func (g *Grid) SetScrollHook(fn func(row []Cell)) { g.onScroll = fn }

// scrollUp shifts every row up by one; the top row is handed to the scroll hook
// (if any) then discarded, and the bottom row is blanked.
func (g *Grid) scrollUp() {
	if g.onScroll != nil {
		g.onScroll(g.cells[:g.cols])
	}
	copy(g.cells, g.cells[g.cols:])
	bottom := (g.rows - 1) * g.cols
	for i := bottom; i < len(g.cells); i++ {
		g.cells[i] = blank
	}
}

// tab advances the cursor to the next tab stop, clamped to the last column.
func (g *Grid) tab() {
	next := (g.curX/tabWidth + 1) * tabWidth
	g.curX = min(next, g.cols-1)
}

// backspace moves the cursor one column left, stopping at column 0.
func (g *Grid) backspace() {
	if g.curX > 0 {
		g.curX--
	}
}

// eraseLine blanks part of the cursor's row: mode 0 from the cursor to the end,
// 1 from the start to the cursor, 2 the whole line.
func (g *Grid) eraseLine(mode int) {
	x0, x1 := 0, g.cols-1
	switch mode {
	case 0:
		x0 = g.curX
	case 1:
		x1 = g.curX
	case 2:
		// whole line
	default:
		return
	}
	base := g.curY * g.cols
	for x := x0; x <= x1; x++ {
		g.cells[base+x] = blank
	}
}

// eraseDisplay blanks part of the grid: mode 0 from the cursor to the end, 1
// from the start to the cursor, 2 the whole screen.
func (g *Grid) eraseDisplay(mode int) {
	switch mode {
	case 0:
		g.eraseLine(0)
		g.fillRows(g.curY+1, g.rows)
	case 1:
		g.eraseLine(1)
		g.fillRows(0, g.curY)
	case 2, 3:
		g.fill(0, blank)
	}
}

// fillRows blanks whole rows in [y0, y1).
func (g *Grid) fillRows(y0, y1 int) {
	for y := y0; y < y1; y++ {
		base := y * g.cols
		for x := 0; x < g.cols; x++ {
			g.cells[base+x] = blank
		}
	}
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
