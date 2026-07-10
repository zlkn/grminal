package pane

import (
	"io"
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
)

// pipePTY is a blocking PTY backed by an io.Pipe, so a writer goroutine can feed
// output while Run() drains it — the real PTY-goroutine vs render-loop scenario.
type pipePTY struct {
	r *io.PipeReader
	w *io.PipeWriter
}

func newPipePTY() *pipePTY {
	r, w := io.Pipe()
	return &pipePTY{r: r, w: w}
}

func (p *pipePTY) Read(b []byte) (int, error)     { return p.r.Read(b) }
func (p *pipePTY) Write(b []byte) (int, error)    { return len(b), nil }
func (p *pipePTY) Close() error                   { return p.r.Close() }
func (p *pipePTY) Resize(rows, cols uint16) error { return nil }

var _ PTY = (*pipePTY)(nil)

// TestConcurrentSnapshotDuringRun exercises the grid from two goroutines at
// once: Run() applies PTY output while the main goroutine snapshots and resizes.
// It must be clean under `go test -race`; that is the whole point of the test.
func TestConcurrentSnapshotDuringRun(t *testing.T) {
	pty := newPipePTY()
	p := NewPane(pty, 20, 5, 1000)

	done := make(chan error, 1)
	go func() { done <- p.Run() }()

	// Producer: stream terminal output, then EOF to stop Run().
	go func() {
		for i := 0; i < 200; i++ {
			_, _ = pty.w.Write([]byte("\x1b[1;32mhello\x1b[0m world\r\n"))
		}
		_ = pty.w.Close()
	}()

	// Consumer: hammer Snapshot (and occasionally Resize) concurrently.
	var last vte.Snapshot
	for i := 0; i < 2000; i++ {
		last = p.Snapshot()
		if i%500 == 0 {
			_ = p.Resize(20+i/500, 5)
		}
	}

	if err := <-done; err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Snapshot is a real, self-consistent copy.
	if len(last.Cells) != last.Cols*last.Rows {
		t.Errorf("snapshot inconsistent: %d cells, %dx%d", len(last.Cells), last.Cols, last.Rows)
	}
}

func TestSnapshotIsIndependentCopy(t *testing.T) {
	pty := newFakePTY("hi")
	p := NewPane(pty, 5, 1, 1000)
	_ = p.Run()

	snap := p.Snapshot()
	if snap.Cols != 5 || snap.Rows != 1 {
		t.Fatalf("snapshot dims = %dx%d, want 5x1", snap.Cols, snap.Rows)
	}
	if got := snap.Row(0); string([]rune{got[0].Rune, got[1].Rune}) != "hi" {
		t.Errorf("snapshot row0 = %q, want prefix 'hi'", string([]rune{got[0].Rune, got[1].Rune}))
	}
	// Mutating the snapshot must not affect the live grid.
	snap.Cells[0] = vte.Cell{Rune: 'Z'}
	if again := p.Snapshot(); again.Cells[0].Rune != 'h' {
		t.Error("snapshot aliases live grid: mutation leaked back")
	}
}
