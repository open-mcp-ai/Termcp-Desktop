package mcp

import (
	"strings"
	"testing"
	"time"
)

// TestPtyEmptyReadReportsWaitedAndSampled pins 误读 3 & 桩 5:
//
// A 204 that arrives in 0.03ms is a crash/socket death; a 204 in 2000ms is legitimate
// silence. They used to look identical.
//
// And an empty read of a pty is a SAMPLE of a live shell that outlives every command;
// it does not mean the command exited. Both facts must be in the reply.
func TestPtyEmptyReadReportsWaitedAndSampled(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// DRAIN UNTIL QUIET: a pty shell prints its prompt in several bursts (theme,
	// then the cursor escape, then the trailing newline), so one read is not enough
	// to reach the state this test is about. The 204 under test is what a read
	// answers ONCE THE STREAM IS SILENT.
	var got string
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got = termText(t, s, map[string]any{"cmd": "read " + h, "timeout": float64(1)})
		if strings.HasPrefix(got, "TERM/1 204") {
			break
		}
	}

	if !strings.Contains(got, "Sampled: true") {
		t.Errorf("empty pty read must carry Sampled: true so caller knows shell is alive:\n%s", got)
	}
	if !strings.Contains(got, "Waited:") {
		t.Errorf("reply must report actual elapsed time (Waited header):\n%s", got)
	}
}

// TestSemicolonBetweenRunAndShellCommandExplainsItself pins 误读 1, IN THE FORM THE
// RULE NOW TAKES.
//
// A naive caller writing `run h:1 cd /tmp; ls -la` used to get the shell command read
// as a second DIRECTIVE, and the reply had to explain that `;` is termcp syntax. The
// literal payload is refused before that question is asked now: `cd /tmp` is not a
// reference, so the reply names the rule and says where the shell text goes. What must
// not happen is a bare `unknown directive: ls` — the caller is told the shape instead
// of guessing it.
func TestSemicolonBetweenRunAndShellCommandExplainsItself(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// Two statements, the second is a shell command that the `;` cut loose — and the
	// first one's payload is not a variable, which is the refusal under test.
	got := termText(t, s, map[string]any{"cmd": "run " + h + " true; make -j4"})

	if !strings.HasPrefix(got, "TERM/1 400") {
		t.Fatalf("a literal payload must be refused 400:\n%s", got)
	}
	if code, _ := headerValue(t, got, "Error-Code"); code != "invalid_argument" {
		t.Errorf("Error-Code = %q, want invalid_argument:\n%s", code, got)
	}
	// The reply must name the RULE, not leave the caller with "termcp has no make".
	if !strings.Contains(got, "takes a VARIABLE") {
		t.Errorf("error must say the payload has to be one reference:\n%s", got)
	}
	// And it must name where the shell line goes, because that is the fix.
	if !strings.Contains(got, "args:") {
		t.Errorf("error must suggest args: [...] as the escape:\n%s", got)
	}
	if !strings.Contains(got, "$1") {
		t.Errorf("error must show the reference the payload takes:\n%s", got)
	}
	// Nothing ran: the refused statement is the FIRST one, so the reply is a single
	// 400 slot — the `make -j4` the caller thought was a second shell command never
	// had a slot to run in.
	if n := strings.Count(got, "TERM/1 "); n != 1 {
		t.Errorf("expected one slot and nothing executed, got %d slots:\n%s", n, got)
	}
}
