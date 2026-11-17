package mcp

import (
	"strings"
	"testing"
)

// TestAChannelAddressAnswersAboutThatChannel pins issues #142 and #148.
//
// `get <session>:<index>` returned the session record — byte-for-byte the same reply
// as `get <session>`, with only `Path` differing — so nothing in it described the
// channel, and `get <session>:99` returned the same 200. An index that does not exist
// was indistinguishable from a hit, which is the worst shape an answer can have: it
// reads as confirmation.
func TestAChannelAddressAnswersAboutThatChannel(t *testing.T) {
	s := newTestServer(t)
	first := openPipe(t, s, "sleep 30")
	sid := strings.SplitN(first, ":", 2)[0]
	second := termText(t, s, map[string]any{"cmd": "open " + sid})
	// open replies with the new channel's handle; take the one that is not the first.
	other := ""
	for _, line := range strings.Fields(second) {
		if strings.HasPrefix(line, sid+":") && line != first {
			other = line
		}
	}
	if other == "" {
		t.Skipf("could not open a second channel; open answered:\n%s", second)
	}

	got := termText(t, s, map[string]any{"cmd": "get " + other})
	if !strings.Contains(got, "channel: "+other) {
		t.Fatalf("get <channel> does not name the channel it answered about:\n%s", got)
	}
	// The session record's own fields must not be what comes back: that was the bug.
	if strings.Contains(got, "ssh_config: ") {
		t.Errorf("get <channel> answered with the session record:\n%s", got)
	}

	// An index that does not exist is a 404 that says what was available, not a 200
	// carrying somebody else's answer.
	missing := termText(t, s, map[string]any{"cmd": "get " + sid + ":99"})
	if code := statusCode(t, missing); code != 404 {
		t.Errorf("get <session>:99 answered %d, want 404:\n%s", code, missing)
	}
	if !strings.Contains(missing, ":99") || !strings.Contains(missing, "no channel") {
		t.Errorf("the 404 does not say which index was asked for:\n%s", missing)
	}
}

// TestLsSessionWithAnIndexIsAnsweredNotDropped pins the other half of #148.
//
// `ls <session> 5` dropped the index and answered the whole listing — or, on a
// session with no live channels, the global "(nothing is running)", which is a
// sentence about a different collection entirely. A caller using a stale index to ask
// "is my channel up" read that as "the session is dead".
func TestLsSessionWithAnIndexIsAnsweredNotDropped(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sleep 30")
	sid := strings.SplitN(h, ":", 2)[0]

	got := termText(t, s, map[string]any{"cmd": "ls " + sid + " 5"})
	if code := statusCode(t, got); code < 400 {
		t.Fatalf("`ls <session> 5` answered %d; the index was dropped:\n%s", code, got)
	}
	if !strings.Contains(got, "get "+sid+":5") {
		t.Errorf("the refusal does not come with the address that answers about one channel:\n%s", got)
	}
	// And the address it recommends must be the one that exists.
	if out := termText(t, s, map[string]any{"cmd": "get " + sid + ":1"}); statusCode(t, out) != 200 {
		t.Errorf("the recommended `get <session>:1` does not work (%d):\n%s", statusCode(t, out), out)
	}
}
