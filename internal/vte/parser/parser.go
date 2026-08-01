// Package parser implements the ANSI/VTE state machine. It consumes bytes from
// the PTY and mutates a vte.Grid; it holds no reference to the GPU or PTY OS handles.
package parser

import (
	"io"
	"strconv"
	"unicode/utf8"

	"github.com/yzolkin/go-vte/internal/vte"
)

// Parser is the ANSI/VTE state machine. State persists across Write calls so
// escape sequences and multi-byte UTF-8 runes may span chunk boundaries.
type Parser struct {
	g     *vte.Grid
	reply io.Writer // where device-query responses are written (the PTY)
	pen   vte.Style // current graphic rendition applied to printed cells

	state    pstate
	params   [maxParams]int
	nparams  int
	priv     byte   // CSI private marker: one of < = > ?
	inter    byte   // CSI intermediate byte (0x20-0x2f), e.g. the SP of DECSCUSR
	overflow bool   // a param exceeded maxParams; ignore the rest of the sequence
	oscBuf   []byte // accumulates the current OSC string payload

	// Partial UTF-8 decoding buffer for multi-byte runes split across Write chunks.
	utfBuf [4]byte
	utfLen int
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
func NewParser(g *vte.Grid) *Parser { return &Parser{g: g} }

// SetReply sets the writer that receives responses to device queries (DA, DSR,
// cursor-position). In the running app this is the PTY, so answers are delivered
// to the child process as if typed. Without it, such queries are silently
// ignored — which makes shells like fish hang waiting for a reply.
func (p *Parser) SetReply(w io.Writer) { p.reply = w }

// saveCursor and restoreCursor bridge the pen into the grid's save slot.
func (p *Parser) saveCursor() { p.g.SaveCursor(p.pen) }

func (p *Parser) restoreCursor() {
	p.pen = p.g.RestoreCursor()
	p.g.SetCurBG(p.pen.BG) // keep background-color erase in sync with the pen
}

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
	i := 0

	for i < len(b) {
		c := b[i]
		switch p.state {
		case stateGround:
			// Handle printable runes (possibly multi-byte UTF-8).
			if c >= 0x20 && c != 0x7f {
				var r rune
				var size int

				if p.utfLen > 0 {
					// Pending partial UTF-8 sequence from a previous chunk.
					need := 4 - p.utfLen
					if need > len(b)-i {
						need = len(b) - i
					}
					copy(p.utfBuf[p.utfLen:], b[i:i+need])
					totalLen := p.utfLen + need

					if utf8.FullRune(p.utfBuf[:totalLen]) {
						r, size = utf8.DecodeRune(p.utfBuf[:totalLen])
						p.g.PutCell(vte.Cell{Rune: r, Style: p.pen})
						i += size - p.utfLen
						p.utfLen = 0
						continue
					} else {
						// Still incomplete: buffer available bytes and wait for next Write.
						p.utfLen = totalLen
						i += need
						continue
					}
				}

				// Check if current slice has a full rune or an incomplete trailing sequence.
				if !utf8.FullRune(b[i:]) {
					// Incomplete rune at end of buffer: save in utfBuf for next Write.
					p.utfLen = copy(p.utfBuf[:], b[i:])
					i = len(b)
					continue
				}

				r, size = utf8.DecodeRune(b[i:])
				p.g.PutCell(vte.Cell{Rune: r, Style: p.pen})
				i += size
				continue
			}
			p.ground(c)
			i++
		case stateEscape:
			p.escape(c)
			i++
		case stateCSI:
			p.csi(c)
			i++
		case stateOSC:
			p.osc(c)
			i++
		case stateOSCEsc:
			// The byte after ESC inside an OSC terminates it (ST = ESC \).
			if c == '\\' {
				p.finishOSC()
				i++
			} else {
				p.finishOSC()
				// Re-evaluate c in stateGround so aborted sequence is not lost.
			}
		case stateCharset:
			p.state = stateGround // swallow the single designator byte
			i++
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
		p.g.LineFeed()
	case '\r':
		p.g.CarriageReturn()
	case '\t':
		p.g.Tab()
	case 0x08: // BS
		p.g.Backspace()
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
		p.saveCursor()
		p.state = stateGround
	case '8': // DECRC - restore cursor
		p.restoreCursor()
		p.state = stateGround
	case 'D': // IND - index (line feed)
		p.g.LineFeed()
		p.state = stateGround
	case 'E': // NEL - next line
		p.g.CarriageReturn()
		p.g.LineFeed()
		p.state = stateGround
	case 'M': // RI - reverse index
		p.g.ReverseIndex()
		p.state = stateGround
	case 'c': // RIS - reset to initial state
		p.g.Clear()
		p.pen = vte.Style{}
		p.state = stateGround
	case 0x1b:
		// A second ESC just restarts the sequence; stay in the escape state.
	default:
		p.state = stateGround // ignore other escapes
	}
}

// beginCSI resets parameter state at the start of a CSI sequence.
func (p *Parser) beginCSI() {
	p.nparams = 0
	p.priv = 0
	p.inter = 0
	p.overflow = false
	for i := range p.params {
		p.params[i] = 0
	}
}

// csi accumulates parameters and dispatches on the final byte.
func (p *Parser) csi(c byte) {
	switch {
	case c == 0x1b:
		p.state = stateEscape
	case c >= '0' && c <= '9':
		if p.nparams == 0 {
			p.nparams = 1
		}
		if idx := p.nparams - 1; idx < maxParams {
			p.params[idx] = p.params[idx]*10 + int(c-'0')
		}
	case c == ';', c == ':': // accept ';' and ':' as parameter delimiters
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
	case c >= 0x20 && c <= 0x2f: // intermediate byte
		p.inter = c
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
func (p *Parser) finishOSC() {
	p.state = stateGround
	s := p.oscBuf
	for i := 0; i < len(s); i++ {
		if s[i] == ';' {
			if code := string(s[:i]); code == "0" || code == "2" {
				p.g.SetTitle(string(s[i+1:]))
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

// dispatchCSI executes a completed CSI sequence with final byte c.
func (p *Parser) dispatchCSI(c byte) {
	if p.inter != 0 {
		if p.inter == ' ' && c == 'q' && p.priv == 0 {
			p.g.SetCursorStyle(p.param(0, 0))
		}
		return
	}
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
		p.saveCursor()
		return
	case 'u': // SCORC - restore cursor
		p.restoreCursor()
		return
	}
	switch c {
	case 'H', 'f': // CUP - cursor position
		p.g.CursorTo(oneBased(p.param(1, 1))-1, oneBased(p.param(0, 1))-1)
	case 'A': // CUU
		p.g.CursorUp(oneBased(p.param(0, 1)))
	case 'B': // CUD
		p.g.CursorDown(oneBased(p.param(0, 1)))
	case 'C': // CUF
		p.g.MoveCursor(p.g.CursorX()+oneBased(p.param(0, 1)), p.g.CursorY())
	case 'D': // CUB
		p.g.MoveCursor(p.g.CursorX()-oneBased(p.param(0, 1)), p.g.CursorY())
	case 'G', '`': // CHA
		p.g.MoveCursor(oneBased(p.param(0, 1))-1, p.g.CursorY())
	case 'd': // VPA
		p.g.CursorTo(p.g.CursorX(), oneBased(p.param(0, 1))-1)
	case 'J': // ED
		p.g.EraseDisplay(p.param(0, 0))
	case 'K': // EL
		p.g.EraseLine(p.param(0, 0))
	case 'm': // SGR
		p.applySGR()
	case 'r': // DECSTBM
		p.g.SetScrollRegion(oneBased(p.param(0, 1))-1, p.param(1, p.g.Rows())-1)
	case 'L': // IL
		p.g.InsertLines(p.count())
	case 'M': // DL
		p.g.DeleteLines(p.count())
	case '@': // ICH
		p.g.InsertChars(p.count())
	case 'P': // DCH
		p.g.DeleteChars(p.count())
	case 'X': // ECH
		p.g.EraseChars(p.count())
	case 'S': // SU
		p.g.ScrollRangeUp(p.g.ScrollTop(), p.g.ScrollBot(), p.count())
	case 'T': // SD
		p.g.ScrollRangeDown(p.g.ScrollTop(), p.g.ScrollBot(), p.count())
	}
}

func (p *Parser) count() int {
	if n := p.param(0, 1); n > 0 {
		return n
	}
	return 1
}

func (p *Parser) deviceAttributes() {
	switch p.priv {
	case '>':
		p.respond("\x1b[>0;10;0c")
	case 0:
		p.respond("\x1b[?6c")
	}
}

func (p *Parser) deviceStatus() {
	switch p.param(0, 0) {
	case 5:
		p.respond("\x1b[0n")
	case 6:
		row, col := p.g.CursorReport()
		p.respond("\x1b[" + strconv.Itoa(row) + ";" + strconv.Itoa(col) + "R")
	}
}

func (p *Parser) setPrivateMode(on bool) {
	for i := 0; i < p.nparams; i++ {
		switch p.params[i] {
		case 1:
			p.g.SetAppCursorKeys(on)
		case 6:
			p.g.SetOriginMode(on)
		case 7:
			p.g.SetAutoWrap(on)
		case 25:
			p.g.SetCursorVisible(on)
		case 1000:
			p.g.SetMouseMode(vte.MouseX11, on)
		case 1002:
			p.g.SetMouseMode(vte.MouseButtonMode, on)
		case 1003:
			p.g.SetMouseMode(vte.MouseAny, on)
		case 1006:
			p.g.SetMouseSGR(on)
		case 47, 1047:
			p.toggleAlt(on)
		case 1048:
			if on {
				p.saveCursor()
			} else {
				p.restoreCursor()
			}
		case 1049:
			if on {
				p.saveCursor()
				p.g.EnterAlt()
			} else {
				p.g.ExitAlt()
				p.restoreCursor()
			}
		}
	}
}

func (p *Parser) toggleAlt(on bool) {
	if on {
		p.g.EnterAlt()
	} else {
		p.g.ExitAlt()
	}
}

func oneBased(v int) int {
	if v == 0 {
		return 1
	}
	return v
}

func (p *Parser) applySGR() {
	if p.nparams == 0 {
		p.pen = vte.Style{}
		p.g.SetCurBG(p.pen.BG)
		return
	}
	for i := 0; i < p.nparams; i++ {
		n := p.params[i]
		switch {
		case n == 0:
			p.pen = vte.Style{}
		case n == 1:
			p.pen.Attrs |= vte.AttrBold
		case n == 3:
			p.pen.Attrs |= vte.AttrItalic
		case n == 4:
			p.pen.Attrs |= vte.AttrUnderline
		case n == 7:
			p.pen.Attrs |= vte.AttrReverse
		case n == 22:
			p.pen.Attrs &^= vte.AttrBold
		case n == 23:
			p.pen.Attrs &^= vte.AttrItalic
		case n == 24:
			p.pen.Attrs &^= vte.AttrUnderline
		case n == 27:
			p.pen.Attrs &^= vte.AttrReverse
		case n >= 30 && n <= 37:
			p.pen.FG = vte.Color{Kind: vte.ColorIndexed, Idx: uint8(n - 30)}
		case n == 38:
			i = p.extColor(i, &p.pen.FG)
		case n == 39:
			p.pen.FG = vte.Color{}
		case n >= 40 && n <= 47:
			p.pen.BG = vte.Color{Kind: vte.ColorIndexed, Idx: uint8(n - 40)}
		case n == 48:
			i = p.extColor(i, &p.pen.BG)
		case n == 49:
			p.pen.BG = vte.Color{}
		case n >= 90 && n <= 97:
			p.pen.FG = vte.Color{Kind: vte.ColorIndexed, Idx: uint8(n - 90 + 8)}
		case n >= 100 && n <= 107:
			p.pen.BG = vte.Color{Kind: vte.ColorIndexed, Idx: uint8(n - 100 + 8)}
		}
	}
	p.g.SetCurBG(p.pen.BG)
}

func (p *Parser) extColor(i int, dst *vte.Color) int {
	switch p.param(i+1, -1) {
	case 5:
		if i+2 < p.nparams {
			*dst = vte.Color{Kind: vte.ColorIndexed, Idx: uint8(p.params[i+2])}
			return i + 2
		}
	case 2:
		if i+4 < p.nparams {
			*dst = vte.Color{
				Kind: vte.ColorRGB,
				R:    uint8(p.params[i+2]),
				G:    uint8(p.params[i+3]),
				B:    uint8(p.params[i+4]),
			}
			return i + 4
		}
	}
	return i
}
