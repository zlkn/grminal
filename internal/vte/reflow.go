package vte

// reflow re-lays the active buffer out at cols×rows. Rows joined by a soft wrap
// (tracked in g.wrapped) are concatenated back into one logical line and then
// re-split at the new width, so text that wrapped at one size is rewrapped—not
// left broken—at another; hard line breaks are preserved. The cursor is carried
// to the same character. When the rewrapped content is taller than rows, the
// excess top lines are evicted to the scroll hook (primary screen only) and the
// view stays anchored to the bottom, matching ordinary scrolling.
//
// Only rows up to the last non-blank row (or the cursor row, whichever is lower
// on screen) take part; trailing blank rows are scratch space, regenerated as
// padding, so they never push real content off the top.
//
// It sets cells, wrapped, cols, rows and the cursor, and homes the scroll
// region. The inactive (alt) buffer is handled separately by Resize.
func (g *Grid) reflow(cols, rows int) {
	used := g.usedRows()

	// 1. Reassemble logical lines from the used rows, remembering where the
	//    cursor falls within its line (an offset from the line start).
	type logical struct {
		cells  []Cell
		curOff int // cursor offset within the line, or -1 if the cursor is elsewhere
	}
	var lines []logical
	for y := 0; y < used; y++ {
		if y == 0 || !g.wrapped[y] {
			lines = append(lines, logical{curOff: -1})
		}
		ln := &lines[len(lines)-1]
		if y == g.curY {
			ln.curOff = len(ln.cells) + g.curX
		}
		ln.cells = append(ln.cells, g.rowSlice(y)...)
	}

	// 2. Rewrap each logical line to the new width, collecting output rows and
	//    their wrapped flags, and mapping the cursor to its new row/col.
	var (
		outRows    [][]Cell
		outWrapped []bool
		curRow     = clamp(g.curY, 0, rows-1)
		curCol     = clamp(g.curX, 0, cols-1)
	)
	for _, ln := range lines {
		// Drop trailing blanks (autowrap padding) but never past the cursor cell.
		n := len(ln.cells)
		for n > 0 && ln.cells[n-1] == blank {
			n--
		}
		if ln.curOff >= n && ln.curOff < len(ln.cells) {
			n = ln.curOff + 1
		}
		content := ln.cells[:n]

		start := len(outRows)
		if len(content) == 0 {
			outRows = append(outRows, blankCells(cols))
			outWrapped = append(outWrapped, false)
		} else {
			for off := 0; off < len(content); off += cols {
				end := min(off+cols, len(content))
				row := blankCells(cols)
				copy(row, content[off:end])
				outRows = append(outRows, row)
				outWrapped = append(outWrapped, off > 0) // continuations after the first
			}
		}
		if ln.curOff >= 0 {
			curRow = start + ln.curOff/cols
			curCol = ln.curOff % cols
		}
	}

	// 3. Fit to rows: evict the top overflow (bottom-anchored), then pad.
	if len(outRows) > rows {
		evict := len(outRows) - rows
		if g.onScroll != nil && g.alt == nil {
			for i := 0; i < evict; i++ {
				g.onScroll(outRows[i])
			}
		}
		outRows = outRows[evict:]
		outWrapped = outWrapped[evict:]
		curRow -= evict
	}
	for len(outRows) < rows {
		outRows = append(outRows, blankCells(cols))
		outWrapped = append(outWrapped, false)
	}

	// 4. Flatten into the backing store.
	cells := make([]Cell, cols*rows)
	for y := 0; y < rows; y++ {
		copy(cells[y*cols:(y+1)*cols], outRows[y])
	}
	g.cells = cells
	g.wrapped = outWrapped
	g.cols, g.rows = cols, rows
	g.curX = clamp(curCol, 0, cols-1)
	g.curY = clamp(curRow, 0, rows-1)
	g.wrapPending = false
	g.scrollTop, g.scrollBot = 0, rows-1
}

// usedRows returns the number of rows from the top that hold meaningful state:
// through the last non-blank row or the cursor row, whichever is lower on
// screen. Rows below are blank scratch and are excluded from reflow.
func (g *Grid) usedRows() int {
	last := g.curY
	for y := g.rows - 1; y >= 0; y-- {
		row := g.rowSlice(y)
		blankLine := true
		for _, c := range row {
			if c != blank {
				blankLine = false
				break
			}
		}
		if !blankLine {
			if y > last {
				last = y
			}
			break
		}
	}
	return last + 1
}

// blankCells returns a fresh row of n default cells.
func blankCells(n int) []Cell {
	row := make([]Cell, n)
	for i := range row {
		row[i] = blank
	}
	return row
}
