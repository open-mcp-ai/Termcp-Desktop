package mcp

import (
	"strings"
	"testing"
)

// A REFERENCE FILLS A VALUE. IT NEVER BECOMES PART OF THE GRAMMAR — end to end.
//
// The parser refuses the shape (see internal/directive's reference_grammar_test.go);
// this test pins what a CALLER reads, because the failure being fixed was not a crash
// but a success: `close <session> $1` closed the whole session and answered 204, and
// `read <session> $1` read the wrong channel and answered 200.
//
// The reply is asserted on both halves of the boundary:
//
//	what must be refused  a reference where a kind, a flag name or an index goes
//	what must still work  a reference in the target, a flag's value, a frame, a
//	                      declared data word, and the WHOLE address
func TestAReferenceInAGrammarPositionIsRefusedNotReinterpreted(t *testing.T) {
	s := newTestServer(t)

	cases := []struct {
		name, cmd string
		args      []any
	}{
		{"kind", "call $1 $2 $3", []any{"s:1", "mcp", `{}`}},
		{"channel index on close", "close s $1", []any{"2"}},
		{"channel index on read", "read s $1", []any{"2"}},
		{"flag name", "read s:1 cdp:1 $1 3", []any{"--at"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			args := map[string]any{"cmd": c.cmd, "timeout": float64(2)}
			if len(c.args) > 0 {
				args["args"] = c.args
			}
			text := termText(t, s, args)
			if !strings.Contains(text, "400") {
				t.Fatalf("%s should be refused with a 400, got:\n%s", c.cmd, text)
			}
			// The refusal has to name the rule, or the caller learns nothing about
			// which positions take a value.
			if !strings.Contains(text, "a reference fills a VALUE") {
				t.Fatalf("%s was refused for the wrong reason:\n%s", c.cmd, text)
			}
			// And it must not have run: no object may be named as acted on.
			if strings.Contains(text, "Closed:") {
				t.Fatalf("%s acted on something while being refused:\n%s", c.cmd, text)
			}
		})
	}
}

// TestTheWholeAddressIsStillAValue is the half that keeps the refusal from being a
// removal: a computed index is spelled by passing the whole handle, which needs no new
// syntax and no permission.
func TestTheWholeAddressIsStillAValue(t *testing.T) {
	s := newTestServer(t)

	// No session named s:2 exists, so the read fails — but it must fail by RESOLVING
	// the address (404 / not found), not by refusing the reference as grammar. That
	// difference is the whole point: the value arrived and became the target.
	text := termText(t, s, map[string]any{
		"cmd":     "read $1",
		"args":    []any{"s:2"},
		"timeout": float64(2),
	})
	if strings.Contains(text, "a reference fills a VALUE") {
		t.Fatalf("the whole address must stay a value:\n%s", text)
	}
	if !strings.Contains(text, "s:2") {
		t.Fatalf("the target should be the resolved address, got:\n%s", text)
	}
}

// TestARefusedCloseLeavesEveryChannelAlive is the assertion the first version of this
// fix was missing: it checked that the bad spelling was REFUSED, not that the object it
// would have destroyed SURVIVED.
//
// The bug (#199) answered 204 while closing the whole session, so a test that only reads
// the reply cannot tell the fix from the bug. What distinguishes them is whether the
// session is still there afterwards — which is exactly what a caller would have lost.
func TestARefusedCloseLeavesEveryChannelAlive(t *testing.T) {
	s := newTestServer(t)
	first := openPipe(t, s, "sleep 30")
	sid := strings.SplitN(first, ":", 2)[0]

	// A second channel, so there is something whose index a computed close would aim at.
	reply := termText(t, s, map[string]any{"cmd": "open " + sid})
	second := ""
	for _, f := range strings.Fields(reply) {
		if strings.HasPrefix(f, sid+":") && f != first {
			second = f
		}
	}
	if second == "" {
		t.Skipf("could not open a second channel; open answered:\n%s", reply)
	}

	// The refused spelling: an index that arrives as a value. Before the fix this closed
	// the SESSION and answered 204.
	text := termText(t, s, map[string]any{
		"cmd":     "close " + sid + " $1",
		"args":    []any{"2"},
		"timeout": float64(2),
	})
	if !strings.Contains(text, "400") {
		t.Fatalf("close %s $1 should be refused with a 400, got:\n%s", sid, text)
	}
	if strings.Contains(text, "Closed:") {
		t.Fatalf("the refused close still reported a close:\n%s", text)
	}

	// Both channels still answer, and the session is still listed. This is the half that
	// was missing: a refusal that had already acted would look identical in the reply.
	for _, h := range []string{first, second} {
		if out := termText(t, s, map[string]any{"cmd": "get " + h, "timeout": float64(1)}); strings.Contains(out, "404") {
			t.Fatalf("channel %s did not survive the refused close:\n%s", h, out)
		}
	}
	if out := termText(t, s, map[string]any{"cmd": "ls", "timeout": float64(2)}); !strings.Contains(out, sid) {
		t.Fatalf("session %s is gone after the refused close:\n%s", sid, out)
	}
}
