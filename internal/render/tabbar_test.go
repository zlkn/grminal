package render

import (
	"testing"

	"github.com/yzolkin/go-vte/internal/config"
)

func TestTabSegments(t *testing.T) {
	tests := []struct {
		width, n int
	}{
		{100, 1},
		{100, 3},
		{101, 2},
		{7, 3},
	}
	for _, tc := range tests {
		segs := tabSegments(tc.width, tc.n)
		if len(segs) != tc.n {
			t.Errorf("tabSegments(%d,%d) len = %d, want %d", tc.width, tc.n, len(segs), tc.n)
			continue
		}
		sum := 0
		for _, w := range segs {
			if w < 0 {
				t.Errorf("tabSegments(%d,%d) has negative width %d", tc.width, tc.n, w)
			}
			sum += w
		}
		if sum != tc.width {
			t.Errorf("tabSegments(%d,%d) sums to %d, want %d", tc.width, tc.n, sum, tc.width)
		}
		// Widths differ by at most 1 (as even as possible).
		min, max := segs[0], segs[0]
		for _, w := range segs {
			if w < min {
				min = w
			}
			if w > max {
				max = w
			}
		}
		if max-min > 1 {
			t.Errorf("tabSegments(%d,%d) = %v, uneven (spread %d)", tc.width, tc.n, segs, max-min)
		}
	}
}

func TestTabSegmentsSingleFillsWidth(t *testing.T) {
	if segs := tabSegments(80, 1); len(segs) != 1 || segs[0] != 80 {
		t.Errorf("tabSegments(80,1) = %v, want [80]", segs)
	}
}

func TestTabSegmentsZero(t *testing.T) {
	if segs := tabSegments(80, 0); segs != nil {
		t.Errorf("tabSegments(80,0) = %v, want nil", segs)
	}
}

func TestGridSizeReservesTabBar(t *testing.T) {
	r, err := NewRenderer(config.Default(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if r.TabBarHeight() <= 0 {
		t.Fatalf("TabBarHeight = %v, want positive", r.TabBarHeight())
	}
	const h = 600
	_, cellH := r.CellSize()
	_, rows := r.GridSize(800, h)
	// The content rows must fit below the reserved tab bar (default padding 0).
	if float64(rows)*cellH > float64(h)-r.TabBarHeight() {
		t.Errorf("rows=%d (%.0fpx) overflow content area %.0fpx (h=%d, bar=%.0f)",
			rows, float64(rows)*cellH, float64(h)-r.TabBarHeight(), h, r.TabBarHeight())
	}
}
