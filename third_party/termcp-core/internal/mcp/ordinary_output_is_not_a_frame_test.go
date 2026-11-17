package mcp

import (
	"strings"
	"testing"
)

// ORDINARY OUTPUT IS ALLOWED TO HOLD JSON (#184).
//
// The scan for a frame used to ask "does this parse", which is a different question
// from "is this a frame" — and the everyday wrong answer is a PowerShell error line:
// driving chrome-devtools-mcp on a box, a stuck tool call printed
//
//	InputObject[0]: some text
//
// and `read <channel> --frame` answered 200 with the body `[0]`. `[0]` is legal JSON,
// so the criterion was satisfied and the caller was handed something it would read as
// an answer: a peer that replied with an empty array. That is worse than "not yet",
// because "not yet" is a fact the caller can act on and a wrong frame is not.
//
// The criterion is now the message: an object with `id` or `method`, or an array of
// them. Nothing was lost — the cursor advances only past the bytes RETURNED, so the
// line stays in the buffer and the reply behind it is still delivered.
func TestOrdinaryOutputHoldingABracketedNumberIsNotAFrame(t *testing.T) {
	s := newTestServer(t)
	// The real reply is behind the noise, which is what makes this a test of the scan
	// rather than of the shape check alone.
	h := openPipe(t, s, `sh -c 'printf "InputObject[0]: some text\n"; `+
		`printf "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n"'`)
	waitForShellToExit(t, s, shellIDFromHandle(t, s, h))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{"cmd": "read " + h + " --frame"})
	if strings.Contains(got, "body\":\"[0]\"") || strings.Contains(got, "\n[0]\n") {
		t.Fatalf("the bracketed number was returned as a frame:\n%s", got)
	}
	if !strings.Contains(got, `"ok":true`) {
		t.Fatalf("the reply behind the noise was not delivered:\n%s", got)
	}
}

// AND WHEN THERE IS NO MESSAGE, THE ANSWER NAMES THE BYTES. A 204 that says only "no
// complete frame yet" is the sentence a caller retries for ever: the bytes in front
// of it will never become a frame, and only the caller can tell whether they are a
// banner, a log line, or the peer's way of reporting a failure.
func TestARecvWithNoMessageQuotesWhatItFound(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, `sh -c 'printf "InputObject[0]: some text\n"'`)
	waitForShellToExit(t, s, shellIDFromHandle(t, s, h))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{"cmd": "read " + h + " --frame", "timeout": 1})
	if !statusLineIs(got, "204") {
		t.Fatalf("a buffer holding no message must answer 204, not a body:\n%s", got)
	}
	if !strings.Contains(got, "InputObject[0]") {
		t.Errorf("the 204 did not quote the bytes it looked at:\n%s", got)
	}
}

// A COMPLETE VALUE THAT IS NOT A MESSAGE IS NAMED AS SUCH. `[0]` on a line of its own
// parses, so "not yet" would send the caller back to wait for bytes that already
// arrived and will never change shape.
func TestAValueThatIsNotAMessageSaysSo(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, `sh -c 'printf "[0]\n"'`)
	waitForShellToExit(t, s, shellIDFromHandle(t, s, h))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{"cmd": "read " + h + " --frame", "timeout": 1})
	if !statusLineIs(got, "204") {
		t.Fatalf("`[0]` is not a frame:\n%s", got)
	}
	if !strings.Contains(got, "not a JSON-RPC message") {
		t.Errorf("the 204 did not say the bytes are JSON but not a message:\n%s", got)
	}
}
