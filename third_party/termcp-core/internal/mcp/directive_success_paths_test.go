package mcp

import (
	"strings"
	"testing"
	"time"
)

// TestInputTypesWithoutSubmitting is the property that separates `input` from `run`:
// the bytes reach the channel and the Enter does not.
//
// It was never exercised. Every test of `input` drove a refusal (a closed pipe, a
// foreign session, a missing payload), so the directive's success path had no
// coverage at all — and the difference it exists for is exactly what a refusal test
// cannot see. `run` submits; `input` leaves the line unfinished.
//
// A pipe running `cat` makes both halves observable: cat copies what it reads to the
// channel, so the bytes appear, while the NEWLINE that would end the line only
// arrives with `key enter`.
func TestInputTypesWithoutSubmitting(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "cat")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	const marker = "typed-not-submitted"
	if got := termText(t, s, map[string]any{"cmd": "input " + h + " $1", "args": []any{marker}}); !strings.HasPrefix(got, "TERM/1 2") {
		t.Fatalf("input to a live channel must succeed:\n%s", got)
	}

	// No --from-line: the window is the tail, which is where bytes that have not
	// completed a line yet can still be seen.
	before := pollBody(t, s, h, func(body string) bool { return strings.Contains(body, marker) })
	if !strings.HasSuffix(before, marker) {
		t.Fatalf("the typed bytes did not reach the channel un-submitted; body = %q", before)
	}

	// The Enter `run` would have added is what ends the line, and the transcript says
	// so in its own words: before it, the channel has no COMPLETE line (reading the
	// transcript from line 1 is refused for exactly that reason); after it, the line
	// is there. Waiting on the newline byte alone is not enough — the window renders
	// lines, and a lone newline renders as nothing.
	termText(t, s, map[string]any{"cmd": "key " + h + " enter"})
	var complete string
	for i := 0; i < 50; i++ {
		complete = termText(t, s, map[string]any{"cmd": "read " + h + " --from-line 1", "timeout": 0})
		if strings.Contains(complete, marker) && !strings.Contains(complete, "complete line(s)") {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if strings.Contains(complete, "complete line(s)") || !strings.Contains(complete, marker) {
		t.Fatalf("the line never completed, so `key enter` did not end it:\n%s", complete)
	}
}

// TestEditChangesAStoredProfile is the success path of `edit`, which no test drove.
//
// `create` was covered, and so were the refusals — a disabled write gate, a missing
// entry, a stale flag. The verb's actual job (change a stored field through the
// command surface) was not: every `edit` in the suite exited on the gate or on a bad
// flag, so a handler that dropped the value on the floor would have passed the lot.
func TestEditChangesAStoredProfile(t *testing.T) {
	s := profilesTestServer(t)
	if text := termText(t, s, map[string]any{
		"cmd": "create host editable --host 192.0.2.1 --user u --password p",
	}); !strings.Contains(text, "success") {
		t.Fatalf("create failed, so there is nothing to edit:\n%s", text)
	}

	if text := termText(t, s, map[string]any{
		"cmd": "edit host editable --host 192.0.2.99 --user deploy",
	}); !strings.Contains(text, "success") {
		t.Fatalf("editing a stored profile failed:\n%s", text)
	}

	// Read it back through the command surface: the values that changed are the ones
	// the store now holds.
	got := termText(t, s, map[string]any{"cmd": "ls host"})
	if !strings.Contains(got, "192.0.2.99") || !strings.Contains(got, "deploy") {
		t.Fatalf("the edit did not reach the store:\n%s", got)
	}
}

// pollBody reads the channel's transcript from line 1 until want says so, and returns
// the last body it saw. A peek (timeout 0) is used so the loop does not spend the
// tool's blocking window on every attempt.
func pollBody(t *testing.T, s *Server, h string, want func(body string) bool) string {
	t.Helper()
	var body string
	for i := 0; i < 50; i++ {
		reply := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": 0})
		body = replyBody(reply)
		if want(body) {
			return body
		}
		time.Sleep(50 * time.Millisecond)
	}
	return body
}

// replyBody is the text after the blank line that ends the headers.
func replyBody(reply string) string {
	if i := strings.Index(reply, "\n\n"); i >= 0 {
		return reply[i+2:]
	}
	return reply
}
