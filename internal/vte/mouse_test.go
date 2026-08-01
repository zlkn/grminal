package vte_test

import (
	"bytes"
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

func TestMouseModeEncoding(t *testing.T) {
	g := vte.NewGrid(10, 10)
	p := parser.NewParser(g)
	var reply bytes.Buffer
	p.SetReply(&reply)

	p.Write([]byte("\x1b[?1000h")) // enable X11 mouse mode
	p.Write([]byte("\x1b[?1006h")) // SGR mode

	ev := vte.MouseEvent{
		Col:    2,
		Row:    3,
		Button: vte.MouseLeft,
	}

	b := g.EncodeMouse(ev)
	if got := string(b); got != "\x1b[<0;3;4M" {
		t.Errorf("EncodeMouse = %q, want %q", got, "\x1b[<0;3;4M")
	}

	ev.Release = true
	b = g.EncodeMouse(ev)
	if got := string(b); got != "\x1b[<0;3;4m" {
		t.Errorf("EncodeMouse release = %q, want %q", got, "\x1b[<0;3;4m")
	}
}
