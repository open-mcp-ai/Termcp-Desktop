package mcp

import (
	"strings"
	"testing"
	"time"
)

// HOW THE WAIT ENDED IS A FACT THE CALLER NEEDS, AND IT WAS THE ONE NOT SAID.
//
// A pty has no completion signal, so a `run` on one ends its wait one of two ways:
// the window runs out, or the stream goes quiet (half a second with no new byte). The
// quiet case is the ordinary one — it is how a finished command looks — and it said
// nothing at all. So a command that went silent while it worked was indistinguishable
// from a command that had finished: the reply was 200, the body was the echoed line,
// and the caller had no way to tell whether more was coming.
//
// Three callers reading the same reply hit it the same way ("the output looks
// truncated, and nothing tells me whether it is"). Measured before the fix, a
// `sleep 6` with a 2s window: code 200, Waited 1540ms, no header saying anything about
// why the wait stopped at 1540ms.
//
// `Still-Running` is deliberately not reused: it is set when the window expired while
// bytes were still arriving, which is evidence. A quiet stream is not evidence of
// anything, so the reply names the evidence instead of guessing at a conclusion.
func TestAPtyRunSaysTheStreamWentQuiet(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// The command prints, then works silently for longer than the quiet window. The
	// wait ends on the quietness, not on the window.
	got := termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1",
		"args":    []any{"echo FIRST-LINE; sleep 4; echo LATE-LINE"},
		"timeout": 30,
	})
	if v, ok := headerValue(t, got, "Completion"); !ok || v != "quiet" {
		t.Fatalf("a wait that ended on quietness does not say so (Completion=%q, present=%v):\n%s", v, ok, firstLine(got))
	}
	// And it must not claim the other thing: nothing here proves the command finished
	// or that it is still running.
	if v, ok := headerValue(t, got, "Still-Running"); ok {
		t.Fatalf("a quiet stream claimed Still-Running=%q, which is evidence this reply does not have", v)
	}

	// The late output is still in the stream, which is what makes the header useful
	// rather than alarming: the command kept working and its bytes arrived after the
	// reply did. Polled, because the point is that they arrive LATER.
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(termText(t, s, map[string]any{"cmd": "read " + h + " --tail 20", "timeout": 0}), "LATE-LINE") {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("the bytes the quiet wait left behind never became readable")
}

// A PIPE STILL REPORTS THE ONE THING A PIPE CAN: its process exiting.
//
// Completion is a pty statement. On a pipe the observable fact is the exit, and a
// quiet stream there means the process is waiting for input — a different sentence
// entirely, and one this header must not overwrite.
func TestAPipeRunDoesNotClaimQuiet(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "cat")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1",
		"args":    []any{"hello-pipe"},
		"timeout": 10,
	})
	if v, ok := headerValue(t, got, "Completion"); ok {
		t.Fatalf("a pipe reply carried Completion=%q; a pipe's completion is its exit, which is reported as a status", v)
	}
}
