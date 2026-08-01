package app

import "testing"

// blinkPhase splits the cycle into equal on/off halves starting visible, so a
// freshly reset blink (tick 0) always shows the cursor.
func TestBlinkPhase(t *testing.T) {
	cases := []struct {
		tick, period int
		want         bool
	}{
		{0, 30, true},   // a reset cycle starts visible
		{29, 30, true},  // last tick of the on phase
		{30, 30, false}, // off phase
		{59, 30, false},
		{60, 30, true}, // back on
		{0, 0, true},   // no period configured: steady on
		{99, -1, true}, // negative period: steady on
	}
	for _, tc := range cases {
		if got := blinkPhase(tc.tick, tc.period); got != tc.want {
			t.Errorf("blinkPhase(%d, %d) = %t, want %t", tc.tick, tc.period, got, tc.want)
		}
	}
}
