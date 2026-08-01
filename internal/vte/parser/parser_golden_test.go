package parser_test

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/testutil"
	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

// TestParserGolden snapshots the grid produced by a realistic mixed stream
// (clear, cursor positioning, SGR, tabs, wrapping).
func TestParserGolden(t *testing.T) {
	g := vte.NewGrid(24, 6)
	p := parser.NewParser(g)

	stream := "" +
		"\x1b[2J\x1b[H" + // clear screen, home
		"\x1b[1;34mGO-VTE\x1b[0m terminal\r\n" + // bold-blue title
		"\x1b[32mstatus:\x1b[0m ok\r\n" + // green label
		"col\tval\r\n" + // tab stop
		"\x1b[5;3Hplaced" // absolute cursor position

	p.Write([]byte(stream))
	testutil.Golden(t, "parser_mixed", []byte(g.Dump()))
}
