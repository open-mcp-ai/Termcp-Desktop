package mcp

import (
	"strings"
	"testing"
)

// TestOnlyANegativeDialTimeoutIsRefused.
//
// The dialler clamps the timeout to [30,120], so a profile saying 5 was stored, echoed back
// by `get host`, and then dialled at 30 seconds: the number the caller set and the number in
// force were different, and nothing said so. Silently clamping is the failure this repo
// keeps closing — the value is accepted, appears in the config, and is not what runs.
func TestOnlyANegativeDialTimeoutIsRefused(t *testing.T) {
	s := newTestServer(t)
	s.ManageSSHConfigs = true

	// NEGATIVE IS THE ONLY VALUE THE DIALLER MUST REPLACE to have anything to do at
	// all, and it is a typo rather than a wish. A SHORT timeout is legal (30 is the
	// DEFAULT, not a floor, so a caller that wants a 5 second dial to fail fast can have
	// one) and so is a LONG one (a slow link or a bastion chain is a real reason, and only
	// the caller can see the link it is on). The first version of this test refused both,
	// which is how a change meant to fix a silent clamp replaced one refusal with another.
	got := termText(t, s, map[string]any{
		"cmd": "create host slow --host 192.0.2.1 --user u --password p --dial-timeout-seconds -1",
	})
	if !strings.HasPrefix(got, "TERM/1 4") {
		t.Errorf("a negative dial timeout was accepted:\n%s", got)
	}
	for _, want := range []string{"must not be negative", "is the value to stay under"} {
		if !strings.Contains(got, want) {
			t.Errorf("the refusal must say %q:\n%s", want, got)
		}
	}

	// Everything else is the caller's number, including the two the old range refused.
	for _, ok := range []string{"0", "1", "5", "30", "120", "600"} {
		got := termText(t, s, map[string]any{
			"cmd": "create host ok" + ok + " --host 192.0.2.1 --user u --password p --dial-timeout-seconds " + ok,
		})
		if strings.HasPrefix(got, "TERM/1 4") {
			t.Errorf("a %ss dial timeout was refused:\n%s", ok, got)
		}
	}
}

// TestTheClampIsTheValidator is the one-owner check: the bound the dialler applies and the
// bound the validator refuses on have to be the same number, or a value can pass validation
// and still be replaced at dial time.
func TestTheClampIsTheValidator(t *testing.T) {
	s := newTestServer(t)
	s.ManageSSHConfigs = true
	got := termText(t, s, map[string]any{
		"cmd": "create host clamped --host 192.0.2.1 --user u --password p --dial-timeout-seconds 31",
	})
	if strings.HasPrefix(got, "TERM/1 4") {
		t.Fatalf("a value inside the clamp's range was refused:\n%s", got)
	}
	// The stored value has to be the one the caller set: a validator that accepts 31 and
	// then lets the dialler use 30 would be the same silent substitution, one step later.
	back := termText(t, s, map[string]any{"cmd": "get host clamped"})
	if !strings.Contains(back, "31") {
		t.Errorf("the stored timeout is not the one that was set: %s", back)
	}
}
