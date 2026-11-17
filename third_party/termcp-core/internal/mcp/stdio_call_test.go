package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// A ROUND TRIP IS ONE CALL, WHICHEVER WAY THE PEER WAS STARTED.
//
// `call` against a URL peer waits for the reply (peerChannel sets a deadline for it);
// against a stdio peer it wrote the frame and answered 202, so the same verb behaved
// differently depending on how the peer had been started — and the caller could not
// see which it had. Every round trip cost two calls, and the second one needed the
// caller to correlate replies by hand (issue #175).
//
// The peer here is a script that answers one request: the sleep proves the wait is a
// wait, and the id proves the reply that comes back is the answer to this call.
func TestAStdioCallWaitsForItsOwnReply(t *testing.T) {
	s := newTestServer(t)
	script := writeScript(t, `read line; printf '%s\n' '{"jsonrpc":"2.0","id":7,"result":{"ok":true}}'`)
	h := openPipe(t, s, "sh "+script)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "call " + h + ` {"jsonrpc":"2.0","id":7,"method":"tools/list"}`,
		"timeout": 20,
	})
	if !strings.Contains(got, `"result"`) {
		t.Fatalf("one call did not return the reply:\n%s", got)
	}
	// THE STATUS LINE, NOT THE WHOLE REPLY. This assertion used to be a substring test
	// for "202" over the entire reply, which the command echo satisfied by accident:
	// the random session id in `Path: call 548e1202-666:1 {...}` contains it. The test
	// then failed on CI for the id it happened to draw and passed locally for every
	// other one — this repo's own rule that an assertion must not be satisfiable by
	// the echo.
	if !strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("call answered something other than the reply:\n%s", got)
	}
}

// --send KEEPS THE OLD SHAPE, and it is the form that says "do not wait".
//
// Nothing was taken away by making `call` wait: the caller that wants fire-and-forget
// writes --send, and the CDP path already honoured it.
func TestSendDoesNotWait(t *testing.T) {
	s := newTestServer(t)
	script := writeScript(t, `read line; sleep 30`)
	h := openPipe(t, s, "sh "+script)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "call " + h + ` --send {"jsonrpc":"2.0","id":7,"method":"tools/list"}`,
		"timeout": 20,
	})
	if !strings.HasPrefix(got, "TERM/1 202") {
		t.Fatalf("--send waited for a reply that was never coming:\n%s", got)
	}
}

// A FRAME THAT IS NOT THE ANSWER IS LABELLED, NOT PASSED OFF AS ONE.
//
// Replies arrive in completion order — correct JSON-RPC, and the reason a caller used
// to have to keep a queue. When the frame that lands first belongs to an earlier
// request, the reply says so instead of letting the caller believe it got its answer.
func TestAFrameThatAnswersSomethingElseSaysSo(t *testing.T) {
	s := newTestServer(t)
	// The peer answers id 1 first, then whatever was asked. Two writes, one reply each:
	// the first call takes the first frame, the second call takes the next.
	script := writeScript(t, `read line; printf '%s\n' '{"jsonrpc":"2.0","id":1,"result":{"stale":true}}'`)
	h := openPipe(t, s, "sh "+script)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "call " + h + ` {"jsonrpc":"2.0","id":99,"method":"tools/list"}`,
		"timeout": 20,
	})
	if !strings.Contains(got, "stale") {
		t.Fatalf("the frame the peer wrote was lost:\n%s", got)
	}
	if !strings.Contains(got, "id 1") || !strings.Contains(got, "id 99") {
		t.Fatalf("a reply to another id was not labelled as such:\n%s", got)
	}
}

// A NOTIFICATION HAS NO ANSWER, so waiting for one would be a lie. The reply says
// which it is rather than sitting on the caller's window.
func TestANotificationIsNotWaitedFor(t *testing.T) {
	s := newTestServer(t)
	script := writeScript(t, `read line; sleep 30`)
	h := openPipe(t, s, "sh "+script)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "call " + h + ` {"jsonrpc":"2.0","method":"notifications/initialized"}`,
		"timeout": 20,
	})
	if !strings.Contains(got, "notification") {
		t.Fatalf("a frame with no id was waited on:\n%s", got)
	}
}

// THE ID IS COMPARED AS SPELLED. A JSON-RPC id may be a string or a number, and `"1"`
// and `1` are different ids to the peer. A comparison that normalised them would call
// two frames the same one.
func TestTheIDComparisonKeepsTheSpelling(t *testing.T) {
	cases := []struct {
		frame string
		want  string
	}{
		{`{"jsonrpc":"2.0","id":1}`, "1"},
		{`{"jsonrpc":"2.0","id":"1"}`, `"1"`},
		// `id: null` is an error response's id, never a request's, so it is read as "no
		// id": there is nothing to wait for, and saying so beats waiting out the window.
		{`{"jsonrpc":"2.0","id":null}`, ""},
		{`{"jsonrpc":"2.0","method":"x"}`, ""},
		{`not json`, ""},
		{`{"jsonrpc":"2.0","id":{"nested":1}}`, `{"nested":1}`},
	}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%.20s", c.frame), func(t *testing.T) {
			if got := frameID(c.frame); got != c.want {
				t.Fatalf("frameID(%s) = %q, want %q", c.frame, got, c.want)
			}
		})
	}
}
