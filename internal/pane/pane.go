// Package pane is a single terminal instance: it owns a PTY and the VTE grid
// the PTY's output is rendered into, and holds input focus. It knows nothing
// about the GPU.
package pane

import (
	"errors"
	"io"
	"sync"

	"github.com/yzolkin/go-vte/internal/vte"
)

// readBufSize is the chunk size for draining the PTY.
const readBufSize = 32 * 1024

// bufPool recycles read buffers to keep the PTY drain loop allocation-free.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, readBufSize)
		return &b
	},
}

// Pane is one concrete terminal instance.
type Pane struct {
	pty    PTY
	grid   *vte.Grid
	parser *vte.Parser
}

// NewPane returns a pane wrapping pty with a fresh cols×rows grid.
func NewPane(pty PTY, cols, rows int) *Pane {
	g := vte.NewGrid(cols, rows)
	return &Pane{pty: pty, grid: g, parser: vte.NewParser(g)}
}

// Grid returns the pane's character grid.
func (p *Pane) Grid() *vte.Grid { return p.grid }

// Run drains the PTY into the grid until EOF (or a read error), feeding each
// chunk through the grid's writer. It is synchronous by design so it is
// deterministically testable; the concurrent read goroutine is layered on at a
// later stage.
func (p *Pane) Run() error {
	bufp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufp)
	buf := *bufp

	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			// Parser.Write never errors and always consumes all input.
			_, _ = p.parser.Write(buf[:n])
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// Write forwards keystrokes to the PTY.
func (p *Pane) Write(b []byte) (int, error) { return p.pty.Write(b) }

// Resize resizes the grid and informs the PTY. Note the PTY takes (rows, cols).
func (p *Pane) Resize(cols, rows int) error {
	p.grid.Resize(cols, rows)
	return p.pty.Resize(uint16(rows), uint16(cols))
}

// Close releases the PTY.
func (p *Pane) Close() error { return p.pty.Close() }
