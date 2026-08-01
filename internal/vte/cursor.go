package vte

// CursorShape is the form the cursor is drawn in. The zero value defers to the
// user's configured cursor_style; DECSCUSR (CSI Ps SP q) overrides it per app.
type CursorShape uint8

const (
	CursorShapeDefault   CursorShape = iota // use the configured style
	CursorShapeBlock                        // fills the cell
	CursorShapeUnderline                    // bar along the cell's bottom edge
	CursorShapeBar                          // vertical stripe on the cell's left edge
)

// CursorBlink is the tri-state blink setting. DECSCUSR can ask for blinking or
// steady explicitly; Ps=0 hands the choice back to the user's config, which is
// the zero value.
type CursorBlink uint8

const (
	CursorBlinkDefault CursorBlink = iota // use the configured cursor_blink
	CursorBlinkOn
	CursorBlinkOff
)

// SavedCursor is one DECSC/DECRC (also SCOSC/SCORC, ?1048, ?1049) save slot. The
// DEC spec saves the graphic rendition and origin mode along with the position,
// not just the coordinates. The zero value is "home, default pen", which is also
// what DECRC must do when nothing was ever saved.
type SavedCursor struct {
	x, y   int
	wrap   bool  // pending autowrap (do_wrap)
	origin bool  // DECOM
	pen    Style // SGR rendition; owned by the parser, passed in on save
}

// SaveCursor records the cursor state in the active buffer's slot. pen comes from
// the parser, which owns the current rendition.
func (g *Grid) SaveCursor(pen Style) {
	g.saved = SavedCursor{
		x:      g.curX,
		y:      g.curY,
		wrap:   g.wrapPending,
		origin: g.originMode,
		pen:    pen,
	}
}

// RestoreCursor applies the active buffer's save slot and returns the saved pen
// for the parser to adopt. Origin mode is restored before the position, so the
// row is confined to the region that mode implies.
func (g *Grid) RestoreCursor() Style {
	s := g.saved
	g.originMode = s.origin
	g.curX = clamp(s.x, 0, g.cols-1)
	if g.originMode {
		g.curY = clamp(s.y, g.scrollTop, g.scrollBot)
	} else {
		g.curY = clamp(s.y, 0, g.rows-1)
	}
	g.wrapPending = s.wrap
	return s.pen
}

// SetCursorVisible toggles cursor visibility (DECTCEM, mode 25).
func (g *Grid) SetCursorVisible(v bool) { g.cursorHidden = !v }

// CursorVisible reports whether the cursor should be drawn (DECTCEM).
func (g *Grid) CursorVisible() bool { return !g.cursorHidden }

// CursorShape returns the shape requested by the app, or CursorShapeDefault when
// it has not asked for one.
func (g *Grid) CursorShape() CursorShape { return g.cursorShape }

// CursorBlink returns the blink state requested by the app, or
// CursorBlinkDefault when it has not asked for one.
func (g *Grid) CursorBlink() CursorBlink { return g.cursorBlink }

// SetCursorStyle applies DECSCUSR (CSI Ps SP q), which is how editors switch the
// cursor per mode (neovim: bar while inserting). Ps 0 resets to the user's
// configured style; 1/2 block, 3/4 underline, 5/6 bar, with odd values blinking.
// Unknown parameters are ignored rather than guessed at.
func (g *Grid) SetCursorStyle(ps int) {
	switch ps {
	case 0:
		g.cursorShape, g.cursorBlink = CursorShapeDefault, CursorBlinkDefault
	case 1, 2:
		g.cursorShape, g.cursorBlink = CursorShapeBlock, blinkFor(ps)
	case 3, 4:
		g.cursorShape, g.cursorBlink = CursorShapeUnderline, blinkFor(ps)
	case 5, 6:
		g.cursorShape, g.cursorBlink = CursorShapeBar, blinkFor(ps)
	}
}

// blinkFor maps a DECSCUSR parameter to its blink state: odd values blink.
func blinkFor(ps int) CursorBlink {
	if ps%2 == 1 {
		return CursorBlinkOn
	}
	return CursorBlinkOff
}

// ResetCursorState returns every cursor mode to its power-on value (RIS).
func (g *Grid) ResetCursorState() {
	g.cursorHidden = false
	g.cursorShape, g.cursorBlink = CursorShapeDefault, CursorBlinkDefault
	g.saved, g.savedAlt = SavedCursor{}, SavedCursor{}
	g.wrapPending = false
}

// CursorUp moves the cursor up n rows (CUU). It never scrolls: a cursor starting
// inside the scrolling region stops at the region's top margin, one outside it
// stops at the top of the screen.
func (g *Grid) CursorUp(n int) {
	lo := 0
	if g.inScrollRegion() {
		lo = g.scrollTop
	}
	g.curY = clamp(g.curY-n, lo, g.rows-1)
	g.wrapPending = false
}

// CursorDown moves the cursor down n rows (CUD), stopping at the bottom margin
// when it starts inside the scrolling region (and never scrolling).
func (g *Grid) CursorDown(n int) {
	hi := g.rows - 1
	if g.inScrollRegion() {
		hi = g.scrollBot
	}
	g.curY = clamp(g.curY+n, 0, hi)
	g.wrapPending = false
}

// inScrollRegion reports whether the cursor row currently lies inside the
// scrolling region.
func (g *Grid) inScrollRegion() bool {
	return g.curY >= g.scrollTop && g.curY <= g.scrollBot
}

// CursorReport returns the 1-based row and column for a DSR cursor-position
// report. Under origin mode the row is relative to the scrolling region's top,
// matching how CUP and VPA address rows in that mode.
func (g *Grid) CursorReport() (row, col int) {
	row = g.curY + 1
	if g.originMode {
		row = g.curY - g.scrollTop + 1
	}
	return row, g.curX + 1
}
