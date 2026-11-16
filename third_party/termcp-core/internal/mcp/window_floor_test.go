package mcp

import (
	"strings"
	"testing"
)

// TestAWindowTooNarrowForOneCharacterIsRefused.
//
// A byte window never ends inside a character: the bytes that would have been cut stay
// unread, so the next read delivers them whole. That rule has no answer for a window
// smaller than one character — it withholds the same bytes on every call and the cursor
// never moves, so the reply is an unsatisfied range on every call, which is exactly what
// "there is no data here" looks like. A caller paging one byte at a time against a stream
// that starts with a multi-byte character loops forever and is told nothing.
func TestAWindowTooNarrowForOneCharacterIsRefused(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "printf '\344\270\255TAIL'")

	for _, n := range []int{1, 2, 3} {
		got := termText(t, s, map[string]any{"cmd": "read " + h, "bytes_limit": n, "timeout": 2})
		if !strings.HasPrefix(got, "TERM/1 400") {
			t.Errorf("bytes_limit %d was accepted:\n%s", n, got)
		}
		// The refusal has to teach, or the caller's next move is to retry the same call.
		if !strings.Contains(got, "--cursor") {
			t.Errorf("bytes_limit %d was refused without saying what to do instead:\n%s", n, got)
		}
	}

	// 0 keeps its own meaning ("the channel only, no bytes"), and the floor itself works.
	if got := termText(t, s, map[string]any{"cmd": "read " + h, "bytes_limit": 0, "timeout": 2}); strings.HasPrefix(got, "TERM/1 4") {
		t.Errorf("bytes_limit 0 must stay legal:\n%s", got)
	}
	if got := termText(t, s, map[string]any{"cmd": "read " + h, "bytes_limit": 4, "timeout": 2}); strings.HasPrefix(got, "TERM/1 4") {
		t.Errorf("bytes_limit 4 must be legal:\n%s", got)
	}
}
