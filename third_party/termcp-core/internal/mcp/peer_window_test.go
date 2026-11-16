package mcp

import (
	"testing"
	"time"
)

// TestThePeerWindowAsksWhetherTheCallerSpoke.
//
// `timeout` has a default of its own, so the value in termArgs is never zero and a peer
// call could not tell "the caller asked for 2 seconds" from "nobody asked". It read the
// second as the first, so the 15s peer default was dead code and every `call` through the
// term door was bounded by the read window instead — a cold tunnel through the internal SSH
// server can exceed two seconds, and CI saw exactly that as a 504 on an ordinary LSP frame.
func TestThePeerWindowAsksWhetherTheCallerSpoke(t *testing.T) {
	cases := []struct {
		name string
		win  termArgs
		want time.Duration
	}{
		{"nobody asked", termArgs{TimeoutSeconds: defaultTimeoutSeconds}, defaultPeerWindow},
		{"the caller named a short one", termArgs{TimeoutSeconds: 1, TimeoutSet: true}, time.Second},
		{"the caller named the default", termArgs{TimeoutSeconds: defaultTimeoutSeconds, TimeoutSet: true}, 2 * time.Second},
		{"the caller named a long one", termArgs{TimeoutSeconds: 45, TimeoutSet: true}, 45 * time.Second},
		// An explicit zero is "do not wait", which a request/response call cannot use: it
		// keeps the peer default rather than becoming an instantly-expired deadline.
		{"the caller said zero", termArgs{TimeoutSeconds: 0, TimeoutSet: true}, defaultPeerWindow},
	}
	for _, c := range cases {
		if got := peerCallWindow(c.win); got != c.want {
			t.Errorf("%s: peerCallWindow = %v, want %v", c.name, got, c.want)
		}
	}
}

// TestTheTermDoorReportsWhetherTheCallerNamedATimeout pins the flag's source: it is the
// presence of the key, not the value, and a caller that sends the default explicitly is
// still a caller that spoke.
func TestTheTermDoorReportsWhetherTheCallerNamedATimeout(t *testing.T) {
	absent, err := parseTermArgs(map[string]any{})
	if err != nil {
		t.Fatalf("no arguments must be legal: %v", err)
	}
	if absent.TimeoutSet {
		t.Error("an argument nobody sent reports itself as sent")
	}
	present, err := parseTermArgs(map[string]any{"timeout": float64(defaultTimeoutSeconds)})
	if err != nil {
		t.Fatalf("an explicit default must be legal: %v", err)
	}
	if !present.TimeoutSet {
		t.Error("an explicit timeout reports itself as absent, which is the bug this flag exists for")
	}
}
