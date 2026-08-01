package vte_test

import "testing"

func TestReflowWrapOnResize(t *testing.T) {
	g := feed(10, 4, "hello world")
	g.Resize(5, 4)

	if got := row(g, 0); got != "hello" {
		t.Errorf("row0 = %q, want \"hello\"", got)
	}
	if got := row(g, 1); got != " worl" {
		t.Errorf("row1 = %q, want \" worl\"", got)
	}
	if got := row(g, 2); got != "d    " {
		t.Errorf("row2 = %q, want \"d    \"", got)
	}
}

func TestReflowPreservesCursor(t *testing.T) {
	g := feed(10, 4, "hello world")
	g.Resize(5, 4)

	if cx, cy := g.Cursor(); cx != 1 || cy != 2 {
		t.Errorf("cursor after reflow = (%d,%d), want (1,2)", cx, cy)
	}
}
