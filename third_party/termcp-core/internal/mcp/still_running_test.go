package mcp

import (
	"strings"
	"testing"
)

// TestRunSaysWhenTheWindowExpiredWithTheCommandStillRunning pins issue P2-1 from the
// rc3 report: a long command's reply was indistinguishable from a finished, silent
// one, so a caller could not tell "still working" from "done" and kept polling a
// channel that had already finished.
//
// PIPE ONLY. A pipe channel's run waits for its process to exit, so returning while
// it lives means the window expired mid-command. A pty's shell outlives every
// command, so the same flag there would be permanently true and say nothing.
func TestRunSaysWhenTheWindowExpiredWithTheCommandStillRunning(t *testing.T) {
	s := newTestServer(t)

	h := openPipe(t, s, `sh -c 'sleep 5; echo LATE'`)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1",
		"args":    []any{"sleep 5"},
		"timeout": float64(1),
	})
	if !strings.Contains(got, "Still-Running: true") {
		t.Fatalf("a run whose window expired must say the command is still running:\n%s", got)
	}
	if strings.Contains(got, "Error-Code") {
		t.Errorf("still running is not an error:\n%s", got)
	}
}

// TestFinishedPipeDoesNotClaimToBeRunning pins the other side: a pipe that exited
// inside the window reports its exit state, not still_running.
func TestFinishedPipeDoesNotClaimToBeRunning(t *testing.T) {
	s := newTestServer(t)

	got := termText(t, s, map[string]any{
		"cmd":     "connect internal --argv sh -c 'echo DONE_NOW'",
		"timeout": float64(5),
	})
	if strings.Contains(got, "Still-Running") {
		t.Errorf("a finished command must not claim to be running:\n%s", got)
	}
	if !strings.Contains(got, "DONE_NOW") {
		t.Errorf("the output should be there:\n%s", got)
	}
}
