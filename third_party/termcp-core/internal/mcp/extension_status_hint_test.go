package mcp

import (
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/extensions"
)

// TestEveryTransitionalStatusCarriesTheNextHint is the deterministic half of the bug the
// stress run found.
//
// The test that caught it (TestCreateExtensionReportsStartingNotUnavailable) races the
// reconciler: whether the status is still empty or already `probing` depends on who gets
// there first, and the hint disappeared in exactly the second case — the common one, since
// the create itself triggers the walk. A race is a bad place to pin a rule, so the rule is
// pinned here against the status words directly.
func TestEveryTransitionalStatusCarriesTheNextHint(t *testing.T) {
	const name = "fresh"
	cases := []struct {
		name    string
		st      extensions.Status
		enabled bool
		word    string
		hint    bool
	}{
		{
			// Nobody has walked yet: the fallback, which is what the hint was written for.
			name: "not yet walked", st: extensions.Status{}, enabled: true,
			word: extensions.StatusProbing, hint: true,
		},
		{
			// The reconciler picked it up. Same thing to the caller, same hint — and this
			// is the case the early return used to swallow.
			name: "the reconciler is on it", st: extensions.Status{Status: extensions.StatusProbing}, enabled: true,
			word: extensions.StatusProbing, hint: true,
		},
		{
			// A session exists while it is still being probed: still transitional.
			name: "probing with a session", st: extensions.Status{Status: extensions.StatusProbing, SessionID: "s1"}, enabled: true,
			word: extensions.StatusProbing, hint: true,
		},
		{
			// Tried and failed: "unavailable" is the word that sends the caller to the reason.
			name: "tried and failed", st: extensions.Status{Status: extensions.StatusUnavailable, Failures: []extensions.Failure{{Stage: "start", Reason: "boom"}}}, enabled: true,
			word: extensions.StatusUnavailable, hint: false,
		},
		{
			// Up: there is nothing to look forward to.
			name: "available", st: extensions.Status{Status: extensions.StatusAvailable, SessionID: "s1"}, enabled: true,
			word: extensions.StatusAvailable, hint: false,
		},
		{
			// Nothing will ever probe it, so "starting" would be a promise nobody keeps.
			name: "disabled", st: extensions.Status{Status: extensions.StatusUnavailable}, enabled: false,
			word: extensions.StatusDisabled, hint: false,
		},
	}
	for _, c := range cases {
		word, next := extensionStatusWord(name, c.st, c.enabled)
		if word != c.word {
			t.Errorf("%s: word = %q, want %q", c.name, word, c.word)
		}
		if c.hint && !strings.Contains(next, name) {
			t.Errorf("%s: the next-hint must name the extension so it can be pasted: %q", c.name, next)
		}
		if !c.hint && next != "" {
			t.Errorf("%s: nothing to look forward to, but the hint says %q", c.name, next)
		}
	}
}
