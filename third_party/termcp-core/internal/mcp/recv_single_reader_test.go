package mcp

import (
	"strings"
	"sync"
	"testing"
)

// ONE FRAME, ONE READER (#183).
//
// The cursor is a read POSITION, and "who gets which bytes" is undefined while two
// readers advance the same position: both read `from`, both find the same frame, both
// return it. The cursor does not go backwards, so nothing is LOST — the contract just
// degrades from exactly-once to at-least-once, which is the one thing JSON-RPC id
// pairing cannot survive: a client that receives two `id:3` and no `id:4` waits for an
// answer that was already consumed by the other reader.
//
// This is reachable, not theoretical. An extension's channel is named after the
// extension, and that session is kept warm for reuse — so two agents driving the same
// extension are two readers on one channel.
func TestTwoConcurrentRecvsDoNotBothGetTheSameFrame(t *testing.T) {
	s := newTestServer(t)
	// The frame arrives AFTER both reads are already polling, so both are inside the
	// window when it lands — the shape that makes the race certain rather than likely.
	h := openPipe(t, s, `sh -c 'sleep 1; printf "{\"jsonrpc\":\"2.0\",\"id\":1}\n"; `+
		`sleep 1; printf "{\"jsonrpc\":\"2.0\",\"id\":2}\n"'`)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	var wg sync.WaitGroup
	replies := make([]string, 2)
	for i := range replies {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			replies[i] = termText(t, s, map[string]any{"cmd": "read " + h + " --frame", "timeout": 8})
		}(i)
	}
	wg.Wait()

	frames := make([]string, 2)
	for i, r := range replies {
		if !statusLineIs(r, "200") {
			t.Fatalf("reader %d did not get a frame:\n%s", i, r)
		}
		frames[i] = frameBody(r)
	}
	if frames[0] == frames[1] {
		t.Fatalf("both readers were handed the same frame (%s); one frame has one reader", frames[0])
	}
}

// frameBody pulls the body out of a TERM/1 reply, which is the frame itself here.
func frameBody(reply string) string {
	_, body, found := strings.Cut(reply, "\n\n")
	if !found {
		return strings.TrimSpace(reply)
	}
	return strings.TrimSpace(body)
}
