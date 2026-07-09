package vte

import (
	"strings"
	"testing"
)

// perfChunk is a representative mix of printable text, SGR styling and control
// bytes, sized to stress the hot path.
var perfChunk = []byte(strings.Repeat(
	"\x1b[1;31mERROR\x1b[0m the quick brown fox jumps over the lazy dog 0123456789\r\n",
	32,
))

// TestParserZeroAlloc enforces the headline goal: steady-state parsing does not
// allocate. The grid is pre-allocated, params live in a fixed array, and runes
// decode in place — so Write over an already-sized grid must be alloc-free.
func TestParserZeroAlloc(t *testing.T) {
	g := NewGrid(80, 24)
	p := NewParser(g)
	p.Write(perfChunk) // warm up

	if allocs := testing.AllocsPerRun(200, func() { p.Write(perfChunk) }); allocs != 0 {
		t.Errorf("Write allocated %.1f times/op, want 0", allocs)
	}
}

// BenchmarkParserThroughput reports MB/s and allocs/op for the parser.
// Run: go test ./internal/vte -run=^$ -bench=Throughput -benchmem
func BenchmarkParserThroughput(b *testing.B) {
	g := NewGrid(80, 24)
	p := NewParser(g)
	b.SetBytes(int64(len(perfChunk)))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.Write(perfChunk)
	}
}
