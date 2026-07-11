package vte

// enterAlt switches to a cleared alternate screen buffer, saving the primary
// one. A no-op if the alternate screen is already active.
func (g *Grid) enterAlt() {
	if g.alt != nil {
		return
	}
	g.alt = g.cells
	g.cells = make([]Cell, g.cols*g.rows)
	g.fill(0, blank)
}

// exitAlt restores the primary screen buffer. A no-op on the primary screen.
func (g *Grid) exitAlt() {
	if g.alt == nil {
		return
	}
	if len(g.alt) == g.cols*g.rows {
		g.cells = g.alt
	}
	g.alt = nil
}

// saveCursor records the cursor position (DECSC / DEC 1048 / 1049).
func (g *Grid) saveCursor() { g.savedCurX, g.savedCurY = g.curX, g.curY }

// restoreCursor moves the cursor back to the saved position (DECRC).
func (g *Grid) restoreCursor() { g.MoveCursor(g.savedCurX, g.savedCurY) }

// setCursorVisible toggles cursor visibility (DEC mode 25).
func (g *Grid) setCursorVisible(v bool) { g.cursorHidden = !v }

// --- scroll region and line/char editing -------------------------------------

// rowSlice returns the backing cells of row y.
func (g *Grid) rowSlice(y int) []Cell { return g.cells[y*g.cols : (y+1)*g.cols] }

// blankRow clears row y.
func (g *Grid) blankRow(y int) {
	row := g.rowSlice(y)
	for i := range row {
		row[i] = blank
	}
}

// setScrollRegion sets the DECSTBM scrolling region and homes the cursor. An
// invalid range resets to the full screen.
func (g *Grid) setScrollRegion(top, bot int) {
	if top < 0 {
		top = 0
	}
	if bot > g.rows-1 {
		bot = g.rows - 1
	}
	if top >= bot {
		top, bot = 0, g.rows-1
	}
	g.scrollTop, g.scrollBot = top, bot
	g.curX, g.curY = 0, 0
}

// scrollRangeUp scrolls rows [top,bot] up by n, blanking the bottom n rows.
func (g *Grid) scrollRangeUp(top, bot, n int) {
	if n <= 0 || top > bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := top; y+n <= bot; y++ {
		copy(g.rowSlice(y), g.rowSlice(y+n))
	}
	for y := bot - n + 1; y <= bot; y++ {
		g.blankRow(y)
	}
}

// scrollRangeDown scrolls rows [top,bot] down by n, blanking the top n rows.
func (g *Grid) scrollRangeDown(top, bot, n int) {
	if n <= 0 || top > bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := bot; y-n >= top; y-- {
		copy(g.rowSlice(y), g.rowSlice(y-n))
	}
	for y := top; y < top+n; y++ {
		g.blankRow(y)
	}
}

// reverseIndex (RI) moves the cursor up, scrolling the region down at the top.
func (g *Grid) reverseIndex() {
	if g.curY == g.scrollTop {
		g.scrollRangeDown(g.scrollTop, g.scrollBot, 1)
		return
	}
	if g.curY > 0 {
		g.curY--
	}
}

// insertLines (IL) inserts n blank lines at the cursor row within the region.
func (g *Grid) insertLines(n int) {
	if g.curY < g.scrollTop || g.curY > g.scrollBot {
		return
	}
	g.scrollRangeDown(g.curY, g.scrollBot, n)
}

// deleteLines (DL) deletes n lines at the cursor row within the region.
func (g *Grid) deleteLines(n int) {
	if g.curY < g.scrollTop || g.curY > g.scrollBot {
		return
	}
	g.scrollRangeUp(g.curY, g.scrollBot, n)
}

// insertChars (ICH) shifts the cursor row right by n, inserting blanks.
func (g *Grid) insertChars(n int) {
	row := g.rowSlice(g.curY)
	x := g.curX
	if x >= len(row) || n <= 0 {
		return
	}
	if n > len(row)-x {
		n = len(row) - x
	}
	copy(row[x+n:], row[x:len(row)-n])
	for i := x; i < x+n; i++ {
		row[i] = blank
	}
}

// deleteChars (DCH) shifts the cursor row left by n, blanking the tail.
func (g *Grid) deleteChars(n int) {
	row := g.rowSlice(g.curY)
	x := g.curX
	if x >= len(row) || n <= 0 {
		return
	}
	if n > len(row)-x {
		n = len(row) - x
	}
	copy(row[x:], row[x+n:])
	for i := len(row) - n; i < len(row); i++ {
		row[i] = blank
	}
}

// eraseChars (ECH) blanks n cells from the cursor without moving it.
func (g *Grid) eraseChars(n int) {
	row := g.rowSlice(g.curY)
	for i := g.curX; i < g.curX+n && i < len(row); i++ {
		row[i] = blank
	}
}
