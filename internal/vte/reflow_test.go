package vte

import "testing"

// These tests pin the reflow-on-resize behaviour: soft-wrapped lines are
// rejoined and re-split at the new width, while hard newlines stay put. This is
// what keeps a line that wrapped at a narrow width from staying broken after the
// window grows (the stale-wrap artifact that showed up under tmux splits).

// Growing the width rejoins a line that had soft-wrapped, collapsing its two
// rows back into one.
func TestReflowGrowRejoinsWrappedLine(t *testing.T) {
	g := feed(4, 3, "abcdef") // wraps: "abcd" / "ef"
	if row(g, 0) != "abcd" || row(g, 1) != "ef  " {
		t.Fatalf("precondition rows = %q / %q", row(g, 0), row(g, 1))
	}
	g.Resize(8, 3)
	if got := row(g, 0); got != "abcdef  " {
		t.Errorf("row0 after grow = %q, want %q (rejoined)", got, "abcdef  ")
	}
	if got := row(g, 1); got != "        " {
		t.Errorf("row1 after grow = %q, want blank", got)
	}
}

// Shrinking the width re-splits a line that no longer fits, marking the overflow
// as a soft-wrap continuation.
func TestReflowShrinkSplitsLine(t *testing.T) {
	g := feed(8, 3, "abcdef")
	g.Resize(4, 3)
	if row(g, 0) != "abcd" || row(g, 1) != "ef  " {
		t.Errorf("rows after shrink = %q / %q, want \"abcd\" / \"ef  \"", row(g, 0), row(g, 1))
	}
	if !g.wrapped[1] {
		t.Error("row1 should be marked a soft-wrap continuation after shrink")
	}
}

// A hard newline is a logical boundary: reflow must not merge the two lines even
// when there is room for them on one row.
func TestReflowKeepsHardNewline(t *testing.T) {
	g := feed(4, 3, "ab\r\ncd") // two separate hard lines
	g.Resize(8, 3)
	if row(g, 0) != "ab      " {
		t.Errorf("row0 = %q, want \"ab      \" (not merged)", row(g, 0))
	}
	if row(g, 1) != "cd      " {
		t.Errorf("row1 = %q, want \"cd      \" (not merged)", row(g, 1))
	}
	if g.wrapped[1] {
		t.Error("row1 follows a hard newline; it must not be marked wrapped")
	}
}

// Round-tripping the width restores the original layout: narrow → wide → narrow.
func TestReflowRoundTrip(t *testing.T) {
	g := feed(4, 4, "abcdefghij") // "abcd"/"efgh"/"ij"
	g.Resize(10, 4)
	if got := row(g, 0); got != "abcdefghij" {
		t.Fatalf("row0 after grow = %q, want full line", got)
	}
	g.Resize(4, 4)
	if row(g, 0) != "abcd" || row(g, 1) != "efgh" || row(g, 2) != "ij  " {
		t.Errorf("rows after round-trip = %q / %q / %q", row(g, 0), row(g, 1), row(g, 2))
	}
}

// Trailing blank rows below the content are scratch space: they must not push
// real content off the top when the width shrinks and lines wrap taller.
func TestReflowTrailingBlanksDoNotEvict(t *testing.T) {
	g := feed(8, 4, "abcdef") // one line of content, rows 1-3 blank
	g.Resize(4, 4)
	if row(g, 0) != "abcd" || row(g, 1) != "ef  " {
		t.Errorf("content evicted by trailing blanks: rows = %q / %q", row(g, 0), row(g, 1))
	}
}
