package vte_test

import "testing"

func TestDECSTBMBoundsScrollUp(t *testing.T) {
	g := feed(3, 4, "1\r\n2\r\n3\r\n4\x1b[2;3r\x1b[3;1H\r\n")
	if row(g, 0) != "1  " || row(g, 1) != "3  " || row(g, 2) != "   " || row(g, 3) != "4  " {
		t.Errorf("rows = %q / %q / %q / %q, want 1/3/blank/4",
			row(g, 0), row(g, 1), row(g, 2), row(g, 3))
	}
}

func TestCUUConformedToScrollRegion(t *testing.T) {
	g := feed(3, 4, "\x1b[2;3r\x1b[2;1H\x1b[5A")
	if _, cy := g.Cursor(); cy != 1 {
		t.Errorf("cursor Y = %d, want 1 (top margin)", cy)
	}
}
