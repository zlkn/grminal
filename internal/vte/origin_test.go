package vte_test

import "testing"

func TestOriginModeConlinesCUPToScrollRegion(t *testing.T) {
	g := feed(10, 10, "\x1b[3;8r\x1b[?6h\x1b[H") // region 3..8, DEOM on, CUP home
	if cx, cy := g.Cursor(); cx != 0 || cy != 2 {
		t.Errorf("cursor after DEOM home = (%d,%d), want (0,2)", cx, cy)
	}

	g = feed(10, 10, "\x1b[3;8r\x1b[?6h\x1b[2;4H") // 1-based (row 2 of region, col 4)
	if cx, cy := g.Cursor(); cx != 3 || cy != 3 {
		t.Errorf("cursor after region-relative CUP = (%d,%d), want (3,3)", cx, cy)
	}
}
