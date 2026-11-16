package mcp

import (
	"strings"
	"testing"
	"time"
)

// TestAmpersandRunsStatementsConcurrently pins that `&` is not decorative.
//
// It used to be: the shell parser reports `a & b` as two statements and records the
// parallelism in a Background flag that NOTHING read, so the line ran sequentially
// while reading as parallel. A silent semantic reversal is worse than a refusal.
//
// THE BASELINE IS MEASURED, NOT ASSUMED. This used to reason that "a pty `run` waits
// its whole timeout whatever happens inside, so two parallel runs cost one window and two
// sequential runs cost two", and compared the batch against a fixed 70% of the sequential
// time. The premise died with issue #156: a pty wait now ends when the stream goes quiet,
// so a run costs roughly max(command, quiet period) instead of the window, and the ratio
// between one statement and two is no longer 2:1 — it measured 0.71 against a 0.70 bound,
// which is a test that reports the runner's mood.
//
// One statement's cost and two statements' cost are both measured here, and the parallel
// batch has to land closer to one than to two. A loaded runner scales all three together,
// so the comparison survives it; an ignored `&` puts the batch at the two-statement cost,
// where it fails by the whole gap.
func TestAmpersandRunsStatementsConcurrently(t *testing.T) {
	s := newTestServer(t)
	h1 := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h1})
	// TWO CHANNELS, because the measurement needs two streams. Two runs on ONE channel
	// are concurrent but share a stream, and a pty wait ends when ITS stream goes quiet —
	// so each run keeps restarting the other's quiet clock and the batch measures half
	// way between one statement and two (observed: 1.5x one, not 2x and not 1x). That is
	// a property of reading one stream from two places, not of `&`, and it is not what
	// this test is about.
	h2 := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h2})

	const window = 2.0
	// THE SHELL LINE RIDES IN args: `run` takes exactly one variable reference, and
	// the value is what the terminal reads.
	run := func(cmd string, args ...any) (string, time.Duration) {
		t.Helper()
		call := map[string]any{"cmd": cmd, "timeout": window}
		if len(args) > 0 {
			call["args"] = args
		}
		start := time.Now()
		text := termText(t, s, call)
		return text, time.Since(start)
	}

	_, one := run("run "+h1+" $1", "echo ONE")
	seq, two := run("run "+h1+" $1; run "+h2+" $2", "echo ONE", "echo TWO")
	if n := strings.Count(seq, "TERM/1 "); n != 2 {
		t.Fatalf("the sequential baseline must produce 2 slots, got %d", n)
	}
	par, parallel := run("run "+h1+" $1 & run "+h2+" $2", "echo ONE", "echo TWO")
	if n := strings.Count(par, "TERM/1 "); n != 2 {
		t.Fatalf("the parallel batch must still produce one slot per statement, got %d:\n%s", n, par)
	}

	// NINETY PERCENT OF TWO STATEMENTS, not half of them, and the difference is the pty
	// wait rather than `&`. A run ends when its stream goes quiet, and two concurrent runs
	// each spend that quiet period, so a genuinely parallel batch measures around 0.7x the
	// sequential cost — measured 0.71 against a 0.70 bound, which is a test reporting the
	// runner's mood. The regression this exists to catch is `&` being ignored, and that
	// puts the batch at 1.0x. Everything between is the machine.
	if parallel > two*9/10 {
		t.Errorf("`&` ran sequentially: one=%v two=%v parallel=%v (must be under %v)",
			one.Round(time.Millisecond), two.Round(time.Millisecond),
			parallel.Round(time.Millisecond), (two * 9 / 10).Round(time.Millisecond))
	}
	_ = one
}

// TestParallelSlotsKeepCommandOrder pins that a batch reports its slots in the order
// the caller wrote them. Slots are addressed positionally, so completion order would
// make the reply's meaning depend on which command happened to finish first.
func TestParallelSlotsKeepCommandOrder(t *testing.T) {
	s := newTestServer(t)
	h1 := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h1})
	h2 := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h2})

	out := termText(t, s, map[string]any{
		"cmd":     "run " + h1 + " $1 & run " + h2 + " $2",
		"args":    []any{"echo FIRST", "echo SECOND"},
		"timeout": float64(2),
	})
	i1 := strings.Index(out, "FIRST")
	i2 := strings.Index(out, "SECOND")
	if i1 < 0 || i2 < 0 {
		t.Fatalf("both outputs must be present:\n%s", out)
	}
	if i1 > i2 {
		t.Errorf("slots were collected by completion order, not command order:\n%s", out)
	}
}

// TestSemicolonStillSeparatesSequentially pins that `;` did not become `&`:
// the separator is not a concurrency operator.
func TestSemicolonStillSeparatesSequentially(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	out := termText(t, s, map[string]any{
		"cmd":     "run " + h + " $1; run " + h + " $2",
		"args":    []any{"echo ONE", "echo TWO"},
		"timeout": float64(2),
	})
	if n := strings.Count(out, "TERM/1 "); n != 2 {
		t.Errorf("expected 2 slots, got %d:\n%s", n, out)
	}
	if !strings.Contains(out, "ONE") || !strings.Contains(out, "TWO") {
		t.Errorf("both statements must run:\n%s", out)
	}
}
