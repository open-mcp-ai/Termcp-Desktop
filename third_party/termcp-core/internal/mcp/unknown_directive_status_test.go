package mcp

import (
	"strings"
	"testing"
)

// TestAnUnknownDirectiveAnswersTheSameWayWhicheverWayYouAsk pins issue #145.
//
// The same missing verb, the same message, two different statuses:
//
//	help bogusverb  -> 404 unknown directive: bogusverb
//	bogusverb       -> 400 unknown directive: bogusverb
//
// A caller that reads 404 as "this build has no such verb" and 400 as "I wrote
// this line wrong" reaches opposite conclusions about the same absence depending
// on whether it asked for help first — and the body is byte-identical, so the
// status is the only signal there is. Nothing here is a resource: the caller named
// a directive this build does not have, which is a bad request either way.
func TestAnUnknownDirectiveAnswersTheSameWayWhicheverWayYouAsk(t *testing.T) {
	s := newTestServer(t)

	direct := termText(t, s, map[string]any{"cmd": "bogusverb"})
	viaHelp := termText(t, s, map[string]any{"cmd": "help bogusverb"})

	directCode := statusCode(t, direct)
	helpCode := statusCode(t, viaHelp)
	if directCode != helpCode {
		t.Errorf("`bogusverb` answered %d but `help bogusverb` answered %d; the same absence must not have two answers:\n%s\n%s",
			directCode, helpCode, direct, viaHelp)
	}
	if directCode != 400 {
		t.Errorf("status = %d, want 400: naming a directive this build does not have is a bad request", directCode)
	}
	// The message is what the caller acts on, so it must survive the code change.
	if !strings.Contains(viaHelp, "unknown directive: bogusverb") {
		t.Errorf("the help path stopped naming the directive it could not find:\n%s", viaHelp)
	}
	// A verb that DOES exist must still be answered as one: the fix is the status
	// of the miss, not a blanket 400 that swallows the hit.
	if ok := termText(t, s, map[string]any{"cmd": "help ls"}); statusCode(t, ok) != 200 {
		t.Errorf("`help ls` answered %d, want 200:\n%s", statusCode(t, ok), ok)
	}
}
