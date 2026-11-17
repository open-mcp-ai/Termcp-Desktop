package mcp

import (
	"strings"
	"testing"
)

// TestCondChainGatesOnThePreviousStatement pins the `&&` / `||` operators between
// DIRECTIVES.
//
// They used to be shell operators that never reached the parser: `run h:1 a && b`
// arrived as one statement whose payload was "a && b", so the shell short-circuited
// it and termcp had no opinion. That is still the right reading for shell text —
// which now has to travel through `$1`/`$FOO`, the channels built for it — but a
// caller writing a chain of directives got the wrong thing entirely.
func TestCondChainGatesOnThePreviousStatement(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// `&&` after a success runs the next statement. The shell line is a VALUE: the
	// payload is one reference, so the `&&` can only be the directive chain's.
	got := termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1 && notify done",
		"args":    []any{"echo ONE"},
		"timeout": 3,
	})
	if !strings.Contains(got, "ONE") {
		t.Fatalf("the first statement did not run:\n%s", got)
	}
	if strings.Contains(got, "short_circuited") {
		t.Errorf("`&&` skipped a statement after a success:\n%s", got)
	}

	// `||` after a success does NOT run the next statement.
	got = termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1 || notify nope",
		"args":    []any{"echo TWO"},
		"timeout": 3,
	})
	if !strings.Contains(got, "short_circuited") {
		t.Errorf("`||` must skip after a success:\n%s", got)
	}
}

// TestCondChainReportsTheSkip pins that a short-circuited statement still answers.
//
// A cmd whose second statement did not run must not come back with one slot fewer:
// slots are addressed positionally, so a caller reading slot 2 would be reading the
// wrong statement without being told.
func TestCondChainReportsTheSkip(t *testing.T) {
	s := newTestServer(t)

	// A directive that fails: `run` on a handle that does not resolve.
	got := termText(t, s, map[string]any{
		"cmd":     "run nosuch:1 $1 && notify unreachable",
		"args":    []any{"x"},
		"timeout": 2,
	})
	if !strings.Contains(got, "short_circuited") {
		t.Fatalf("the skipped statement must say so:\n%s", got)
	}
	if !strings.Contains(got, "not run") {
		t.Errorf("the skip slot should explain itself:\n%s", got)
	}
}

// TestCondChainDoesNotSplitInsideASubstitutedValue pins the property that makes the
// whole design safe: an operator inside a value is data, never syntax.
//
// The substitution runs BEFORE the parse and replaces each value with one opaque
// token, so a `&&` inside args[0] cannot be seen by the shell parser. Without that
// ordering, `args: ["a && b"]` would have been split into two directives — the
// injection the quoting rules exist to prevent.
func TestCondChainDoesNotSplitInsideASubstitutedValue(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1",
		"args":    []any{"echo A && echo B"},
		"timeout": 3,
	})
	if strings.Contains(got, "short_circuited") {
		t.Errorf("an operator inside args[0] became a directive boundary:\n%s", got)
	}
	if !strings.Contains(got, "A") {
		t.Errorf("the shell line did not run:\n%s", got)
	}
}
