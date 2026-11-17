package session

import "testing"

// TestTheDialTimeoutFloorIsOneSecond.
//
// 30 is the DEFAULT, not a floor. A comment here once read "clamps DialTimeoutSeconds to
// [30, 120]", which described a bound the code did not have, and reading it as the domain
// turned a fast-fail setting into a refusal — a caller that wanted a 5 second dial so a
// script fails fast could no longer say so at all. Reading it as an UPPER bound too kept a
// second wrong answer alive: 600 was stored, echoed back by `get host`, and dialled at 120.
// The clamp now touches one thing and only one: an unset value becomes the default.
func TestTheDialTimeoutFloorIsOneSecond(t *testing.T) {
	cases := []struct{ in, want int }{
		{0, 30},    // unset: the default
		{-5, 30},   // a negative value is a typo, and the field is an int in a config file
		{1, 1},     // the floor, and it is honoured
		{5, 5},     // the fast-fail case the wrong comment would have refused
		{29, 29},   // below the default, still the caller's number
		{30, 30},   // the default
		{120, 120}, // the recommendation, and just a number
		{600, 600}, // above it: honoured, because the link is the caller's to know
	}
	for _, c := range cases {
		if got := ClampDialTimeoutSeconds(c.in); got != c.want {
			t.Errorf("ClampDialTimeoutSeconds(%d) = %d, want %d", c.in, got, c.want)
		}
	}
}
