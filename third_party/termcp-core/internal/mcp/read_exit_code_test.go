package mcp

import (
	"strings"
	"testing"
)

// TestReadOnExitedPipeReturns200Not500 pins issue P1-1 from the rc3 report:
// reading an exited channel whose command returned a non-zero exit code used to
// answer 500, dragging down any preceding statements in the same call.
func TestReadOnExitedPipeReturns200Not500(t *testing.T) {
	s := newTestServer(t)

	// A pipe channel that exits with code 3.
	text := termText(t, s, map[string]any{"cmd": "connect internal --argv sh -c 'echo FAIL_LINE; exit 3'"})
	h := handleFrom(t, text)
	waitForShellToExit(t, s, shellIDFromHandle(t, s, h))

	// `read` must return 200, NOT 500.
	got := termText(t, s, map[string]any{"cmd": "read " + h})
	if !strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("read on exited non-zero pipe should return 200, got:\n%s", got)
	}
	if !strings.Contains(got, "FAIL_LINE") {
		t.Errorf("read should contain the output:\n%s", got)
	}
	if !strings.Contains(got, "Exit-Code: 3") {
		t.Errorf("read should still report Exit-Code in headers:\n%s", got)
	}
	if !strings.Contains(got, "Error-Code: child_exit") {
		t.Errorf("read should report Error-Code: child_exit:\n%s", got)
	}

	// Multi-statement: run followed by read of that pipe. The overall call must not
	// be marked as failed.
	hPty := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + hPty})

	multi := termText(t, s, map[string]any{
		"cmd":     "run " + hPty + " $1; read " + h,
		"args":    []any{"echo ALIVE"},
		"timeout": 2,
	})
	if strings.Contains(multi, "500 Internal") {
		t.Fatalf("the multi-statement call was polluted by a 500 from read:\n%s", multi)
	}
	if !strings.Contains(multi, "ALIVE") {
		t.Errorf("first statement output missing:\n%s", multi)
	}
}
