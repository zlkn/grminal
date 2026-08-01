package vte

// EnterAlt switches to a cleared alternate screen buffer, saving the primary
// one. A no-op if the alternate screen is already active.
func (g *Grid) EnterAlt() {
	if g.alt != nil {
		return
	}
	g.alt = g.cells
	g.altWrapped = g.wrapped
	g.saved, g.savedAlt = g.savedAlt, g.saved // the alt screen gets its own slot
	g.cells = make([]Cell, g.cols*g.rows)
	g.wrapped = make([]bool, g.rows)
	g.fill(0, blank)
	g.wrapPending = false
}

// ExitAlt restores the primary screen buffer. A no-op on the primary screen.
func (g *Grid) ExitAlt() {
	if g.alt == nil {
		return
	}
	if len(g.alt) == g.cols*g.rows {
		g.cells = g.alt
		if len(g.altWrapped) == g.rows {
			g.wrapped = g.altWrapped
		} else {
			g.wrapped = make([]bool, g.rows)
		}
	}
	g.alt = nil
	g.altWrapped = nil
	g.saved, g.savedAlt = g.savedAlt, g.saved // back to the primary screen's slot
	g.wrapPending = false
}

// OnAltScreen reports whether the alternate screen is active. Full-screen apps
// (vim, htop) run there and manage their own scrolling, so local scrollback
// history does not apply; the input layer reads this to gate wheel scrolling.
func (g *Grid) OnAltScreen() bool { return g.alt != nil }

// SetTitle records the window/tab title (OSC 0/2).
func (g *Grid) SetTitle(s string) { g.title = s }

// Title returns the window/tab title.
func (g *Grid) Title() string { return g.title }

// --- scroll region and line/char editing -------------------------------------

// RowSlice returns the backing cells of row y.
func (g *Grid) RowSlice(y int) []Cell { return g.cells[y*g.cols : (y+1)*g.cols] }

// BlankRow clears row y, using the current background (bce) so scrolled-in and
// inserted lines pick up an active background color.
func (g *Grid) BlankRow(y int) {
	row := g.RowSlice(y)
	blank := g.BlankCell()
	for i := range row {
		row[i] = blank
	}
}

// SetScrollRegion sets the DECSTBM scrolling region and homes the cursor. An
// invalid range resets to the full screen.
func (g *Grid) SetScrollRegion(top, bot int) {
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
	// DECSTBM homes the cursor, respecting origin mode.
	g.curX, g.curY = 0, 0
	if g.originMode {
		g.curY = g.scrollTop
	}
	g.wrapPending = false
}

// ScrollRangeUp scrolls rows [top,bot] up by n, blanking the bottom n rows.
func (g *Grid) ScrollRangeUp(top, bot, n int) {
	if n <= 0 || top > bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := top; y+n <= bot; y++ {
		copy(g.RowSlice(y), g.RowSlice(y+n))
		g.wrapped[y] = g.wrapped[y+n]
	}
	for y := bot - n + 1; y <= bot; y++ {
		g.BlankRow(y)
		g.wrapped[y] = false
	}
}

// ScrollRangeDown scrolls rows [top,bot] down by n, blanking the top n rows.
func (g *Grid) ScrollRangeDown(top, bot, n int) {
	if n <= 0 || top > bot {
		return
	}
	if n > bot-top+1 {
		n = bot - top + 1
	}
	for y := bot; y-n >= top; y-- {
		copy(g.RowSlice(y), g.RowSlice(y-n))
		g.wrapped[y] = g.wrapped[y-n]
	}
	for y := top; y < top+n; y++ {
		g.BlankRow(y)
		g.wrapped[y] = false
	}
}

// ReverseIndex (RI) moves the cursor up, scrolling the region down at the top.
func (g *Grid) ReverseIndex() {
	if g.curY == g.scrollTop {
		g.ScrollRangeDown(g.scrollTop, g.scrollBot, 1)
		return
	}
	if g.curY > 0 {
		g.curY--
	}
}

// InsertLines (IL) inserts n blank lines at the cursor row within the region.
func (g *Grid) InsertLines(n int) {
	if g.curY < g.scrollTop || g.curY > g.scrollBot {
		return
	}
	g.ScrollRangeDown(g.curY, g.scrollBot, n)
}

// DeleteLines (DL) deletes n lines at the cursor row within the region.
func (g *Grid) DeleteLines(n int) {
	if g.curY < g.scrollTop || g.curY > g.scrollBot {
		return
	}
	g.ScrollRangeUp(g.curY, g.scrollBot, n)
}

// InsertChars (ICH) shifts the cursor row right by n, inserting blanks.
func (g *Grid) InsertChars(n int) {
	row := g.RowSlice(g.curY)
	x := g.curX
	if x >= len(row) || n <= 0 {
		return
	}
	if n > len(row)-x {
		n = len(row) - x
	}
	copy(row[x+n:], row[x:len(row)-n])
	blank := g.BlankCell()
	for i := x; i < x+n; i++ {
		row[i] = blank
	}
}

// DeleteChars (DCH) shifts the cursor row left by n, blanking the tail.
func (g *Grid) DeleteChars(n int) {
	row := g.RowSlice(g.curY)
	x := g.curX
	if x >= len(row) || n <= 0 {
		return
	}
	if n > len(row)-x {
		n = len(row) - x
	}
	copy(row[x:], row[x+n:])
	blank := g.BlankCell()
	for i := len(row) - n; i < len(row); i++ {
		row[i] = blank
	}
}

// EraseChars (ECH) blanks n cells from the cursor without moving it.
func (g *Grid) EraseChars(n int) {
	row := g.RowSlice(g.curY)
	blank := g.BlankCell()
	for i := g.curX; i < g.curX+n && i < len(row); i++ {
		row[i] = blank
	}
}
