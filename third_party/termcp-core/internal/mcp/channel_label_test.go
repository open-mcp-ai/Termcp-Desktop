package mcp

import (
	"strings"
	"testing"
)

// TestAChannelLabelIsShownBesideItsAddress is issue #188.
//
// The label is not an address: the first column stays what every other command takes,
// and the label is what a caller running four things at once reads to tell which is
// which. A caller that asked for a name has to see that the server took it, and one
// that did not must not be handed an empty column.
func TestAChannelLabelIsShownBesideItsAddress(t *testing.T) {
	labelled := createdBody(map[string]any{"shell_id": "abc123", "session_id": "abc123", "index": float64(2), "name": "web"}, "pipe")
	if !strings.Contains(labelled, "[web]") {
		t.Errorf("a chosen label must appear in the reply:\n%s", labelled)
	}
	if !strings.HasPrefix(labelled, "abc123:2 ") {
		t.Errorf("the label must not replace the address:\n%s", labelled)
	}

	// The generated default is not a label the caller chose, so it is not dressed as
	// one — otherwise every reply carries a `[shell-…]` nobody asked for.
	plain := createdBody(map[string]any{"shell_id": "abc123", "session_id": "abc123", "index": float64(1), "name": "shell-9f2c"}, "pipe")
	if strings.Contains(plain, "[") {
		t.Errorf("the default name must not be printed as a label:\n%s", plain)
	}
}

// TestAnEmptyLabelIsRefused is the other half of #188: a caller that computed a name and
// got "" would open an unnamed channel and believe it was labelled.
//
// The refusal happens before the peer is dialled, so this needs no SSH session — the
// cdp target here is never contacted.
func TestAnEmptyLabelIsRefused(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	// `open` acts on the CONTAINER, so it takes the session id — the channel address
	// names one shell inside it and is refused by name.
	sessID := h[:strings.Index(h, ":")]
	got := termText(t, s, map[string]any{"cmd": "open " + sessID + " --name '' --argv sleep 5"})
	if !statusLineIs(got, "400") {
		t.Fatalf("an empty --name must be refused:\n%s", got)
	}
	if !strings.Contains(got, "empty value") {
		t.Errorf("the refusal must say what was wrong with the value:\n%s", got)
	}
}

// TestTwoChannelsCannotShareALabel is the session-layer half of #188.
//
// A label exists so a caller can tell one channel from another; two channels carrying
// the same one defeats exactly that, and `ls` then shows two rows that read alike. The
// refusal names the channel that holds it, and it happens BEFORE the SSH channel exists
// so a refused open costs nothing.
func TestTwoChannelsCannotShareALabel(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	awaitPtyReady(t, s, h)
	sessID := h[:strings.Index(h, ":")]

	first := termText(t, s, map[string]any{"cmd": "open " + sessID + " --name web --argv sleep 30"})
	if !strings.HasPrefix(first, "TERM/1 200") {
		t.Fatalf("the first labelled channel must open:\n%s", first)
	}
	second := termText(t, s, map[string]any{"cmd": "open " + sessID + " --name web --argv sleep 30"})
	if !statusLineIs(second, "400") {
		t.Fatalf("a duplicate label must be refused:\n%s", second)
	}
	if !strings.Contains(second, "web") || !strings.Contains(second, "already the label of") {
		t.Errorf("the refusal must name the label and the channel that holds it:\n%s", second)
	}

	// A different label still works, so the check is about the name and not about
	// opening a second channel.
	other := termText(t, s, map[string]any{"cmd": "open " + sessID + " --name db --argv sleep 30"})
	if !strings.HasPrefix(other, "TERM/1 200") {
		t.Fatalf("a free label must open:\n%s", other)
	}
}
