package app

import "testing"

func TestShouldRepeat(t *testing.T) {
	const delay, interval = 30, 2
	tests := []struct {
		name            string
		d, delay, intvl int
		want            bool
	}{
		{"before delay", 29, delay, interval, false},
		{"at delay (first repeat)", 30, delay, interval, true},
		{"between repeats", 31, delay, interval, false},
		{"second repeat", 32, delay, interval, true},
		{"press frame never repeats", 1, delay, interval, false},
		{"disabled by zero delay", 100, 0, interval, false},
		{"disabled by negative delay", 100, -5, interval, false},
		{"interval clamped to 1", 42, 40, 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shouldRepeat(tt.d, tt.delay, tt.intvl); got != tt.want {
				t.Errorf("shouldRepeat(%d, %d, %d) = %v, want %v", tt.d, tt.delay, tt.intvl, got, tt.want)
			}
		})
	}
}
