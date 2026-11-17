package mcp

import (
	"strings"
	"testing"
	"time"

	"github.com/open-mcp-ai/termcp/internal/session"
)

// TestTheReplySaysWhenAPersonTypedHere is the one part of "this channel is shared" that
// termcp can prove.
//
// The unanswerable question is "is a program in the foreground waiting for input?" — the
// process group lives on the far side of the SSH connection, and a Windows terminal has
// no such notion; a program blocked at a prompt and a finished command are byte-identical
// from here. But the transcript records the SOURCE of every input span (`i` a person
// through the HTTP/WebSocket surface, `a` an agent through MCP), because the log has to
// be replayable and auditable. So a caller about to type into a channel a human is using
// can be told, instead of finding out from the bytes.
func TestTheReplySaysWhenAPersonTypedHere(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// Nothing has happened yet: no header. A header that is always present is a header
	// nobody reads.
	if got := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": 2}); strings.Contains(got, "Typed-By") {
		t.Errorf("the reply claims a person typed before anyone did:\n%s", got)
	}

	// A person types through the session API — the same door the Web UI uses, which is
	// what makes these bytes `i` rather than `a`.	// THE DISCRIMINATION IS THE POINT: an agent's own typing is not "a person". A header
	// that fired on `a` spans too would fire on every reply this caller ever gets, which
	// is the same as not having it.
	if run := termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{"echo agent-typing"}, "timeout": 30}); strings.Contains(run, "Typed-By") {
		t.Errorf("an agent's own command was reported as outside input:\n%s", run)
	}

	cs, bad := s.requireShell(h)
	if bad != nil {
		t.Fatalf("no channel %s: %v", h, bad)
	}
	if err := cs.SendTerminalBytesFrom([]byte("echo typed-by-a-person"), true, session.InputFromAPI); err != nil {
		t.Fatalf("the person's keystrokes were refused: %v", err)
	}

	// WAIT FOR THE BYTES TO BE IN THE LOG, NOT FOR A DURATION.
	//
	// The person's line reaches the channel's log asynchronously, and a read that runs
	// too early sees a window that ENDS BEFORE it: the probe for this flake showed
	// `Content-Range: bytes 0-176/177` while the input span sat at 177, so the reply had
	// nothing to report and was right not to. The test read that as a product failure —
	// it failed once in a full shuffled run and never in twelve repeats, which is what a
	// race looks like.
	//
	// The wait is a POSITIONAL read: `--cursor 0` names its window, so it consumes
	// nothing, and repeating it cannot skip past the bytes under test the way a second
	// plain read would.
	deadline := time.Now().Add(10 * time.Second)
	for {
		seen := termText(t, s, map[string]any{"cmd": "read " + h + " --cursor 0", "timeout": 2})
		if strings.Contains(seen, "typed-by-a-person") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the person's line never reached the channel's log, so the header below "+
				"cannot be about it:\n%s", seen)
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": 3})
	if !strings.Contains(got, "Typed-By: api") {
		t.Errorf("a human typed into this channel and the reply does not say so:\n%s", got)
	}

	// And it is not sticky-by-accident in the wrong direction: a reply whose window
	// starts after the person's line is about bytes the caller already accounted for.
	if after := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": 2}); strings.Contains(after, "Typed-By") {
		t.Errorf("the header follows the caller past the bytes it describes:\n%s", after)
	}
}
