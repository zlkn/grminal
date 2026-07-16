package app

import "testing"

func TestFormatTabLabels(t *testing.T) {
	// Alt not held: titles only, empty title falls back to its number.
	got := formatTabLabels([]string{"", "vim", "htop"}, false)
	want := []string{"1", "vim", "htop"}
	assertLabels(t, got, want)

	// Alt held: numbers prefix titled tabs; untitled tab still shows its number.
	got = formatTabLabels([]string{"", "vim", "htop"}, true)
	want = []string{"1", "2 vim", "3 htop"}
	assertLabels(t, got, want)
}

func TestFormatTabLabelsEmpty(t *testing.T) {
	if got := formatTabLabels(nil, false); len(got) != 0 {
		t.Errorf("formatTabLabels(nil) = %v, want empty", got)
	}
}

func assertLabels(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}
