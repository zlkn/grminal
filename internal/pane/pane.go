// Package pane is a single terminal instance: it owns a PTY and the VTE grid
// the PTY's output is rendered into, and holds input focus. It knows nothing
// about the GPU.
package pane

import (
	"errors"
	"io"
	"sync"

	"github.com/yzolkin/go-vte/internal/scrollback"
	"github.com/yzolkin/go-vte/internal/vte"
)

// readBufSize is the chunk size for draining the PTY.
const readBufSize = 32 * 1024

// scrollbackLines is how many scrolled-off lines each pane retains.
const scrollbackLines = 10000

// bufPool recycles read buffers to keep the PTY drain loop allocation-free.
var bufPool = sync.Pool{
	New: func() any {
		b := make([]byte, readBufSize)
		return &b
	},
}

// Pane is one concrete terminal instance.
//
// The grid is shared between the PTY drain goroutine (which mutates it) and the
// render loop (which reads it). All grid access is serialised by mu; readers
// take an independent Snapshot rather than touching the live grid.
type Pane struct {
	pty    PTY
	mu     sync.Mutex // guards grid mutation and snapshotting
	grid   *vte.Grid
	parser *vte.Parser
	scroll *scrollback.Ring
}

// NewPane returns a pane wrapping pty with a fresh cols×rows grid and a
// scrollback ring fed by lines that scroll off the top.
func NewPane(pty PTY, cols, rows int) *Pane {
	g := vte.NewGrid(cols, rows)
	ring := scrollback.NewRing(scrollbackLines, cols)
	g.SetScrollHook(ring.Push)
	parser := vte.NewParser(g)
	parser.SetReply(pty) // device-query responses go back to the child process
	return &Pane{pty: pty, grid: g, parser: parser, scroll: ring}
}

// Scrollback returns the pane's history ring.
func (p *Pane) Scrollback() *scrollback.Ring { return p.scroll }

// Grid returns the pane's character grid. It is not safe to read concurrently
// with Run; use Snapshot for that. Intended for single-threaded/test use.
func (p *Pane) Grid() *vte.Grid { return p.grid }

// Snapshot returns an independent copy of the grid, safe to call concurrently
// with Run. This is the renderer's entry point into pane state.
func (p *Pane) Snapshot() vte.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grid.Snapshot()
}

// Run drains the PTY into the grid until EOF (or a read error), feeding each
// chunk through the parser. Blocking PTY reads happen outside the lock; only the
// grid mutation is serialised, so a concurrent Snapshot is never starved.
func (p *Pane) Run() error {
	bufp := bufPool.Get().(*[]byte)
	defer bufPool.Put(bufp)
	buf := *bufp

	for {
		n, err := p.pty.Read(buf)
		if n > 0 {
			p.mu.Lock()
			// Parser.Write never errors and always consumes all input.
			_, _ = p.parser.Write(buf[:n])
			p.mu.Unlock()
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
	p.mu.Lock()
	p.grid.Resize(cols, rows)
	p.mu.Unlock()
	return p.pty.Resize(uint16(rows), uint16(cols))
}

// Close releases the PTY.
func (p *Pane) Close() error { return p.pty.Close() }
