package mcp

import (
	"context"
	"strings"
	"testing"
)

// The window's definition domain, and what a caller learns from leaving it.
//
// The time and space window is termcp's limit on how long one call may wait and how
// much one call may put into the caller's context. Both are costs the caller pays, and
// the protocol refuses to let either grow without bound. A value outside the domain is
// therefore not a preference to clamp — clamping it would answer a request nobody made
// while hiding that the request was refused.

// TestNoCeilingIsRefusedOnAnythingTheCallerPaysFor pins where the line sits.
//
// Four numbers describe what one call may spend, and all four are the caller's to choose:
// how long to wait and how much to read. Each one used to be a 400 above a value this
// server picked. The caller is the party paying and the only one that knows the job, so
// each is now a size to stay under, published as such and honoured above it — the reply
// reports what actually happened (`Waited`, `Still-Running`, `Truncated`, `Content-Range`)
// instead of a refusal standing in for a decision this server cannot make.
func TestNoCeilingIsRefusedOnAnythingTheCallerPaysFor(t *testing.T) {
	s := newTestServer(t)
	got := s.Term(context.Background(), "ls", map[string]any{
		"cmd":               "ls",
		"timeout":           99999,
		"bytes_limit":       99999,
		"lines_limit":       99999,
		"total_bytes_limit": 99999,
	}).Text()

	for _, name := range []string{"timeout", "bytes_limit", "lines_limit", "total_bytes_limit"} {
		if strings.Contains(got, name+" must be <=") {
			t.Errorf("%s is a cost the caller chose, and it was refused:\n%s", name, got)
		}
	}
}

// TestALongWaitIsHonouredAndTheDesignIsStillTaught pins both halves of the time rule.
//
// `timeout: 320` used to be a 400, and the refusal was written to teach: this window is a
// bound on waiting, not a wait for completion, and the number 60 came from the transport in
// front of the caller. The teaching was right and the refusal was not — the caller that
// wants to wait 320 seconds is the one who will lose the reply, and it is the only party
// that knows which client it is talking through. So the number is honoured, and the same
// explanation now lives where the choice is made: in the argument's own description, which
// is what a caller reads before sending anything.
func TestALongWaitIsHonouredAndTheDesignIsStillTaught(t *testing.T) {
	s := newTestServer(t)
	got := s.Term(context.Background(), "ls", map[string]any{"cmd": "ls", "timeout": 320}).Text()
	if strings.Contains(got, "400") || strings.Contains(got, "timeout must be") {
		t.Fatalf("a long wait is the caller's own risk to take:\n%s", got)
	}

	// The recommendation, and the one ceiling that is real, are published where the
	// caller reads them.
	decl, ok := termArgDeclFor("timeout")
	if !ok {
		t.Fatal("no declaration for timeout")
	}
	for _, want := range []string{"wait to stay under", "transport", "--argv"} {
		if !strings.Contains(decl.DocNote+decl.BoundNote, want) {
			t.Errorf("the timeout documentation must say %q:\n%s\n%s", want, decl.DocNote, decl.BoundNote)
		}
	}

	// The floors are what is left, and a floor still teaches: a window narrower than one
	// character can never return one, and the remedy is a cursor rather than a bigger
	// window.
	floor := s.Term(context.Background(), "ls", map[string]any{"cmd": "ls", "bytes_limit": 2}).Text()
	for _, want := range []string{"smaller than one character", "--cursor"} {
		if !strings.Contains(floor, want) {
			t.Errorf("the floor refusal must say %q:\n%s", want, floor)
		}
	}
}

// TestASlotLimitAboveTheReplyCapIsRefusedOnlyWhenAsked pins the cross-field rule and
// its one exception.
//
// `bytes_limit: 32768` under a 1000 total is silently cut to the total: the caller
// gets fewer bytes than it named and no line saying so. That is refused. But a caller
// that only lowers the total is asking for a small reply, and the per-slot default
// being larger is how that is expressed — refusing it would make "cap the whole reply"
// impossible to say.
func TestASlotLimitAboveTheReplyCapIsRefusedOnlyWhenAsked(t *testing.T) {
	s := newTestServer(t)

	both := s.Term(context.Background(), "ls", map[string]any{
		"cmd": "ls", "bytes_limit": 32768, "total_bytes_limit": 1000,
	}).Text()
	if !strings.Contains(both, "larger than total_bytes_limit") {
		t.Errorf("a named per-slot limit above the reply cap must be refused:\n%s", both)
	}

	// Only the total: legal, and the reply is capped by it.
	only := s.Term(context.Background(), "ls", map[string]any{
		"cmd": "ls", "total_bytes_limit": 1000,
	}).Text()
	if strings.Contains(only, "larger than total_bytes_limit") {
		t.Errorf("lowering the total alone must not be refused:\n%s", only)
	}
}
