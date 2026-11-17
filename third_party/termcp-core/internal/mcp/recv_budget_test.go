package mcp

import (
	"strings"
	"testing"
	"time"
)

// A RECV THAT WAS GIVEN NO WINDOW WAITS THE READ DEFAULT, NOT THE PEER DEFAULT.
//
// The caller's window is the budget now, and the window's own default is two seconds —
// the number every other read of a channel sits on. The peer-call default next door is
// fifteen, and it is fifteen for a reason that does not hold here: a peer call is a
// request/response, while a recv that finds nothing is a read that found nothing. Taking
// the peer default made a silent channel block for fifteen seconds, which is not what a
// caller who named no window asked for and not what this directive used to do.
//
// The gap between the two numbers is wide enough to test without timing races: anything
// under six seconds is the read default, anything near fifteen is the peer default.
func TestARecvWithNoWindowWaitsTheReadDefault(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, `sh -c 'sleep 60'`)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	start := time.Now()
	got := termText(t, s, map[string]any{"cmd": "read " + h + " --frame"})
	elapsed := time.Since(start)

	if !strings.Contains(got, "no complete frame") {
		t.Fatalf("expected the not-yet answer, got:\n%s", got)
	}
	if elapsed > 6*time.Second {
		t.Errorf("a recv given no window waited %v: that is the peer-call default, not the read default", elapsed)
	}
	if elapsed < time.Second {
		t.Errorf("a recv given no window answered in %v, so it never waited at all", elapsed)
	}
	// THE REPLY NAMES THE KNOB. A caller told "not yet" has no way to learn that the wait
	// was this server's choice rather than its own.
	if !strings.Contains(got, "timeout") {
		t.Errorf("the not-yet answer does not say that the wait is the caller's to raise:\n%s", got)
	}
}
