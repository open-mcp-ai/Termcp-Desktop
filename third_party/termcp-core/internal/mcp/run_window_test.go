package mcp

import (
	"strings"
	"testing"
	"time"
)

// runWindowUntil runs `run <h> [flags] $1` and retries until the reply BODY carries the
// marker.
//
// A FIXED TIMEOUT IS NOT A SYNCHRONISATION POINT (issue #189): a pipe channel has no
// "the command finished" signal either, so `run` returns whatever arrived inside its
// window and a loaded runner can hand back less. The assertions here are about WHICH
// lines the window held, so the window has to have held them before they are made.
func runWindowUntil(t *testing.T, s *Server, h, marker, line string, flags ...string) string {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for {
		cmd := "run " + h
		for _, f := range flags {
			cmd += " " + f
		}
		cmd += " $1"
		// A SHORT WINDOW, AND THE LOOP DOES THE WAITING. The read waits its whole window
		// on a live pipe channel (there is no completion signal to end it early), so a
		// generous timeout is paid in full on every attempt; the retry above is what
		// establishes the precondition, and it costs nothing when the output is already
		// there.
		reply := termText(t, s, map[string]any{"cmd": cmd, "args": []any{line}, "timeout": 2})
		if strings.Contains(replyBody(reply), marker) {
			return reply
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %q with %v never showed %q:\n%s", line, flags, marker, reply)
		}
	}
}

// TestRunTailIsScopedToThisCommand is issue #187, first half.
//
// `run --tail N` must be a tail of THIS command's output. The transcript's own tail
// selector reads from the end of the whole channel and switches on TailLines before
// Offset, so passing both would ignore the offset and hand back the previous command's
// last lines as if this command had printed them — which is exactly the confusion
// issue #194 is about, in the opposite direction.
//
// The first command prints MORE lines than the tail, so a window that read the
// channel's tail would return them; the second prints fewer, so the only way old-3 can
// appear is if the window was not scoped.
func TestRunTailIsScopedToThisCommand(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sh")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	runWindowUntil(t, s, h, "old-3", "printf 'old-1\\nold-2\\nold-3\\n'")

	got := runWindowUntil(t, s, h, "new-1", "printf 'new-1\\n'", "--tail", "3")
	body := replyBody(got)
	for _, leaked := range []string{"old-1", "old-2", "old-3"} {
		if strings.Contains(body, leaked) {
			t.Errorf("--tail reached past this command's output and returned %q:\n%s", leaked, got)
		}
	}

	// The trim is a FACT. Nothing was trimmed here (one line, tail of three), so the
	// header must be absent — a header that is always present says nothing.
	if strings.Contains(got, "Trimmed:") {
		t.Errorf("nothing was trimmed, so the header must be absent:\n%s", got)
	}

	// And when there IS something to trim, the reply says what it kept.
	trimmed := runWindowUntil(t, s, h, "new-4", "printf 'new-1\\nnew-2\\nnew-3\\nnew-4\\n'", "--tail", "2")
	if !strings.Contains(trimmed, "Trimmed:") {
		t.Fatalf("a trimmed tail must say what it withheld:\n%s", trimmed)
	}
	if !strings.Contains(replyBody(trimmed), "new-4") || strings.Contains(replyBody(trimmed), "new-1") {
		t.Errorf("--tail 2 did not keep the last two lines:\n%s", trimmed)
	}
}

// TestRunGrepSearchesThisCommandOnly is issue #187, second half.
//
// The search reuses `read --grep` and therefore counts MATCHES rather than raw lines,
// with the real line numbers — but scoped to the offset this command's input started
// at, so a match from an earlier command is not part of the answer.
func TestRunGrepSearchesThisCommandOnly(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sh")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	runWindowUntil(t, s, h, "old-error", "printf 'start\\n[ERROR] old-error\\n'")

	got := runWindowUntil(t, s, h, "wanted", "printf '[INFO] noise\\n[ERROR] wanted\\n'", "--grep", "ERROR")
	if !strings.Contains(got, "Grep:") || !strings.Contains(got, "Matches:") {
		t.Errorf("a searched run must report what it searched for and what it found:\n%s", got)
	}
	body := replyBody(got)
	if strings.Contains(body, "old-error") {
		t.Errorf("--grep searched bytes from a previous command:\n%s", got)
	}
	if strings.Contains(body, "noise") {
		t.Errorf("--grep returned a line that does not match:\n%s", got)
	}
}

// TestRunGrepWaitsForTheCommandItJustTyped is the defect the dogfood pass found.
//
// `read --grep` reads with a zero blocking window on purpose: the bytes are already in
// the transcript, and waiting for bytes that are not there would make "no match" take as
// long as the caller's patience and answer the same. A run's bytes are NOT already
// there, and the same zero window made it scan nothing and answer
// "no line matched `c` in the 0 lines scanned" — true, and useless.
//
// No retry loop here: the assertion IS that one call waits.
func TestRunGrepWaitsForTheCommandItJustTyped(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sh")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// The first run establishes that the channel works; the searched one is the test.
	runWindowUntil(t, s, h, "warm", "printf 'warm\n'")

	// THE MARKER MUST NOT BE IN THE ECHO. `printf 'wanted'` puts the word in the
	// transcript twice — once as the typed line and once as the output — so a search that
	// does not wait is satisfied by the echo and the test proves nothing (verified: it
	// passed with the fix removed). Running a script file keeps the command line
	// (`sh /tmp/…/probe.sh`) free of the word the script prints.
	script := writeScript(t, "sleep 1; printf 'noise\\nwanted\\n'")
	got := termText(t, s, map[string]any{
		"cmd":     "run " + h + " --grep wanted $1",
		"args":    []any{"sh '" + script + "'"},
		"timeout": 5,
	})
	if !statusLineIs(got, "200") {
		t.Fatalf("a searched run must find the line it just printed, in one call:\n%s", got)
	}
	if !strings.Contains(replyBody(got), "wanted") {
		t.Errorf("the search did not wait for the command's output:\n%s", got)
	}
	if strings.Contains(replyBody(got), "noise") {
		t.Errorf("--grep returned a line that does not match:\n%s", got)
	}
}

// TestRunSaysWhereItsInputLanded is issue #194.
//
// The reply to a run is a sample of the channel's stream, and Input-At is the offset the
// sample was taken from: it is what turns "here are some bytes" into "here is where to
// ask for the rest", and it is the only thing that distinguishes an empty reply from a
// caller whose cursor is already past the output.
//
// The assertion is not "the header exists" but "the number is usable": reading from it
// returns this command's output and not the previous command's.
func TestRunSaysWhereItsInputLanded(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sh")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	runWindowUntil(t, s, h, "before-marker", "printf 'before-marker\\n'")

	got := runWindowUntil(t, s, h, "after-marker", "printf 'after-marker\\n'")
	at := headerValueOr(got, "Input-At", "")
	if at == "" {
		t.Fatalf("the run reply does not say where its input landed:\n%s", got)
	}
	if at == "0" {
		t.Errorf("Input-At must be this command's start, not the channel's:\n%s", got)
	}

	replay := termText(t, s, map[string]any{"cmd": "read " + h + " --offset " + at, "timeout": 3})
	if !strings.Contains(replyBody(replay), "after-marker") {
		t.Errorf("reading from Input-At did not return this command's output:\n%s", replay)
	}
	if strings.Contains(replyBody(replay), "before-marker") {
		t.Errorf("Input-At points before this command's input:\n%s", replay)
	}

	// THE EMPTY REPLY IS THE CASE THAT NEEDS IT MOST: a window that returns nothing must
	// still say where the command's bytes would appear, or "nothing yet" and "you are
	// already past it" look identical.
	empty := termText(t, s, map[string]any{
		"cmd": "run " + h + " $1", "args": []any{"printf 'quiet-marker\\n'"}, "bytes_limit": 0,
	})
	if !statusLineIs(empty, "204") || headerValueOr(empty, "Input-At", "") == "" {
		t.Errorf("a reply that hands back nothing must still say where the input landed:\n%s", empty)
	}
}
