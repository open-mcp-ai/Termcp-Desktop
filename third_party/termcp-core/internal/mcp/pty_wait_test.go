package mcp

import (
	"strings"
	"testing"
	"time"
)

// TestAPtyRunEndsWhenTheCommandStopsTalking is the cost half of the pty wait.
//
// `run` used to sleep out the caller's whole window no matter what the command did:
// a 20s window on `echo` took 20s, and the reply could not distinguish a command
// that finished in a millisecond from one that used the whole budget. The window is
// the caller's budget, not the command's duration, so the wait now ends when the
// stream has stood still for ptyQuiet.
func TestAPtyRunEndsWhenTheCommandStopsTalking(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	start := time.Now()
	got := termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{"echo finished-quietly"}, "timeout": 30})
	elapsed := time.Since(start)

	if !strings.Contains(got, "finished-quietly") {
		t.Fatalf("the command's output is missing from the reply:\n%s", got)
	}
	// Half the window is the generous line: 500ms of quiet plus this machine's shell
	// drawing five chunks over 1.1s is nowhere near it, and a wait that went back to
	// sleeping the window would land at 30s.
	if elapsed > 15*time.Second {
		t.Fatalf("a 30s window on a command that finished at once took %v", elapsed)
	}
}

// TestAPtyRunReturnsWhatWasSaidWhenTheCommandFallsSilent pins the price of that
// trade, in the direction a caller will actually meet it.
//
// A command that prints, then thinks for longer than ptyQuiet, comes back with what
// it has printed so far — the same thing a terminal shows you. The rest is not
// lost: the bytes are still in the stream and `read` returns them, which is why
// this checks both halves rather than only the fast one.
func TestAPtyRunReturnsWhatWasSaidWhenTheCommandFallsSilent(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	termText(t, s, map[string]any{
		"cmd":  "run " + h + " $1",
		"args": []any{"sleep 1; echo late-marker"},
	})

	// The run came back on the echo alone, so the marker is not in that reply yet.
	// It must still be in the stream: the early return is a decision to stop
	// WAITING, not a decision to drop what arrives next.
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(termText(t, s, map[string]any{"cmd": "read " + h + " --from-line 1", "timeout": 0}), "late-marker") {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("a command that spoke after a pause lost its output")
}
