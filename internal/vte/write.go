package vte

import "unicode/utf8"

// Write feeds a chunk of terminal output into the grid at the cursor.
//
// This is the MVP "cooked" writer: it handles printable runes (with line wrap),
// LF, CR and TAB, and scrolls the grid up when output runs past the last row.
// Full ANSI/CSI escape parsing arrives at step 5; escape bytes are ignored for
// now. It also assumes each rune is fully contained in b — decoding runes split
// across successive Write calls is the step-5 parser's concern.
//
// Write always consumes all of b and returns len(b), nil to satisfy io.Writer.
func (g *Grid) Write(b []byte) (int, error) {
	n := len(b)
	for len(b) > 0 {
		r, size := utf8.DecodeRune(b)
		b = b[size:]

		switch r {
		case '\n':
			g.lineFeed()
		case '\r':
			g.curX = 0
		case '\t':
			next := (g.curX/tabWidth + 1) * tabWidth
			g.curX = min(next, g.cols-1)
		default:
			if r < 0x20 || r == 0x7f {
				continue // ignore other C0 control bytes for now
			}
			g.putRune(r)
		}
	}
	return n, nil
}

const tabWidth = 8

// putRune writes r at the cursor and advances, wrapping to the next line when
// the cursor moves past the right edge.
func (g *Grid) putRune(r rune) {
	g.cells[g.curY*g.cols+g.curX] = Cell{Rune: r}
	g.curX++
	if g.curX >= g.cols {
		g.curX = 0
		g.lineFeed()
	}
}

// lineFeed moves the cursor down one row, scrolling the grid up when already on
// the last row.
func (g *Grid) lineFeed() {
	if g.curY < g.rows-1 {
		g.curY++
		return
	}
	g.scrollUp()
}

// scrollUp shifts every row up by one; the top row is discarded and the bottom
// row is blanked. The cursor row is left at the (unchanged) bottom.
func (g *Grid) scrollUp() {
	copy(g.cells, g.cells[g.cols:])
	bottom := (g.rows - 1) * g.cols
	for i := bottom; i < len(g.cells); i++ {
		g.cells[i] = blank
	}
}
