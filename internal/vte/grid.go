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

	// alt holds the inactive screen buffer while the alternate screen is active
	// (nil on the primary screen). saved is the DECSC/1048/1049 save slot of the
	// active buffer and savedAlt that of the inactive one; the two are swapped on
	// enter/exit because xterm keeps one saved cursor per screen. Sharing a single
	// slot lets an app's DECSC inside the alt screen clobber the primary's save,
	// which lands the shell prompt in the wrong place on exit.
	alt             []Cell
	saved, savedAlt SavedCursor
	cursorHidden    bool
	// cursorShape and cursorBlink hold the DECSCUSR (CSI Ps SP q) presentation
	// state. Their zero values defer to the user's config.
	cursorShape CursorShape
	cursorBlink CursorBlink

	// wrapped[y] reports whether row y is a soft-wrap continuation of row y-1
	// (the line break above it came from autowrap, not a hard newline). It is
	// the durable record of where logical lines break, used to reflow text when
	// the grid is resized. altWrapped mirrors it for the inactive buffer.
	wrapped    []bool // len == rows
	altWrapped []bool

	// scrollTop/scrollBot bound the vertical scrolling region (DECSTBM),
	// defaulting to the whole screen [0, rows-1].
	scrollTop, scrollBot int

	// wrapPending is xterm's do_wrap flag: it is set after a glyph is written to
	// the last column so the wrap is deferred until the next printable rune
	// (VT100 autowrap). Without it, writing the rightmost cell would advance the
	// row immediately, mis-scrolling apps that address the margin (e.g. neovim).
	wrapPending bool
	// autoWrap is DECAWM (mode ?7). When false, text at the right margin
	// overwrites the last column instead of wrapping. Defaults to true.
	autoWrap bool
	// appCursorKeys is DECCKM (mode ?1). When true (ncurses apps like htop set it
	// via smkx), the cursor keys must be sent as SS3 (ESC O A) rather than CSI
	// (ESC [ A); the input layer reads this to pick the encoding.
	appCursorKeys bool
	// mouseMode is the active mouse-reporting level (?1000/?1002/?1003) and
	// mouseSGR is the SGR extended encoding (?1006). The input layer reads these
	// (via the pane) to decide whether and how to report pointer events.
	mouseMode MouseMode
	mouseSGR  bool
	// originMode is DECOM (mode ?6). When set, CUP/VPA row addressing is relative
	// to the scroll region top and the cursor is confined to the region.
	originMode bool
	// curBG is the current SGR background color, applied to cells cleared by
	// erase/scroll/insert operations (xterm "background color erase", bce). The
	// parser keeps it in sync with the pen's background on every SGR.
	curBG Color

	// title is the window/tab title set via OSC 0/2.
	title string
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
	g := &Grid{
		cols:      cols,
		rows:      rows,
		cells:     make([]Cell, cols*rows),
		wrapped:   make([]bool, rows),
		scrollBot: rows - 1,
		autoWrap:  true,
	}
	g.fill(0, blank)
	return g
}

// Cols returns the number of columns.
func (g *Grid) Cols() int { return g.cols }

// Rows returns the number of rows.
func (g *Grid) Rows() int { return g.rows }

// Cursor returns the current cursor position (x, y).
func (g *Grid) Cursor() (x, y int) { return g.curX, g.curY }

// CursorX returns the current cursor column (0-based).
func (g *Grid) CursorX() int { return g.curX }

// CursorY returns the current cursor row (0-based).
func (g *Grid) CursorY() int { return g.curY }

// ScrollTop returns the top row index of the scrolling region.
func (g *Grid) ScrollTop() int { return g.scrollTop }

// ScrollBot returns the bottom row index of the scrolling region.
func (g *Grid) ScrollBot() int { return g.scrollBot }

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

// MoveCursor sets the cursor position, clamping to the grid bounds. Any explicit
// cursor movement cancels a pending autowrap (xterm resets do_wrap on move).
func (g *Grid) MoveCursor(x, y int) {
	g.curX = clamp(x, 0, g.cols-1)
	g.curY = clamp(y, 0, g.rows-1)
	g.wrapPending = false
}

// CarriageReturn moves the cursor to column 0, cancelling a pending autowrap.
func (g *Grid) CarriageReturn() {
	g.curX = 0
	g.wrapPending = false
}

// CursorTo sets the cursor from a CUP/VPA-style 0-based (col, row). Under origin
// mode (DECOM) the row is relative to the scroll region top and the cursor is
// confined to the region; otherwise it addresses the whole screen.
func (g *Grid) CursorTo(x, y int) {
	if g.originMode {
		g.curX = clamp(x, 0, g.cols-1)
		g.curY = clamp(g.scrollTop+y, g.scrollTop, g.scrollBot)
	} else {
		g.curX = clamp(x, 0, g.cols-1)
		g.curY = clamp(y, 0, g.rows-1)
	}
	g.wrapPending = false
}

// SetOriginMode toggles DECOM (mode ?6) and homes the cursor to the top-left of
// the addressable area (the scroll region top under origin mode, else 0,0).
func (g *Grid) SetOriginMode(on bool) {
	g.originMode = on
	g.curX = 0
	if on {
		g.curY = g.scrollTop
	} else {
		g.curY = 0
	}
	g.wrapPending = false
}

// SetAutoWrap toggles DECAWM (mode ?7).
func (g *Grid) SetAutoWrap(on bool) { g.autoWrap = on }

// SetAppCursorKeys toggles DECCKM (mode ?1).
func (g *Grid) SetAppCursorKeys(on bool) { g.appCursorKeys = on }

// AppCursorKeys reports whether application cursor key mode (DECCKM) is active,
// so the input layer sends cursor keys as SS3 instead of CSI.
func (g *Grid) AppCursorKeys() bool { return g.appCursorKeys }

// Clear implements RIS: it returns every mode this grid tracks to its power-on
// value, blanks the screen and homes the cursor. A partial reset is worse than
// none here — it is the recovery path a user reaches for after a TUI dies mid-
// draw, so a hidden cursor, a leftover scrolling region or an active alternate
// screen must not survive it.
func (g *Grid) Clear() {
	g.ExitAlt() // RIS always lands on the primary screen (no-op if already there)
	g.fill(0, blank)
	g.resetWrapped()
	g.curX, g.curY = 0, 0
	g.scrollTop, g.scrollBot = 0, g.rows-1
	g.autoWrap = true
	g.appCursorKeys = false
	g.mouseMode = MouseOff
	g.mouseSGR = false
	g.originMode = false
	g.curBG = Color{}
	g.ResetCursorState()
}

// resetWrapped marks every row as a hard line start (no soft-wrap continuation).
func (g *Grid) resetWrapped() {
	for i := range g.wrapped {
		g.wrapped[i] = false
	}
}

// Resize changes the grid dimensions, reflowing the active buffer so that text
// which soft-wrapped at the old width is re-wrapped—not left broken—at the new
// one (see reflow). The scroll region resets to the full screen.
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

	g.reflow(cols, rows) // sets cells/wrapped/cols/rows/cursor and homes the region

	// The inactive (alt/primary) buffer is reallocated blank; full-screen apps
	// redraw on resize, so its stale contents are not worth preserving.
	if g.alt != nil {
		g.alt = make([]Cell, cols*rows)
		for i := range g.alt {
			g.alt[i] = blank
		}
		g.altWrapped = make([]bool, rows)
	}
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

// --- write primitives used by the parser -------------------------------------

const tabWidth = 8

// PutCell writes c at the cursor. It implements VT100 deferred autowrap: writing
// the last column does not advance the row, it arms wrapPending; the wrap (or
// clamp, when DECAWM is off) happens on the next printable rune. This keeps the
// cursor from mis-scrolling when an app fills the rightmost cell (see grid_test).
func (g *Grid) PutCell(c Cell) {
	if g.wrapPending {
		g.wrapPending = false
		if g.autoWrap {
			g.curX = 0
			g.LineFeed()
			g.wrapped[g.curY] = true // this row continues the previous one
		}
		// DECAWM off: stay on the last column and overwrite it.
	}
	g.cells[g.curY*g.cols+g.curX] = c
	if g.curX >= g.cols-1 {
		g.wrapPending = true // at the right margin; defer the wrap
	} else {
		g.curX++
	}
}

// LineFeed moves the cursor down one row, preserving the column. At the bottom of
// the scroll region it scrolls the region up instead. It cancels a pending
// autowrap so a trailing LF does not double-advance past a margin-filled row.
func (g *Grid) LineFeed() {
	g.wrapPending = false
	if g.curY == g.scrollBot {
		g.ScrollUp()
		g.wrapped[g.curY] = false // the fresh bottom row starts a hard line
		return
	}
	if g.curY < g.rows-1 {
		g.curY++
		g.wrapped[g.curY] = false
	}
}

// SetScrollHook registers a callback invoked with each row that scrolls off the
// top of the grid. The slice is only valid for the duration of the call; sinks
// (e.g. the scrollback ring) must copy it.
func (g *Grid) SetScrollHook(fn func(row []Cell)) { g.onScroll = fn }

// ScrollUp scrolls the scroll region up by one line. On the primary screen with
// a top-anchored region, the evicted top line is handed to the scroll hook
// (scrollback) before being overwritten.
func (g *Grid) ScrollUp() {
	if g.onScroll != nil && g.alt == nil && g.scrollTop == 0 {
		g.onScroll(g.RowSlice(g.scrollTop))
	}
	g.ScrollRangeUp(g.scrollTop, g.scrollBot, 1)
}

// Tab advances the cursor to the next tab stop, clamped to the last column.
func (g *Grid) Tab() {
	next := (g.curX/tabWidth + 1) * tabWidth
	g.curX = min(next, g.cols-1)
	g.wrapPending = false
}

// Backspace moves the cursor one column left, stopping at column 0. When an
// autowrap is pending it just cancels that (leaving the cursor on the last
// column), matching xterm's do_wrap handling.
func (g *Grid) Backspace() {
	if g.wrapPending {
		g.wrapPending = false
		return
	}
	if g.curX > 0 {
		g.curX--
	}
}

// EraseLine blanks part of the cursor's row: mode 0 from the cursor to the end,
// 1 from the start to the cursor, 2 the whole line.
func (g *Grid) EraseLine(mode int) {
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
	blank := g.BlankCell()
	for x := x0; x <= x1; x++ {
		g.cells[base+x] = blank
	}
}

// EraseDisplay blanks part of the grid: mode 0 from the cursor to the end, 1
// from the start to the cursor, 2 the whole screen.
func (g *Grid) EraseDisplay(mode int) {
	switch mode {
	case 0:
		g.EraseLine(0)
		g.fillRows(g.curY+1, g.rows)
	case 1:
		g.EraseLine(1)
		g.fillRows(0, g.curY)
	case 2, 3:
		g.fill(0, g.BlankCell())
		g.resetWrapped()
	}
}

// fillRows blanks whole rows in [y0, y1).
func (g *Grid) fillRows(y0, y1 int) {
	blank := g.BlankCell()
	for y := y0; y < y1; y++ {
		base := y * g.cols
		for x := 0; x < g.cols; x++ {
			g.cells[base+x] = blank
		}
		g.wrapped[y] = false
	}
}

// fill sets every cell from index start onward to c.
func (g *Grid) fill(start int, c Cell) {
	for i := start; i < len(g.cells); i++ {
		g.cells[i] = c
	}
}

// SetCurBG records the current SGR background used for background-color erase.
func (g *Grid) SetCurBG(c Color) { g.curBG = c }

// BlankCell is the fill used by erase/scroll/insert operations: a space carrying
// the current background color (bce). With the default background it equals the
// plain blank cell.
func (g *Grid) BlankCell() Cell { return Cell{Rune: ' ', Style: Style{BG: g.curBG}} }

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}
