package app

import "testing"

func TestFormatTabLabels(t *testing.T) {
	got := formatTabLabels([]string{"", "vim", "htop"})
	want := []string{"1", "2 vim", "3 htop"}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("label[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestFormatTabLabelsEmpty(t *testing.T) {
	if got := formatTabLabels(nil); len(got) != 0 {
		t.Errorf("formatTabLabels(nil) = %v, want empty", got)
	}
}
