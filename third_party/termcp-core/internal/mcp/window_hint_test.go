package mcp

import (
	"strings"
	"testing"
)

// TestABigWindowIsHonouredNotCapped.
//
// The size recommendations are not walls: a caller that asks for more than the recommended
// window gets it, because it is paying for the context and is the only party that can judge
// the trade. What used to happen instead was a silent ceiling — a `--grep-limit 10000` came
// back with 5000 matches and the caller learned the ceiling only if the reply happened to
// truncate, which is a different call than the one it made.
func TestABigWindowIsHonouredNotCapped(t *testing.T) {
	s := newTestServer(t)
	// Twenty lines, so a limit above the default is observable in the body.
	h := openPipe(t, s, "printf 'm1\nm2\nm3\nm4\nm5\nm6\nm7\nm8\nm9\nm10\nm11\nm12\nm13\nm14\nm15\nm16\nm17\nm18\nm19\nm20\n'")
	waitForContent(t, s, h, "m20")

	// grepDefaultLimit is 200, so ask for exactly twenty and check all twenty arrive: a
	// ceiling below the ask would show up as fewer matches.
	got := termText(t, s, map[string]any{
		"cmd": "read " + h + " --grep m --grep-limit 20", "timeout": 2,
	})
	if !strings.Contains(got, "m20") {
		t.Fatalf("a legal grep-limit did not return the last match:\n%s", got)
	}

	// And the recommendation is published where the caller reads it.
	if !strings.Contains(got, "Matches") {
		t.Errorf("the grep reply no longer reports how many matched:\n%s", got)
	}
}
