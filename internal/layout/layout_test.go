package layout

import "testing"

// overlap reports whether two rects share any area.
func overlap(a, b Rect) bool {
	return a.X < b.X+b.W && b.X < a.X+a.W && a.Y < b.Y+b.H && b.Y < a.Y+a.H
}

// assertTiling verifies the rects exactly tile bounds: every rect lies inside
// bounds, no two overlap, and their areas sum to the bounds area (⇒ no gaps).
func assertTiling(t *testing.T, bounds Rect, rects map[int]Rect) {
	t.Helper()
	total := 0
	list := make([]Rect, 0, len(rects))
	for id, r := range rects {
		if r.X < bounds.X || r.Y < bounds.Y || r.X+r.W > bounds.X+bounds.W || r.Y+r.H > bounds.Y+bounds.H {
			t.Errorf("pane %d rect %+v escapes bounds %+v", id, r, bounds)
		}
		total += r.Area()
		list = append(list, r)
	}
	for i := 0; i < len(list); i++ {
		for j := i + 1; j < len(list); j++ {
			if overlap(list[i], list[j]) {
				t.Errorf("rects overlap: %+v and %+v", list[i], list[j])
			}
		}
	}
	if total != bounds.Area() {
		t.Errorf("area sum = %d, want %d (gaps or overlap)", total, bounds.Area())
	}
}

func TestNewSinglePaneFillsBounds(t *testing.T) {
	b := Rect{0, 0, 100, 50}
	l := New(b)

	rects := l.Rects()
	if len(rects) != 1 {
		t.Fatalf("got %d panes, want 1", len(rects))
	}
	if rects[0] != b {
		t.Errorf("pane 0 = %+v, want %+v", rects[0], b)
	}
	assertTiling(t, b, rects)
}

func TestSplitVerticalTiles(t *testing.T) {
	b := Rect{0, 0, 100, 50}
	l := New(b)
	id, ok := l.SplitVertical(0)
	if !ok {
		t.Fatal("SplitVertical failed")
	}

	rects := l.Rects()
	assertTiling(t, b, rects)
	if rects[0] != (Rect{0, 0, 50, 50}) {
		t.Errorf("left = %+v, want {0 0 50 50}", rects[0])
	}
	if rects[id] != (Rect{50, 0, 50, 50}) {
		t.Errorf("right = %+v, want {50 0 50 50}", rects[id])
	}
}

func TestSplitHorizontalTiles(t *testing.T) {
	b := Rect{0, 0, 100, 50}
	l := New(b)
	id, ok := l.SplitHorizontal(0)
	if !ok {
		t.Fatal("SplitHorizontal failed")
	}

	rects := l.Rects()
	assertTiling(t, b, rects)
	if rects[0] != (Rect{0, 0, 100, 25}) {
		t.Errorf("top = %+v, want {0 0 100 25}", rects[0])
	}
	if rects[id] != (Rect{0, 25, 100, 25}) {
		t.Errorf("bottom = %+v, want {0 25 100 25}", rects[id])
	}
}

func TestNestedTilingInvariant(t *testing.T) {
	b := Rect{0, 0, 120, 60}
	l := New(b)
	right, _ := l.SplitVertical(0) // two columns
	l.SplitHorizontal(right)       // stack the right column
	l.SplitHorizontal(0)           // stack the left column too

	rects := l.Rects()
	if len(rects) != 4 {
		t.Fatalf("got %d panes, want 4", len(rects))
	}
	assertTiling(t, b, rects)
}

func TestOddSizeTilesWithoutGaps(t *testing.T) {
	// 101 is not divisible by 2; the split must still cover every column.
	b := Rect{0, 0, 101, 51}
	l := New(b)
	l.SplitVertical(0)
	assertTiling(t, b, l.Rects())
}

func TestResizeRedistributes(t *testing.T) {
	l := New(Rect{0, 0, 100, 50})
	id, _ := l.SplitVertical(0)

	nb := Rect{0, 0, 200, 100}
	l.Resize(nb)

	rects := l.Rects()
	assertTiling(t, nb, rects)
	if rects[0].W != 100 || rects[id].W != 100 {
		t.Errorf("widths after resize = %d / %d, want 100 / 100", rects[0].W, rects[id].W)
	}
}

func TestMinSizeBlocksSplit(t *testing.T) {
	// A 1×1 area cannot be split into two panes.
	l := New(Rect{0, 0, 1, 1})
	if _, ok := l.SplitVertical(0); ok {
		t.Error("SplitVertical should fail on width 1")
	}
	if _, ok := l.SplitHorizontal(0); ok {
		t.Error("SplitHorizontal should fail on height 1")
	}
}

func TestPaneAt(t *testing.T) {
	l := New(Rect{0, 0, 100, 50})
	right, _ := l.SplitVertical(0)

	if id, ok := l.PaneAt(10, 10); !ok || id != 0 {
		t.Errorf("PaneAt(10,10) = %d,%v, want 0,true", id, ok)
	}
	if id, ok := l.PaneAt(60, 10); !ok || id != right {
		t.Errorf("PaneAt(60,10) = %d,%v, want %d,true", id, ok, right)
	}
	if _, ok := l.PaneAt(999, 999); ok {
		t.Error("PaneAt outside bounds should be false")
	}
}

func TestSplitUnknownPane(t *testing.T) {
	l := New(Rect{0, 0, 100, 50})
	if _, ok := l.SplitVertical(42); ok {
		t.Error("splitting a nonexistent pane should fail")
	}
}
