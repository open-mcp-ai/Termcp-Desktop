package mcp

import (
	"strings"
	"testing"
)

// TestPsLearnModeTeachesTheStateVerbs pins that `ps` with learn_mode expands what
// can be done with the addresses it just printed.
//
// learn_mode normally attaches the CURRENT directive's usage, which is useless on ps:
// a caller that just listed addresses is wondering what consumes them, and the verbs
// that do are exactly the ones the listing cannot show. `close a1f3:1` archives a
// channel while `rm a1f3` erases a session — a distinction an id and a status column
// do not carry.
func TestPsLearnModeTeachesTheStateVerbs(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{"cmd": "ls", "learn_mode": true})
	for _, want := range []string{"get ", "close ", "rm ", "resize "} {
		if !strings.Contains(got, want) {
			t.Errorf("ls learn_mode must name the state verb %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "what an address") {
		t.Errorf("the expansion needs a heading that says what it is:\n%s", got)
	}

	// Without learn_mode the listing stays a listing.
	plain := termText(t, s, map[string]any{"cmd": "ls"})
	if strings.Contains(plain, "what an address") {
		t.Errorf("a plain ps must not carry the teaching block:\n%s", plain)
	}
}
