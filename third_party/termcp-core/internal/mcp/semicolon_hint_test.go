package mcp

import (
	"strings"
	"testing"
)

// TestSemicolonAfterPayloadGetsAHint pins the hint that names the real mistake.
//
// `run h:1 $1; whoami` fails with "unknown directive: whoami", which is true and
// useless: the caller wrote shell commands and the `;` separated DIRECTIVES. The
// reply already carries the directive table for a misspelled name, and this is not
// that case — no table entry would have helped, because the caller was not writing
// termcp statements at all.
//
// THE PAYLOAD IS A REFERENCE NOW, so the shape that reaches this hint is the legal
// `$1` followed by the `;`: the payload itself passes, the shell command after the
// separator is what the caller mistook for one.
func TestSemicolonAfterPayloadGetsAHint(t *testing.T) {
	s := newTestServer(t)
	got := termText(t, s, map[string]any{
		"cmd":  "run a:1 $1; whoami",
		"args": []any{"pwd"},
	})

	if !strings.Contains(got, "unknown directive: whoami") {
		t.Fatalf("the failure itself must still be reported:\n%s", got)
	}
	for _, want := range []string{"HINT", "args", "&&"} {
		if !strings.Contains(got, want) {
			t.Errorf("hint does not mention %q:\n%s", want, got)
		}
	}
	if !strings.Contains(got, "a `run`") {
		t.Errorf("hint does not name the directive that was cut short:\n%s", got)
	}
}

// TestNoHintWhenNothingWasCutShort pins the negative: a typo in a command that has
// no payload has no `;` to blame, so the reply must not suggest args.
func TestNoHintWhenNothingWasCutShort(t *testing.T) {
	s := newTestServer(t)
	got := termText(t, s, map[string]any{"cmd": "ls; bogus"})
	if !strings.Contains(got, "unknown directive: bogus") {
		t.Fatalf("the failure itself must be reported:\n%s", got)
	}
	if strings.Contains(got, "HINT") {
		t.Errorf("a typo after `ps` has nothing to do with a payload:\n%s", got)
	}
}

// TestHelpRunDocumentsArgs pins that `help run` teaches the args channel.
//
// The directive is the one place a caller looks up how to run something, and the
// page has to state the rule that decides the whole shape: the payload is one
// variable reference and the shell text is the value it carries.
func TestHelpRunDocumentsArgs(t *testing.T) {
	s := newTestServer(t)
	// THE WHOLE PAGE, NOT THE DEFAULT WINDOW. `help run` is longer than 8 KiB, so a read
	// with the default window returns a 206 with the tail withheld — and a grep over what
	// arrived then reports "the page does not document vars" when the page documents it
	// two kilobytes further down. The window is the caller's to raise, and this test is
	// the caller.
	got := termText(t, s, map[string]any{
		"cmd":         "help run",
		"bytes_limit": float64(32768),
		"lines_limit": float64(2000),
	})
	for _, want := range []string{"args", "$1", "exactly one reference", "vars"} {
		if !strings.Contains(got, want) {
			t.Errorf("help run does not mention %q:\n%s", want, got)
		}
	}
}

// TestHintSuppressesTheDirectiveTable pins that the two replies do not compete.
//
// The directive table rides along on an unknown-but-directive-looking name, which is
// right for a typo and wrong for a shell command that a `;` cut loose: there the
// caller does not need one of 25 directives, they need a different argument. Showing
// both put 25 names between the caller and the one line that helps.
//
// THE LITERAL PAYLOAD IS THE SAME MISTAKE ONE STEP EARLIER, and it is refused by the
// payload rule instead of the unknown-directive path — so it must not print the
// table either. That caller needs `$1` and args, not a list of verb names.
func TestHintSuppressesTheDirectiveTable(t *testing.T) {
	s := newTestServer(t)
	got := termText(t, s, map[string]any{
		"cmd":  "run a:1 $1; whoami",
		"args": []any{"pwd"},
	})
	if !strings.Contains(got, "HINT") {
		t.Fatalf("expected a hint:\n%s", got)
	}
	if strings.Contains(got, "connect | open | ls") {
		t.Errorf("the directive table competes with the hint:\n%s", got)
	}

	got = termText(t, s, map[string]any{"cmd": "run a:1 pwd; whoami"})
	if !strings.Contains(got, "takes a VARIABLE") {
		t.Fatalf("a literal payload must be refused by the payload rule:\n%s", got)
	}
	if strings.Contains(got, "connect | open | ls") {
		t.Errorf("the directive table competes with the payload refusal:\n%s", got)
	}

	// A plain typo still gets the table: that caller DOES need the name list.
	got = termText(t, s, map[string]any{"cmd": "ls; bogus"})
	if !strings.Contains(got, "connect | open | ls") {
		t.Errorf("a bare typo should still carry the table:\n%s", got)
	}
}
