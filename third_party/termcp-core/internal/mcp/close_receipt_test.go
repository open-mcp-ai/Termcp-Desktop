package mcp

import (
	"strings"
	"testing"
)

// TestACloseSaysWhatItClosed pins the last reply a caller gets about an object.
//
// `close <session>` answered 204 with an empty body — the one reply in the surface
// that said nothing — so "closed the session I named" and "closed something,
// somewhere" looked identical. The reported reaction was to run `ls` and see what was
// missing: a call spent confirming instead of advancing, on the step where being wrong
// is most expensive.
//
// The status stays 204 (there is no content, and the REST entrance drops a body on
// 204), so the confirmation is a header both entrances carry.
func TestACloseSaysWhatItClosed(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	sid, _, _ := strings.Cut(h, ":")

	// A CHANNEL close names the channel and its own name, because `:1` alone says
	// which slot closed and nothing about what was in it.
	termText(t, s, map[string]any{"cmd": "close " + h})
	closed := headerValueOr(termText(t, s, map[string]any{"cmd": "open " + sid + " --name zz-explore"}), "Handle", "")
	if closed == "" {
		t.Fatal("could not open a second channel")
	}
	reply := termText(t, s, map[string]any{"cmd": "close " + closed})
	if !strings.HasPrefix(reply, "TERM/1 204") {
		t.Fatalf("closing a channel is still a 204:\n%s", reply)
	}
	got := headerValueOr(reply, "Closed", "")
	if got == "" {
		t.Fatalf("the close reply does not say what it closed:\n%s", reply)
	}
	if !strings.Contains(got, closed) {
		t.Errorf("the confirmation names %q, not the channel that was closed (%s)", got, closed)
	}
	if !strings.Contains(got, "zz-explore") {
		t.Errorf("the confirmation drops the channel's own name: %q", got)
	}

	// A SESSION close names the session, by the name the caller gave it.
	//
	// `connect --name` names the SESSION and `open --name` names the CHANNEL, which is
	// the distinction the first half of this test relies on: they are different
	// objects, and a close has to name the one it closed.
	named := termText(t, s, map[string]any{"cmd": "connect internal --name zz-keep"})
	other := headerValueOr(named, "Handle", "")
	if other == "" {
		t.Fatalf("could not open a named session:\n%s", named)
	}
	otherSID, _, _ := strings.Cut(other, ":")
	sess := termText(t, s, map[string]any{"cmd": "close " + otherSID})
	got = headerValueOr(sess, "Closed", "")
	if got == "" {
		t.Fatalf("closing a session says nothing about what closed:\n%s", sess)
	}
	if !strings.Contains(got, otherSID) {
		t.Errorf("the confirmation must name the session id, got %q", got)
	}
	// The name is how the caller thinks of it; the uuid is how the wire does.
	if !strings.Contains(got, "zz-keep") {
		t.Errorf("the confirmation drops the session's name: %q", got)
	}
}
