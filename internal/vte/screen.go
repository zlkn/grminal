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
