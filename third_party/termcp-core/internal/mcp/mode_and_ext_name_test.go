package mcp

import (
	"strings"
	"testing"
)

// TestConnectDoesNotAcceptAModeItCannotHonour pins the first half of issue #143.
//
// `--mode` sat in `connect`'s flag table and was never read: the mode follows from
// --argv (no --argv = a terminal, --argv = a pipe), and `open` is where --pty/--pipe
// live. A flag accepted and ignored lies twice — `--mode nonsense` answered 200, and
// `--mode pty --argv echo hi` answered with a pipe, which is the reading a caller
// most needs to be true. Removing it makes the refusal name the flag.
func TestConnectDoesNotAcceptAModeItCannotHonour(t *testing.T) {
	s := newTestServer(t)

	got := termText(t, s, map[string]any{"cmd": "connect internal --name m9 --mode nonsense --argv echo hi"})
	if code := statusCode(t, got); code != 400 {
		t.Fatalf("connect --mode answered %d, want 400 (the flag is gone):\n%s", code, got)
	}
	if !strings.Contains(got, "--mode") {
		t.Errorf("the refusal does not name the flag that is gone:\n%s", got)
	}
	// The mode itself must still be controllable where it is a real choice: `open`
	// takes --pty, and --argv still implies a pipe on connect.
	if out := termText(t, s, map[string]any{"cmd": "connect internal --name m10 --argv echo hi"}); !strings.Contains(out, "pipe") {
		t.Errorf("an --argv connect is no longer a pipe:\n%s", out)
	}
}

// TestLsExtWithANameIsAnsweredNotDropped pins the second half of issue #143.
//
// `ls ext nosuchext_zz` answered the FULL extension list, so a caller that mistyped a
// name read fourteen rows as confirmation that the extension exists. The name is a
// filter the handler cannot apply, which is why every other collection refuses it by
// name — and the refusal is only useful because `get ext <name>` now reads one.
func TestLsExtWithANameIsAnsweredNotDropped(t *testing.T) {
	s := newTestServer(t)

	got := termText(t, s, map[string]any{"cmd": "ls ext nosuchext_zz"})
	if code := statusCode(t, got); code < 400 {
		t.Fatalf("`ls ext <name>` answered %d; the name was dropped:\n%s", code, got)
	}
	if !strings.Contains(got, "get ext nosuchext_zz") {
		t.Errorf("the refusal does not come with the spelling that reads one:\n%s", got)
	}

	// The spelling it recommends has to exist, and to answer about THAT name rather
	// than about the collection. The test server has no extension store wired, so
	// the honest answer here is the layer's own "not configured" — what matters is
	// that it is not a 400 from the parser.
	one := termText(t, s, map[string]any{"cmd": "get ext nosuchext_zz"})
	if code := statusCode(t, one); code == 400 {
		t.Errorf("`get ext <name>` is not a spelling the parser accepts:\n%s", one)
	}
}
