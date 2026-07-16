// Package pane is a single terminal instance: it owns a PTY and the VTE grid
// the PTY's output is rendered into, and holds input focus. It knows nothing
// about the GPU.
package pane

import (
	"errors"
	"io"
	"sync"
	"sync/atomic"

	"github.com/yzolkin/go-vte/internal/scrollback"
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
//
// The grid is shared between the PTY drain goroutine (which mutates it) and the
// render loop (which reads it). All grid access is serialised by mu; readers
// take an independent Snapshot rather than touching the live grid.
type Pane struct {
	pty      PTY
	mu       sync.Mutex // guards grid mutation and snapshotting
	grid     *vte.Grid
	parser   *vte.Parser
	scroll   *scrollback.Ring
	onChange func()      // optional: called when PTY output mutates the grid
	exited   atomic.Bool // set when Run returns (shell exited or PTY closed)
}

// NewPane returns a pane wrapping pty with a fresh cols×rows grid and a
// scrollback ring of scrollbackLines rows, fed by lines that scroll off the top.
func NewPane(pty PTY, cols, rows, scrollbackLines int) *Pane {
	g := vte.NewGrid(cols, rows)
	ring := scrollback.NewRing(scrollbackLines, cols)
	g.SetScrollHook(ring.Push)
	parser := vte.NewParser(g)
	parser.SetReply(pty) // device-query responses go back to the child process
	return &Pane{pty: pty, grid: g, parser: parser, scroll: ring}
}

// Scrollback returns the pane's history ring.
func (p *Pane) Scrollback() *scrollback.Ring { return p.scroll }

// SetChangeHook registers a callback invoked (from the PTY drain goroutine)
// whenever new output mutates the grid. It lets the renderer redraw on demand
// instead of polling every frame. The hook must be safe to call concurrently.
func (p *Pane) SetChangeHook(fn func()) { p.onChange = fn }

// Title returns the pane's current title (set via OSC), safe to call
// concurrently with Run.
func (p *Pane) Title() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grid.Title()
}

// AppCursorKeys reports whether the focused grid is in application cursor key
// mode (DECCKM), so the input layer sends arrows as SS3. Safe to call
// concurrently with Run.
func (p *Pane) AppCursorKeys() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grid.AppCursorKeys()
}

// EncodeMouse returns the PTY bytes for a pointer event under the grid's current
// mouse-reporting mode, or nil if the event must not be reported. Safe to call
// concurrently with Run.
func (p *Pane) EncodeMouse(ev vte.MouseEvent) []byte {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grid.EncodeMouse(ev)
}

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

// MouseEnabled reports whether the app has enabled mouse reporting (so it owns
// the wheel). Safe to call concurrently with Run.
func (p *Pane) MouseEnabled() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grid.MouseEnabled()
}

// OnAltScreen reports whether the alternate screen is active (no scrollback
// history applies). Safe to call concurrently with Run.
func (p *Pane) OnAltScreen() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.grid.OnAltScreen()
}

// ScrollbackLen returns the number of history lines currently available to scroll
// through. Safe to call concurrently with Run.
func (p *Pane) ScrollbackLen() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.scroll.Len()
}

// SnapshotScrolled returns a snapshot of the view scrolled offset lines up into
// scrollback history (0 = the live screen). On the alternate screen or a
// non-positive offset it is identical to Snapshot. Safe to call concurrently
// with Run.
func (p *Pane) SnapshotScrolled(offset int) vte.Snapshot {
	p.mu.Lock()
	defer p.mu.Unlock()
	if offset <= 0 || p.grid.OnAltScreen() {
		return p.grid.Snapshot()
	}
	if offset > p.scroll.Len() {
		offset = p.scroll.Len()
	}
	// Copy the last offset history rows while holding the lock — Ring.At aliases
	// internal storage that a concurrent Push would overwrite.
	history := make([][]vte.Cell, offset)
	base := p.scroll.Len() - offset
	for i := 0; i < offset; i++ {
		src := p.scroll.At(base + i)
		row := make([]vte.Cell, len(src))
		copy(row, src)
		history[i] = row
	}
	return vte.ScrolledSnapshot(history, p.grid.Snapshot(), offset)
}

// Run drains the PTY into the grid until EOF (or a read error), feeding each
// chunk through the parser. Blocking PTY reads happen outside the lock; only the
// grid mutation is serialised, so a concurrent Snapshot is never starved.
func (p *Pane) Run() error {
	defer p.exited.Store(true) // mark dead so the app can close the tab
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
			if p.onChange != nil {
				p.onChange()
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return err
		}
	}
}

// Exited reports whether Run has returned (the shell exited or the PTY closed).
// Safe to call concurrently with Run.
func (p *Pane) Exited() bool { return p.exited.Load() }

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
