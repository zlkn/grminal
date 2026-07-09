package pane

import (
	"bytes"
	"testing"
)

// fakePTY is an in-memory PTY: canned output is served from out, keystrokes
// written by the pane are captured in in, and the last Resize is recorded.
type fakePTY struct {
	out        *bytes.Reader
	in         bytes.Buffer
	rows, cols uint16
	closed     bool
}

func newFakePTY(output string) *fakePTY {
	return &fakePTY{out: bytes.NewReader([]byte(output))}
}

func (f *fakePTY) Read(p []byte) (int, error)  { return f.out.Read(p) }
func (f *fakePTY) Write(p []byte) (int, error) { return f.in.Write(p) }
func (f *fakePTY) Close() error                { f.closed = true; return nil }

func (f *fakePTY) Resize(rows, cols uint16) error {
	f.rows, f.cols = rows, cols
	return nil
}

// Compile-time check that fakePTY satisfies the seam.
var _ PTY = (*fakePTY)(nil)

func rowText(p *Pane, y int) string {
	g := p.Grid()
	rs := make([]rune, g.Cols())
	for x := 0; x < g.Cols(); x++ {
		rs[x] = g.CellAt(x, y).Rune
	}
	return string(rs)
}

func TestPaneRunRendersOutput(t *testing.T) {
	pty := newFakePTY("hello\r\nworld")
	p := NewPane(pty, 10, 3)

	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := rowText(p, 0); got != "hello     " {
		t.Errorf("row0 = %q, want %q", got, "hello     ")
	}
	if got := rowText(p, 1); got != "world     " {
		t.Errorf("row1 = %q, want %q", got, "world     ")
	}
}

func TestPaneWriteForwardsToPTY(t *testing.T) {
	pty := newFakePTY("")
	p := NewPane(pty, 10, 3)

	if _, err := p.Write([]byte("ls\r")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := pty.in.String(); got != "ls\r" {
		t.Errorf("pty received %q, want %q", got, "ls\r")
	}
}

func TestPaneResize(t *testing.T) {
	pty := newFakePTY("")
	p := NewPane(pty, 10, 3)

	if err := p.Resize(100, 40); err != nil {
		t.Fatalf("Resize: %v", err)
	}
	if g := p.Grid(); g.Cols() != 100 || g.Rows() != 40 {
		t.Errorf("grid dims = %dx%d, want 100x40", g.Cols(), g.Rows())
	}
	// PTY takes (rows, cols) — note the order flip.
	if pty.rows != 40 || pty.cols != 100 {
		t.Errorf("pty resized to rows=%d cols=%d, want rows=40 cols=100", pty.rows, pty.cols)
	}
}
