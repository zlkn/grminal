package pane

import (
	"bytes"
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
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
	p := NewPane(pty, 10, 3, 1000)

	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := rowText(p, 0); got != "hello     " {
		t.Errorf("row0 = %q, want %q", got, "hello     ")
	}
	if got := rowText(p, 1); got != "world     " {
		t.Errorf("row1 = %q, want %q", got, "world     ")
	}
	// Draining the PTY (EOF, as after Ctrl+D) marks the pane exited.
	if !p.Exited() {
		t.Error("Exited() = false after Run returned on EOF, want true")
	}
}

func TestPaneNotExitedBeforeRun(t *testing.T) {
	p := NewPane(newFakePTY(""), 10, 3, 1000)
	if p.Exited() {
		t.Error("Exited() = true before Run, want false")
	}
}

func TestScrollbackCapturesScrolledLines(t *testing.T) {
	// A 2-row grid; four logical lines force the first two off the top.
	pty := newFakePTY("L0\r\nL1\r\nL2\r\nL3")
	p := NewPane(pty, 5, 2, 1000)
	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	sb := p.Scrollback()
	if sb.Len() != 2 {
		t.Fatalf("scrollback Len = %d, want 2", sb.Len())
	}
	lineText := func(cells []vte.Cell) string {
		rs := make([]rune, len(cells))
		for i, c := range cells {
			rs[i] = c.Rune
		}
		return string(rs)
	}
	if got := lineText(sb.At(0)); got != "L0   " {
		t.Errorf("oldest scrollback line = %q, want %q", got, "L0   ")
	}
	if got := lineText(sb.At(1)); got != "L1   " {
		t.Errorf("second scrollback line = %q, want %q", got, "L1   ")
	}
}

func TestSnapshotScrolled(t *testing.T) {
	// 2-row grid; L0/L1 scroll into history, L2/L3 remain on the live screen.
	pty := newFakePTY("L0\r\nL1\r\nL2\r\nL3")
	p := NewPane(pty, 5, 2, 1000)
	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}

	snapRow := func(s vte.Snapshot, y int) string {
		rs := make([]rune, 0, s.Cols)
		for _, c := range s.Row(y) {
			rs = append(rs, c.Rune)
		}
		return string(rs)
	}

	// offset 0: the live screen, cursor visible.
	live := p.SnapshotScrolled(0)
	if snapRow(live, 0) != "L2   " || snapRow(live, 1) != "L3   " {
		t.Errorf("offset 0 = %q/%q, want L2/L3", snapRow(live, 0), snapRow(live, 1))
	}
	if !live.CursorVisible {
		t.Error("offset 0 should keep the cursor visible")
	}

	// offset 1: newest history line on top, live row below it, cursor hidden.
	up := p.SnapshotScrolled(1)
	if snapRow(up, 0) != "L1   " || snapRow(up, 1) != "L2   " {
		t.Errorf("offset 1 = %q/%q, want L1/L2", snapRow(up, 0), snapRow(up, 1))
	}
	if up.CursorVisible {
		t.Error("scrolled-back view should hide the cursor")
	}

	// offset beyond history clamps to the two available lines (L0/L1).
	max := p.SnapshotScrolled(99)
	if snapRow(max, 0) != "L0   " || snapRow(max, 1) != "L1   " {
		t.Errorf("clamped offset = %q/%q, want L0/L1", snapRow(max, 0), snapRow(max, 1))
	}
}

func TestPaneTitleFromOSC(t *testing.T) {
	pty := newFakePTY("\x1b]0;my-title\x07done")
	p := NewPane(pty, 10, 2, 1000)
	if err := p.Run(); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if got := p.Title(); got != "my-title" {
		t.Errorf("Title() = %q, want %q", got, "my-title")
	}
}

func TestPaneWriteForwardsToPTY(t *testing.T) {
	pty := newFakePTY("")
	p := NewPane(pty, 10, 3, 1000)

	if _, err := p.Write([]byte("ls\r")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if got := pty.in.String(); got != "ls\r" {
		t.Errorf("pty received %q, want %q", got, "ls\r")
	}
}

func TestPaneResize(t *testing.T) {
	pty := newFakePTY("")
	p := NewPane(pty, 10, 3, 1000)

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
