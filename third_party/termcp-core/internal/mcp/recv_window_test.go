package mcp

import (
	"strings"
	"testing"
	"time"
)

// ONE RECV MUST BE ABLE TO WAIT AS LONG AS THE CALLER SAID.
//
// The recv loop used to give up after a hardcoded 2 seconds, so an MCP server over
// stdio that took longer than that for a `tools/call` answered 204 "no complete frame
// yet" while the reply was in flight. The caller's only move was to poll — and every
// poll is a whole assistant turn, roughly 1000× the cost of the call (issue #174).
//
// Two things made it invisible: 204 is the same answer whether the peer is slow or
// silent, and the peer-call path right next door (peerCallWindow) had always honoured
// the caller's window. Two waits in one file answering to different clocks.
//
// The probe delays the reply past the old 2s budget with the caller's window above it:
// a single recv must return the frame.
func TestARecvWaitsTheWindowTheCallerAskedFor(t *testing.T) {
	if testing.Short() {
		t.Skip("deliberately spends the old 2s budget")
	}
	s := newTestServer(t)
	// The script sleeps, then writes a frame. The sleep is the whole subject: it is
	// longer than the budget this test exists to remove, and shorter than the window.
	script := writeScript(t, `sleep 3; printf '%s\n' '{"jsonrpc":"2.0","id":9,"result":{"ok":true}}'`)
	h := openPipe(t, s, "sh "+script)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	start := time.Now()
	text := termText(t, s, map[string]any{"cmd": "read " + h + " --frame", "timeout": 20})
	elapsed := time.Since(start)

	if !strings.Contains(text, `"id":9`) {
		t.Fatalf("one recv with a 20s window did not wait out a 3s reply (%v): %s", elapsed, firstLine(text))
	}
	if elapsed < 2*time.Second {
		// Not a failure of the product, a failure of the probe: if the reply had
		// arrived inside the old budget, the test would pass on the OLD code too.
		t.Fatalf("the reply arrived in %v, inside the old 2s budget — the probe proves nothing", elapsed)
	}
}
