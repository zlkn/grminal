package pty_test

import (
	"bytes"
	"testing"

	"github.com/yzolkin/go-vte/internal/pty"
	"github.com/yzolkin/go-vte/internal/vte"
)

// fakePTY is an in-memory PTY for testing.
type fakePTY struct {
	readBuf  bytes.Buffer
	writeBuf bytes.Buffer
	rows     uint16
	cols     uint16
	closed   bool
}

func (f *fakePTY) Read(b []byte) (int, error)  { return f.readBuf.Read(b) }
func (f *fakePTY) Write(b []byte) (int, error) { return f.writeBuf.Write(b) }
func (f *fakePTY) Resize(r, c uint16) error {
	f.rows, f.cols = r, c
	return nil
}
func (f *fakePTY) Close() error {
	f.closed = true
	return nil
}

func TestPaneCreationAndResize(t *testing.T) {
	fake := &fakePTY{}
	p := pty.NewPane(fake, 80, 24, 100)

	if got := p.Grid().Cols(); got != 80 {
		t.Errorf("Cols = %d, want 80", got)
	}
	if got := p.Grid().Rows(); got != 24 {
		t.Errorf("Rows = %d, want 24", got)
	}

	_ = p.Resize(100, 30)
	if fake.cols != 100 || fake.rows != 30 {
		t.Errorf("PTY resize = %dx%d, want 100x30", fake.cols, fake.rows)
	}
}

func TestPaneRunDrainsInput(t *testing.T) {
	fake := &fakePTY{}
	fake.readBuf.WriteString("hello world\r\n")

	p := pty.NewPane(fake, 80, 24, 100)
	_ = p.Run() // drains until readBuf is empty (EOF)

	snap := p.Snapshot()
	row0 := string(runeSlice(snap.Row(0)))
	if len(row0) < 11 || row0[:11] != "hello world" {
		t.Errorf("row 0 = %q, want prefix %q", row0, "hello world")
	}
}

func runeSlice(cells []vte.Cell) []rune {
	res := make([]rune, len(cells))
	for i, c := range cells {
		res[i] = c.Rune
	}
	return res
}
