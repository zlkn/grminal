package vte

import (
	"io"
	"strconv"
	"unicode/utf8"
)

// Parser is the ANSI/VTE state machine. It consumes bytes from the PTY and
// mutates a Grid; it holds no reference to the GPU. State persists across Write
// calls so escape sequences may span chunk boundaries.
//
// It implements a pragmatic subset of the VT100/xterm grammar: printable text,
// the common C0 controls, CSI cursor movement and erase, SGR styling, and
// swallowing of OSC strings and charset-designation escapes.
type Parser struct {
	g     *Grid
	reply io.Writer // where device-query responses are written (the PTY)
	pen   Style     // current graphic rendition applied to printed cells

	state    pstate
	params   [maxParams]int
	nparams  int
	priv     byte   // CSI private marker: one of < = > ?
	overflow bool   // a param exceeded maxParams; ignore the rest of the sequence
	oscBuf   []byte // accumulates the current OSC string payload
}

// oscMax caps the OSC payload length to bound memory on malformed input.
const oscMax = 1024

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

// SetReply sets the writer that receives responses to device queries (DA, DSR,
// cursor-position). In the running app this is the PTY, so answers are delivered
// to the child process as if typed. Without it, such queries are silently
// ignored — which makes shells like fish hang waiting for a reply.
func (p *Parser) SetReply(w io.Writer) { p.reply = w }

// respond writes a query response to the reply writer, if one is set.
func (p *Parser) respond(s string) {
	if p.reply != nil {
		_, _ = io.WriteString(p.reply, s)
	}
}

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
			p.finishOSC()
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
		p.oscBuf = p.oscBuf[:0]
		p.state = stateOSC
	case '(', ')', '*', '+':
		p.state = stateCharset
	case '7': // DECSC - save cursor
		p.g.saveCursor()
		p.state = stateGround
	case '8': // DECRC - restore cursor
		p.g.restoreCursor()
		p.state = stateGround
	case 'D': // IND - index (line feed)
		p.g.lineFeed()
		p.state = stateGround
	case 'E': // NEL - next line
		p.g.curX = 0
		p.g.lineFeed()
		p.state = stateGround
	case 'M': // RI - reverse index
		p.g.reverseIndex()
		p.state = stateGround
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

// osc accumulates an OSC string until BEL or the start of an ST (ESC \).
func (p *Parser) osc(c byte) {
	switch c {
	case 0x07: // BEL
		p.finishOSC()
	case 0x1b: // ESC, expect '\' next
		p.state = stateOSCEsc
	default:
		if len(p.oscBuf) < oscMax {
			p.oscBuf = append(p.oscBuf, c)
		}
	}
}

// finishOSC parses the accumulated OSC payload and returns to the ground state.
// OSC 0 and 2 set the window/tab title; other codes are ignored.
func (p *Parser) finishOSC() {
	p.state = stateGround
	s := p.oscBuf
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			if code := string(s[:i]); code == "0" || code == "2" {
				p.g.setTitle(string(s[i+1:]))
			}
			return
		}
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
	// Device queries must be answered even when a private marker is present
	// (secondary DA is CSI > c).
	switch c {
	case 'c':
		p.deviceAttributes()
		return
	case 'n':
		if p.priv == 0 {
			p.deviceStatus()
		}
		return
	}
	if p.priv == '?' {
		switch c {
		case 'h':
			p.setPrivateMode(true)
		case 'l':
			p.setPrivateMode(false)
		}
		return
	}
	if p.priv != 0 {
		return
	}
	switch c {
	case 's': // SCOSC - save cursor
		p.g.saveCursor()
		return
	case 'u': // SCORC - restore cursor
		p.g.restoreCursor()
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
	case 'r': // DECSTBM - set scroll region
		p.g.setScrollRegion(oneBased(p.param(0, 1))-1, p.param(1, p.g.rows)-1)
	case 'L': // IL - insert lines
		p.g.insertLines(p.count())
	case 'M': // DL - delete lines
		p.g.deleteLines(p.count())
	case '@': // ICH - insert characters
		p.g.insertChars(p.count())
	case 'P': // DCH - delete characters
		p.g.deleteChars(p.count())
	case 'X': // ECH - erase characters
		p.g.eraseChars(p.count())
	case 'S': // SU - scroll up
		p.g.scrollRangeUp(p.g.scrollTop, p.g.scrollBot, p.count())
	case 'T': // SD - scroll down
		p.g.scrollRangeDown(p.g.scrollTop, p.g.scrollBot, p.count())
	}
}

// count returns CSI parameter 0 as a positive repeat count (default/0 -> 1).
func (p *Parser) count() int {
	if n := p.param(0, 1); n > 0 {
		return n
	}
	return 1
}

// deviceAttributes answers a DA query. Primary (CSI c) reports a VT102-class
// terminal; secondary (CSI > c) reports a version tuple.
func (p *Parser) deviceAttributes() {
	switch p.priv {
	case '>':
		p.respond("\x1b[>0;10;0c")
	case 0:
		p.respond("\x1b[?6c")
	}
}

// deviceStatus answers a DSR query: 5 -> "terminal OK", 6 -> cursor position
// report (1-based row;col).
func (p *Parser) deviceStatus() {
	switch p.param(0, 0) {
	case 5:
		p.respond("\x1b[0n")
	case 6:
		p.respond("\x1b[" + strconv.Itoa(p.g.curY+1) + ";" + strconv.Itoa(p.g.curX+1) + "R")
	}
}

// setPrivateMode handles DEC private mode set/reset (CSI ? Pm h / l).
func (p *Parser) setPrivateMode(on bool) {
	for i := 0; i < p.nparams; i++ {
		switch p.params[i] {
		case 25: // DECTCEM - cursor visibility
			p.g.setCursorVisible(on)
		case 47, 1047: // alternate screen buffer
			p.toggleAlt(on)
		case 1048: // save/restore cursor
			if on {
				p.g.saveCursor()
			} else {
				p.g.restoreCursor()
			}
		case 1049: // save cursor + alternate screen (clear on enter)
			if on {
				p.g.saveCursor()
				p.g.enterAlt()
			} else {
				p.g.exitAlt()
				p.g.restoreCursor()
			}
		}
	}
}

// toggleAlt enters or exits the alternate screen buffer.
func (p *Parser) toggleAlt(on bool) {
	if on {
		p.g.enterAlt()
	} else {
		p.g.exitAlt()
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
