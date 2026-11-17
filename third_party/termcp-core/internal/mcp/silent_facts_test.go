package mcp

import (
	"strings"
	"testing"
)

// The three places a reply was computed, cut, and then said nothing about it. They share
// one shape: the caller cannot tell "this is all there was" from "I withheld some", and a
// window parameter it passed was either applied invisibly or not applied at all.

// TestAListingObeysTheWindow is the parameter half.
//
// `ls host` and `ls ext` built their slots directly, so `bytes_limit` did nothing to
// them: a six-entry table answered 200 with the whole thing under `bytes_limit: 60`. A
// parameter that is accepted and not applied is the same failure as a flag with no
// effect — the caller believes it bounded the reply and it did not.
func TestAListingObeysTheWindow(t *testing.T) {
	s := newTestServer(t)
	// Six saved profiles, so the table is comfortably longer than the window.
	for _, n := range []string{"alpha", "bravo", "charlie", "delta", "echo", "foxtrot"} {
		termText(t, s, map[string]any{"cmd": "create host " + n + " --host 192.0.2.1 --user u --password p"})
	}

	// One listing is enough to pin the shape, and `ls host` is the one with entries on a
	// bare harness. `ls ext` takes the identical path (generatedSlot). Error replies are
	// deliberately NOT capped: a truncated explanation hides the reason, and a failure
	// that cannot be read is worse than a long one.
	got := termText(t, s, map[string]any{"cmd": "ls host", "bytes_limit": 20})
	if body := replyBody(got); len(body) > 20 {
		t.Errorf("ls host ignored bytes_limit: %d bytes returned\n%s", len(body), got)
	}
	// And the cut is reported, because a listing has no cursor to continue from.
	if !strings.Contains(got, "Truncated:") {
		t.Errorf("ls host was cut by the window without saying so:\n%s", got)
	}
}

// TestAGeneratedReplyCutByTheWindowSaysSo is the report half.
//
// A generated reply has no stream: no cursor pages it, no Content-Range says where it
// stopped. The guide under `bytes_limit: 200` answered 206 with 196 bytes and nothing
// else — the caller could not tell a short manual from a cut one, and had no line telling
// it that raising the window was the remedy.
func TestAGeneratedReplyCutByTheWindowSaysSo(t *testing.T) {
	s := newTestServer(t)

	got := termText(t, s, map[string]any{"cmd": "", "bytes_limit": 200})
	if !strings.HasPrefix(got, "TERM/1 206") {
		t.Fatalf("the guide should come back as partial content:\n%s", got)
	}
	if !strings.Contains(got, "Truncated:") {
		t.Fatalf("the guide was cut without saying so:\n%s", got)
	}
	if !strings.Contains(got, "bytes_limit 200") {
		t.Errorf("the header does not name the window that cut it:\n%s", got)
	}
	// The remedy has to be the right one: there is no cursor for a generated reply.
	if !strings.Contains(got, "raise the window") {
		t.Errorf("the header does not say what to do about it:\n%s", got)
	}

	// Nothing is announced when nothing was cut.
	full := termText(t, s, map[string]any{"cmd": ""})
	if strings.Contains(full, "Truncated:") {
		t.Errorf("a reply that fits its window claims a truncation:\n%s", full)
	}
}

// TestASlotDroppedByTheTotalCapSaysSo is the batch half.
//
// `applyTotalCap` runs AFTER the statements execute, so a statement past the total limit
// has already run: its reply became a slot with headers, code 206 and an empty body,
// which is exactly what "this statement had nothing to say" looks like. A caller reading
// three slots had no way to know that two of them had produced output it never saw.
func TestASlotDroppedByTheTotalCapSaysSo(t *testing.T) {
	s := newTestServer(t)
	got := termText(t, s, map[string]any{"cmd": "ls; ls host; ls ext", "total_bytes_limit": 80})

	for _, want := range []string{"Dropped:", "total_bytes_limit (80)"} {
		if !strings.Contains(got, want) {
			t.Fatalf("a slot was emptied by the total cap without saying why (%q):\n%s", want, got)
		}
	}
	if !strings.Contains(got, "Run it alone") {
		t.Errorf("the header does not say what to do about it:\n%s", got)
	}
}
