package vte

import "unicode/utf8"

// Parser is the ANSI/VTE state machine. It consumes bytes from the PTY and
// mutates a Grid; it holds no reference to the GPU. State persists across Write
// calls so escape sequences may span chunk boundaries.
//
// It implements a pragmatic subset of the VT100/xterm grammar: printable text,
// the common C0 controls, CSI cursor movement and erase, SGR styling, and
// swallowing of OSC strings and charset-designation escapes.
type Parser struct {
	g   *Grid
	pen Style // current graphic rendition applied to printed cells

	state    pstate
	params   [maxParams]int
	nparams  int
	priv     byte // CSI private marker: one of < = > ?
	overflow bool // a param exceeded maxParams; ignore the rest of the sequence
}

type pstate uint8

const (
	stateGround pstate = iota
	stateEscape
	stateCSI
	stateOSC
	stateOSCEsc  // inside OSC, saw ESC, expecting the ST final '\'
	stateCharset // after ESC ( ) * + : swallow one designator byte
)

const maxParams = 16

// NewParser returns a parser that writes into g.
func NewParser(g *Grid) *Parser { return &Parser{g: g} }

// Write feeds a chunk of terminal output through the state machine. It always
// consumes all of b and returns len(b), nil to satisfy io.Writer.
func (p *Parser) Write(b []byte) (int, error) {
	n := len(b)
	for i := 0; i < len(b); i++ {
		c := b[i]
		switch p.state {
		case stateGround:
			// Printable, possibly multi-byte, runes are the common case.
			if c >= 0x20 && c != 0x7f {
				r, size := utf8.DecodeRune(b[i:])
				p.g.putCell(Cell{Rune: r, Style: p.pen})
				i += size - 1
				continue
			}
			p.ground(c)
		case stateEscape:
			p.escape(c)
		case stateCSI:
			p.csi(c)
		case stateOSC:
			p.osc(c)
		case stateOSCEsc:
			// The byte after ESC inside an OSC terminates it (ST = ESC \).
			p.state = stateGround
		case stateCharset:
			p.state = stateGround // swallow the single designator byte
		}
	}
	return n, nil
}

// ground handles C0 control bytes in the ground state.
func (p *Parser) ground(c byte) {
	switch c {
	case 0x1b: // ESC
		p.state = stateEscape
	case '\n', 0x0b, 0x0c: // LF, VT, FF all move down
		p.g.lineFeed()
	case '\r':
		p.g.curX = 0
	case '\t':
		p.g.tab()
	case 0x08: // BS
		p.g.backspace()
	default:
		// other C0 controls ignored
	}
}

// escape handles the byte following ESC.
func (p *Parser) escape(c byte) {
	switch c {
	case '[':
		p.beginCSI()
		p.state = stateCSI
	case ']':
		p.state = stateOSC
	case '(', ')', '*', '+':
		p.state = stateCharset
	case 'c': // RIS - reset to initial state
		p.g.Clear()
		p.pen = Style{}
		p.state = stateGround
	default:
		p.state = stateGround // ignore other escapes
	}
}

// beginCSI resets parameter state at the start of a CSI sequence.
func (p *Parser) beginCSI() {
	p.nparams = 0
	p.priv = 0
	p.overflow = false
	for i := range p.params {
		p.params[i] = 0
	}
}

// csi accumulates parameters and dispatches on the final byte.
func (p *Parser) csi(c byte) {
	switch {
	case c >= '0' && c <= '9':
		if p.nparams == 0 {
			p.nparams = 1
		}
		if idx := p.nparams - 1; idx < maxParams {
			p.params[idx] = p.params[idx]*10 + int(c-'0')
		}
	case c == ';':
		if p.nparams == 0 {
			p.nparams = 1
		}
		if p.nparams < maxParams {
			p.nparams++
		} else {
			p.overflow = true
		}
	case c >= 0x3c && c <= 0x3f: // < = > ? private markers
		p.priv = c
	case c >= 0x20 && c <= 0x2f: // intermediates, unused here
	case c >= 0x40 && c <= 0x7e: // final byte
		if !p.overflow {
			p.dispatchCSI(c)
		}
		p.state = stateGround
	default:
		p.state = stateGround // malformed; bail out
	}
}

// osc consumes an OSC string until BEL or the start of an ST (ESC \).
func (p *Parser) osc(c byte) {
	switch c {
	case 0x07: // BEL
		p.state = stateGround
	case 0x1b: // ESC, expect '\' next
		p.state = stateOSCEsc
	}
}

// param returns CSI parameter i, or def when it is absent.
func (p *Parser) param(i, def int) int {
	if i >= p.nparams {
		return def
	}
	return p.params[i]
}

// dispatchCSI executes a completed CSI sequence with final byte c. Private
// sequences (DEC modes like ESC[?25h) are recognised but ignored for now.
func (p *Parser) dispatchCSI(c byte) {
	if p.priv != 0 {
		return
	}
	switch c {
	case 'H', 'f': // CUP - cursor position (1-based row;col)
		p.g.MoveCursor(oneBased(p.param(1, 1))-1, oneBased(p.param(0, 1))-1)
	case 'A': // CUU - up
		p.g.MoveCursor(p.g.curX, p.g.curY-oneBased(p.param(0, 1)))
	case 'B': // CUD - down
		p.g.MoveCursor(p.g.curX, p.g.curY+oneBased(p.param(0, 1)))
	case 'C': // CUF - forward
		p.g.MoveCursor(p.g.curX+oneBased(p.param(0, 1)), p.g.curY)
	case 'D': // CUB - back
		p.g.MoveCursor(p.g.curX-oneBased(p.param(0, 1)), p.g.curY)
	case 'G', '`': // CHA - cursor horizontal absolute (1-based col)
		p.g.MoveCursor(oneBased(p.param(0, 1))-1, p.g.curY)
	case 'd': // VPA - vertical position absolute (1-based row)
		p.g.MoveCursor(p.g.curX, oneBased(p.param(0, 1))-1)
	case 'J': // ED - erase in display
		p.g.eraseDisplay(p.param(0, 0))
	case 'K': // EL - erase in line
		p.g.eraseLine(p.param(0, 0))
	case 'm': // SGR - select graphic rendition
		p.applySGR()
	}
}

// oneBased normalises a cursor parameter where 0 means 1.
func oneBased(v int) int {
	if v == 0 {
		return 1
	}
	return v
}

// applySGR updates the pen from the SGR parameters. With no parameters it resets
// to the default style.
func (p *Parser) applySGR() {
	if p.nparams == 0 {
		p.pen = Style{}
		return
	}
	for i := 0; i < p.nparams; i++ {
		n := p.params[i]
		switch {
		case n == 0:
			p.pen = Style{}
		case n == 1:
			p.pen.Attrs |= AttrBold
		case n == 3:
			p.pen.Attrs |= AttrItalic
		case n == 4:
			p.pen.Attrs |= AttrUnderline
		case n == 7:
			p.pen.Attrs |= AttrReverse
		case n == 22:
			p.pen.Attrs &^= AttrBold
		case n == 23:
			p.pen.Attrs &^= AttrItalic
		case n == 24:
			p.pen.Attrs &^= AttrUnderline
		case n == 27:
			p.pen.Attrs &^= AttrReverse
		case n >= 30 && n <= 37:
			p.pen.FG = Color{Kind: ColorIndexed, Idx: uint8(n - 30)}
		case n == 38:
			i = p.extColor(i, &p.pen.FG)
		case n == 39:
			p.pen.FG = Color{}
		case n >= 40 && n <= 47:
			p.pen.BG = Color{Kind: ColorIndexed, Idx: uint8(n - 40)}
		case n == 48:
			i = p.extColor(i, &p.pen.BG)
		case n == 49:
			p.pen.BG = Color{}
		case n >= 90 && n <= 97: // bright foreground
			p.pen.FG = Color{Kind: ColorIndexed, Idx: uint8(n - 90 + 8)}
		case n >= 100 && n <= 107: // bright background
			p.pen.BG = Color{Kind: ColorIndexed, Idx: uint8(n - 100 + 8)}
		}
	}
}

// extColor parses an extended color sub-sequence starting at params[i] (the 38
// or 48 selector). It writes the color into dst and returns the index of the
// last parameter it consumed.
//
//	38;5;n        -> indexed
//	38;2;r;g;b    -> truecolor
func (p *Parser) extColor(i int, dst *Color) int {
	switch p.param(i+1, -1) {
	case 5:
		if i+2 < p.nparams {
			*dst = Color{Kind: ColorIndexed, Idx: uint8(p.params[i+2])}
			return i + 2
		}
	case 2:
		if i+4 < p.nparams {
			*dst = Color{
				Kind: ColorRGB,
				R:    uint8(p.params[i+2]),
				G:    uint8(p.params[i+3]),
				B:    uint8(p.params[i+4]),
			}
			return i + 4
		}
	}
	return i
}
