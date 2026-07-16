package vte

// Snapshot is an immutable, independent copy of a Grid's state at one instant.
// The renderer consumes a Snapshot rather than the live grid, which both removes
// the data race with the PTY writer goroutine and makes rendering deterministic.
type Snapshot struct {
	Cols, Rows    int
	Cells         []Cell // len == Cols*Rows, row-major copy
	CurX, CurY    int
	CursorVisible bool
	Title         string
}

// Snapshot returns a deep copy of the grid's current state. Callers may read or
// mutate the returned value freely without affecting the grid.
func (g *Grid) Snapshot() Snapshot {
	cells := make([]Cell, len(g.cells))
	copy(cells, g.cells)
	return Snapshot{
		Cols:          g.cols,
		Rows:          g.rows,
		Cells:         cells,
		CurX:          g.curX,
		CurY:          g.curY,
		CursorVisible: !g.cursorHidden,
		Title:         g.title,
	}
}

// Row returns the cells of row y within the snapshot.
func (s Snapshot) Row(y int) []Cell {
	return s.Cells[y*s.Cols : (y+1)*s.Cols]
}

// ScrolledSnapshot composes a viewport that shows scrollback history above the
// live screen. history is the scrollback lines, oldest first; live is the current
// screen; offset is how many lines the view is scrolled up from the live bottom.
//
// The visible viewport is live.Rows tall. Its top sits at content index
// len(history)-offset, where content index c<len(history) is a history line and
// c>=len(history) is live row c-len(history). offset <= 0 returns live unchanged;
// larger offsets are clamped to len(history). History rows are normalized to the
// live width (short rows padded with blanks, long rows truncated). The cursor is
// hidden whenever the view is scrolled into history.
func ScrolledSnapshot(history [][]Cell, live Snapshot, offset int) Snapshot {
	if offset > len(history) {
		offset = len(history)
	}
	if offset <= 0 {
		return live
	}

	cols, rows := live.Cols, live.Rows
	cells := make([]Cell, cols*rows)
	for i := range cells {
		cells[i] = blank // so padding under-width history rows is drawable space
	}
	top := len(history) - offset // content index of the first visible row
	for y := 0; y < rows; y++ {
		dst := cells[y*cols : (y+1)*cols]
		switch c := top + y; {
		case c < len(history):
			copy(dst, history[c]) // copy() truncates a wider row, pads a narrower one
		default:
			copy(dst, live.Row(c-len(history)))
		}
	}

	return Snapshot{
		Cols:          cols,
		Rows:          rows,
		Cells:         cells,
		CurX:          live.CurX,
		CurY:          live.CurY,
		CursorVisible: false, // no cursor while viewing history
		Title:         live.Title,
	}
}
