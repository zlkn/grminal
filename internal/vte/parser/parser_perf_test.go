package parser_test

import (
	"bytes"
	"testing"

	"github.com/yzolkin/go-vte/internal/vte"
	"github.com/yzolkin/go-vte/internal/vte/parser"
)

func BenchmarkParserThroughput(b *testing.B) {
	chunk := generateBenchmarkPayload(100 * 1024)
	g := vte.NewGrid(120, 40)
	p := parser.NewParser(g)

	b.SetBytes(int64(len(chunk)))
	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		p.Write(chunk)
	}
}

func generateBenchmarkPayload(size int) []byte {
	var buf bytes.Buffer
	buf.Grow(size)
	line := []byte("\x1b[31mhello \x1b[32mworld \x1b[0m\x1b[1;34mfoo bar baz\x1b[0m\r\n")
	for buf.Len() < size {
		buf.Write(line)
	}
	return buf.Bytes()[:size]
}
