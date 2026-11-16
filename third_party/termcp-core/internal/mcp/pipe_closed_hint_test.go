package mcp

import (
	"strings"
	"testing"
)

// TestWriteToFinishedPipeSaysWhatHappened pins the translation of a bare I/O error.
//
// A pipe channel's process can exit while the channel still reports `running` (the
// status is updated when the exit is reaped), so the first write loses the race and
// the caller receives the OS error on a closed stdin: `EOF`, or `process has exited,
// cannot send input`. Neither says the two things that matter — this channel was a
// one-shot program, and its output is still there to read.
func TestWriteToFinishedPipeSaysWhatHappened(t *testing.T) {
	s := newTestServer(t)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "connect internal --argv sh -c 'echo hi'"}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// THE RACE THIS PINS IS "process gone, status not yet reaped" — so the process
	// must actually be gone before the write is attempted. Without this the test
	// asserted the behaviour of a LIVE process on a fast machine (where the write
	// lost the race by luck) and got 200 OK on a slow one, where the write arrived
	// first and the program happily read it.
	waitForShellToExit(t, s, shellIDFromHandle(t, s, h))

	for _, cmd := range []string{"run " + h + " $1", "input " + h + " $1"} {
		got := termText(t, s, map[string]any{"cmd": cmd, "args": []any{"extra"}, "timeout": 2})
		// 409, NOT 5xx: the channel's process is over, which is a state of the thing
		// being addressed, not a fault of the server. A 5xx sent a caller to retry a
		// write that can never land (issue #144).
		if !strings.HasPrefix(got, "TERM/1 409") {
			t.Fatalf("%s: expected 409 (the channel is over, the line was fine), got:\n%s", cmd, got)
		}
		for _, want := range []string{"PIPE channel", "read " + h, "open <session>"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s: the refusal does not mention %q:\n%s", cmd, want, got)
			}
		}
	}

	// And the advice is true: the output is still there.
	got := termText(t, s, map[string]any{"cmd": "read " + h})
	if !strings.Contains(got, "hi") {
		t.Errorf("the hint promises readable output; read returned:\n%s", got)
	}
}

// TestDeadPtyIsNotToldToReadItsExitCode pins the other side: a pty has no exit code,
// so the pipe advice must not be attached to it. Sending a caller to `read` for an
// exit status that cannot exist is worse than the bare error.
func TestDeadPtyIsNotToldToReadItsExitCode(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{"exit"}, "timeout": 2})
	got := termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{"echo after"}, "timeout": 2})
	if strings.HasPrefix(got, "TERM/1 2") {
		t.Skip("this shell survived `exit`; nothing to assert")
	}
	if strings.Contains(got, "PIPE channel") {
		t.Errorf("a pty must not be given pipe advice:\n%s", got)
	}
}

// TestFinishedPipeRefusesTheWriteNotTheRead pins the ordering the fix had to respect.
//
// The obvious fix — make Info() report "exited" as soon as the process channel
// closes — was tried and reverted: it made a READ return an empty body, because read
// paths treat "exited" as "the transcript is complete", and the watcher publishes it
// only after draining. So the process check belongs on the WRITE side alone, where
// nothing is promised about the transcript.
func TestFinishedPipeRefusesTheWriteNotTheRead(t *testing.T) {
	s := newTestServer(t)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "connect internal --argv sh -c 'echo kept'"}))

	// The process must have exited, or this tests the wrong state: the refusal is
	// about a FINISHED pipe, and a live one accepts the write (200 OK on CI).
	waitForShellToExit(t, s, shellIDFromHandle(t, s, h))

	// The write is refused with a readable reason...
	got := termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{"x"}, "timeout": 2})
	if !strings.Contains(got, "process has exited") || strings.Contains(got, "EOF") {
		t.Errorf("expected a named refusal rather than a bare EOF:\n%s", got)
	}

	// ...and the read still returns the whole transcript, not an empty body.
	got = termText(t, s, map[string]any{"cmd": "read " + h})
	if !strings.Contains(got, "kept") {
		t.Errorf("the transcript must survive the refusal:\n%s", got)
	}
	termText(t, s, map[string]any{"cmd": "close " + h})
}

// TestCallerMistakesInInputAreNotServerFaults pins issue #144.
//
// A typo in a key name answered 500 operation_failed, and so did a write to a pipe
// whose process had finished. Both are the same class of mistake — the request was
// wrong — and 5xx says the opposite: retry, alarm, file a bug against the server.
// The two are opposite in a way that matters to the caller (one line to fix, one
// channel that is over), so they get two different 4xx codes.
func TestCallerMistakesInInputAreNotServerFaults(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sleep 30")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// An unknown key name: the caller's argument, so 400 and a list to pick from.
	got := termText(t, s, map[string]any{"cmd": "key " + h + " boguskey"})
	if !strings.HasPrefix(got, "TERM/1 400") {
		t.Errorf("an unknown key name answered %q, want 400:\n%s", strings.SplitN(got, "\n", 2)[0], got)
	}
	if !strings.Contains(got, "unknown key \"boguskey\"") {
		t.Errorf("the refusal stopped naming the key:\n%s", got)
	}
	// The list is the useful half: it is what turns the error into a fix.
	if !strings.Contains(got, "ctrl+c") {
		t.Errorf("the refusal stopped listing the supported keys:\n%s", got)
	}

	// A channel whose process is gone: the resource's state, so 409.
	dead := handleFrom(t, termText(t, s, map[string]any{"cmd": "connect internal --argv sh -c 'echo hi'"}))
	waitForShellToExit(t, s, shellIDFromHandle(t, s, dead))
	got = termText(t, s, map[string]any{"cmd": "key " + dead + " enter", "timeout": 2})
	if !strings.HasPrefix(got, "TERM/1 409") {
		t.Errorf("a key sent to a finished process answered %q, want 409:\n%s", strings.SplitN(got, "\n", 2)[0], got)
	}
}
