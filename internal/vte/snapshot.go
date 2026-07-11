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
