package mcp

import (
	"strings"
	"testing"
	"time"
)

// TestAPlainReadContinuesWhereTheLastRunStopped is issue #162, S2.
//
// `read <channel>` documents its default as "the new bytes since the last read", and
// `run` reads positionally (it names the offset the command was submitted at), so no
// cursor ever moved: after a few runs the first plain read replayed the whole channel
// from byte zero — banner, prompts, every previous command's echo. The caller had
// already been shown all of it, and the Content-Range said "bytes 0-…", which is a
// scrollback dump wearing the shape of a read.
//
// The fix is that showing bytes advances the default reader to the end of what was
// shown, never backwards, and only when bytes were actually shown.
func TestAPlainReadContinuesWhereTheLastRunStopped(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// RETRIED UNTIL THE RUN'S OWN WINDOW SHOWED THE MARKER, and that is not politeness
	// towards a slow machine — it is what the assertion below is about.
	//
	// The shell's line editor installs a moment after the shell first speaks, and input
	// arriving in between is discarded rather than buffered (issue #156); the open's
	// budget covers the usual gap, but a loaded single-CPU machine widens it, and the run
	// then returns the boot banner. The cursor claim is "a read does not replay what a run
	// already showed", so the run has to have shown it — a run that returned the banner
	// moved the cursor to the banner's end, and the marker is legitimately still ahead of
	// it. Retrying re-types the command; the marker is the same either way.
	runUntilShown := func(line string) {
		t.Helper()
		marker := strings.TrimPrefix(line, "echo ")
		deadline := time.Now().Add(20 * time.Second)
		for {
			got := termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{line}, "timeout": 30})
			if strings.Contains(replyBody(got), marker) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("run %q never showed %q:\n%s", line, marker, got)
			}
		}
	}
	for _, line := range []string{"echo first-marker", "echo second-marker"} {
		runUntilShown(line)
	}

	// The read that used to replay from byte 0.
	after := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": 3})

	// A read starting at byte 0 is exactly what the bug looked like.
	if strings.Contains(after, "Content-Range: bytes 0-") {
		t.Errorf("a plain read replayed the channel from byte zero:\n%s", after)
	}
	if strings.Contains(replyBody(after), "first-marker") {
		t.Errorf("a plain read re-showed bytes the previous run already returned:\n%s", after)
	}

	// The other half: the bytes are not lost. An explicit positional read still reaches
	// them — without this the test would pass if the fix had simply dropped the log.
	replay := termText(t, s, map[string]any{"cmd": "read " + h + " --offset 0", "timeout": 3})
	if !strings.Contains(replyBody(replay), "first-marker") {
		t.Errorf("an explicit read from offset 0 no longer replays the transcript:\n%s", replay)
	}
}

// TestAFoldedWindowDoesNotAdvanceTheReadCursor is the exception the fix has to keep.
//
// `on_bytes_overflow: fold` and `bytes_limit: 0` reply without handing the bytes over
// (one says "there are more, here is how to ask", the other says "consume nothing").
// Advancing the cursor for those would turn a withheld window into a skipped one: the
// caller would never see those bytes and the cursor would claim it had.
func TestAFoldedWindowDoesNotAdvanceTheReadCursor(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1",
		"args":    []any{"echo folded-marker"},
		"timeout": 30,
		// A four-byte window with fold (the floor: one character): the reply carries the
		// range, not the bytes.
		"bytes_limit":       4,
		"on_bytes_overflow": "fold",
	})
	// The window has to contain something for the fold to happen at all: a pty's echo can
	// lose the race against the read, and a run that found nothing answers 204 — a
	// different case, and one this test must not pass on.
	deadline := time.Now().Add(20 * time.Second)
	for !strings.HasPrefix(got, "TERM/1 206") {
		if time.Now().After(deadline) {
			t.Fatalf("never got a folded window to test:\n%s", got)
		}
		got = termText(t, s, map[string]any{
			"cmd": "run " + h + " $1", "args": []any{"echo folded-marker"}, "timeout": 30,
			"bytes_limit": 4, "on_bytes_overflow": "fold",
		})
	}
	if !strings.Contains(got, "Folded: bytes ") {
		t.Fatalf("the fold does not name the bytes it withheld:\n%s", got)
	}

	after := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": 3})
	// The property is where the NEXT read starts: a withheld window must not move the
	// cursor past it. Asserting on the body instead would be measuring ANSI stripping —
	// this pty's banner is mostly escape sequences, so the rendered window is much
	// shorter than its byte range and the two would be confused.
	if !strings.Contains(after, "Content-Range: bytes 0-") {
		t.Errorf("the folded window advanced the read cursor past bytes it never showed:\n%s", after)
	}
}
