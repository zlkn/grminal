package vte

import "testing"

// cellsOf turns a string into a row of cells, one rune per cell.
func cellsOf(s string) []Cell {
	rs := []rune(s)
	row := make([]Cell, len(rs))
	for i, r := range rs {
		row[i] = Cell{Rune: r}
	}
	return row
}

// liveSnap builds a Cols×len(rows) live snapshot from equal-width row strings,
// with a visible cursor at (0,0).
func liveSnap(cols int, rows ...string) Snapshot {
	cells := make([]Cell, 0, cols*len(rows))
	for _, r := range rows {
		cells = append(cells, cellsOf(r)...)
	}
	return Snapshot{Cols: cols, Rows: len(rows), Cells: cells, CursorVisible: true}
}

// rowString reads back visible row y of a snapshot as a string.
func rowString(s Snapshot, y int) string {
	var out []rune
	for _, c := range s.Row(y) {
		out = append(out, c.Rune)
	}
	return string(out)
}

func TestScrolledSnapshotOffsetZeroReturnsLive(t *testing.T) {
	live := liveSnap(3, "LA0", "LB0", "LC0")
	history := [][]Cell{cellsOf("H00")}

	got := ScrolledSnapshot(history, live, 0)
	if !got.CursorVisible {
		t.Error("offset 0 must preserve the live cursor visibility")
	}
	for y := 0; y < 3; y++ {
		if rowString(got, y) != rowString(live, y) {
			t.Errorf("offset 0 row %d = %q, want live %q", y, rowString(got, y), rowString(live, y))
		}
	}
}

func TestScrolledSnapshotMixesHistoryAndLive(t *testing.T) {
	live := liveSnap(3, "LA0", "LB0", "LC0")
	history := [][]Cell{cellsOf("H00"), cellsOf("H10"), cellsOf("H20")}

	// offset 1: top = 3-1 = 2 → [H20, LA0, LB0].
	got := ScrolledSnapshot(history, live, 1)
	want := []string{"H20", "LA0", "LB0"}
	for y, w := range want {
		if rowString(got, y) != w {
			t.Errorf("row %d = %q, want %q", y, rowString(got, y), w)
		}
	}
	if got.CursorVisible {
		t.Error("cursor must be hidden while scrolled into history")
	}
}

func TestScrolledSnapshotClampsOffset(t *testing.T) {
	live := liveSnap(3, "LA0", "LB0", "LC0")
	history := [][]Cell{cellsOf("H00"), cellsOf("H10"), cellsOf("H20")}

	// offset 5 > len(history) 3 → clamps to 3 → all history rows shown.
	got := ScrolledSnapshot(history, live, 5)
	want := []string{"H00", "H10", "H20"}
	for y, w := range want {
		if rowString(got, y) != w {
			t.Errorf("row %d = %q, want %q", y, rowString(got, y), w)
		}
	}
}

func TestScrolledSnapshotNormalizesWidth(t *testing.T) {
	live := liveSnap(3, "LA0", "LB0", "LC0")
	// Short row padded to width 3; long row truncated to width 3.
	history := [][]Cell{cellsOf("XY"), cellsOf("ABCDE"), cellsOf("H20")}

	got := ScrolledSnapshot(history, live, 3)
	want := []string{"XY ", "ABC", "H20"} // pad with blank space, truncate overflow
	for y, w := range want {
		if rowString(got, y) != w {
			t.Errorf("row %d = %q, want %q", y, rowString(got, y), w)
		}
	}
}
