package mcp

import (
	"strings"
	"testing"
	"time"
)

// TestLsFoldsFinishedChannelsAndSaysSo is issue #193.
//
// `open --argv` per command leaves a channel behind that stays readable, so `ls
// --shells` grows into a wall of dead pipes and the caller has to read past all of them
// to find the one it is using. Folding them is a VIEW, not reclamation — which is why
// the count has to be printed: a listing that quietly dropped them reads as "those
// channels are gone", and a caller that believes that will not look for output it still
// has.
func TestLsFoldsFinishedChannelsAndSaysSo(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "true")
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// Wait for the process to actually be finished, which is the state the listing is
	// about: `true` exits immediately, but "immediately" is not "by the time the next
	// command runs" on a loaded machine.
	deadline := time.Now().Add(20 * time.Second)
	for {
		got := termText(t, s, map[string]any{"cmd": "get " + h})
		if strings.Contains(got, "status: exited") || strings.Contains(got, "status: error") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the channel never finished:\n%s", got)
		}
		time.Sleep(50 * time.Millisecond)
	}

	folded := termText(t, s, map[string]any{"cmd": "ls"})
	if !strings.Contains(folded, "finished channel(s) not shown") {
		t.Errorf("a folded listing must say how many channels it hid:\n%s", folded)
	}
	if strings.Contains(folded, h+"\t") {
		t.Errorf("a finished channel is still listed without --all:\n%s", folded)
	}

	// The other half: folded is not gone. The address still reads, and --all still
	// lists it — without this the test would pass on a fix that simply forgot them.
	all := termText(t, s, map[string]any{"cmd": "ls --all"})
	if !strings.Contains(all, h) {
		t.Errorf("ls --all must list the channels the default view folded:\n%s", all)
	}
	if !strings.HasPrefix(termText(t, s, map[string]any{"cmd": "read " + h + " --offset 0"}), "TERM/1 2") {
		t.Errorf("a folded channel's output must still be readable:\n%s", h)
	}
}
