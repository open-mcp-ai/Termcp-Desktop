package mcp

import (
	"strings"
	"testing"
	"time"
)

// TestARolledBackWindowSaysSo.
//
// A byte window never ends inside a character: the bytes that would have been cut are
// withheld and the next read delivers them whole. Lossless — but silent, and "I got
// fewer bytes than bytes_limit" looks exactly like "a character straddled the end". A
// caller that pages by its own arithmetic (offset += bytes_limit) then skips the
// withheld bytes and lands INSIDE a character, where the reply is a 400 naming a
// charset the session does not write. The header is what makes the rollback a fact
// instead of a mystery.
func TestARolledBackWindowSaysSo(t *testing.T) {
	s := newTestServer(t)
	// Two ASCII bytes, then a 3-byte character: a 4-byte window ends two bytes into it.
	// The escapes are doubled so the SHELL's printf makes the bytes: a lone 0xE4 in the
	// command line is not valid UTF-8, and the parser refuses the command before it runs.
	h := openPipe(t, s, "printf 'AB\\344\\270\\255TAIL'")
	time.Sleep(400 * time.Millisecond)

	got := termText(t, s, map[string]any{"cmd": "read " + h + " --offset 0", "bytes_limit": 4, "timeout": 2})
	if !strings.Contains(got, "Trimmed: bytes 2-3 withheld") {
		t.Fatalf("the window rolled back two bytes without saying so:\n%s", got)
	}
	// The remedy has to be in the header: the withheld bytes are one cursor away.
	if !strings.Contains(got, "read --cursor 2 continues there") {
		t.Errorf("the header does not say how to get the withheld bytes:\n%s", got)
	}
	if body := replyBody(got); strings.Contains(body, "中") {
		t.Errorf("a 4-byte window cannot hold the whole character; it should have rolled back:\n%s", body)
	}

	// And nothing is announced when nothing was withheld: a header on every reply is a
	// header nobody reads.
	clean := termText(t, s, map[string]any{"cmd": "read " + h + " --offset 0", "timeout": 2})
	if strings.Contains(clean, "Trimmed") {
		t.Errorf("a window that ends on a character boundary claims a rollback:\n%s", clean)
	}
}

// TestAFragmentAtTheEndOfTheStreamIsNotPromisedAway.
//
// Mid-stream the withheld bytes are simply early — the next read starts at them and gets
// them whole, so "read --cursor A continues there" is true. At the END of the stream they
// are a character the writer never finished: no read can complete them, and the same
// header would send the caller into a loop that returns nothing forever. Same count,
// different fact, so the two cases must not share a sentence.
func TestAFragmentAtTheEndOfTheStreamIsNotPromisedAway(t *testing.T) {
	s := newTestServer(t)
	// "AB" then the first byte of a 3-byte character, and nothing more: the stream ends
	// inside a character.
	h := openPipe(t, s, "printf 'AB\\344'")
	time.Sleep(400 * time.Millisecond)

	got := termText(t, s, map[string]any{"cmd": "read " + h + " --offset 0", "timeout": 2})
	if !strings.Contains(got, "Trimmed: bytes 2-2 withheld") {
		t.Fatalf("the trailing fragment was withheld without saying so:\n%s", got)
	}
	if !strings.Contains(got, "do not form a character in this encoding") {
		t.Errorf("the header does not say the fragment is final:\n%s", got)
	}
	if strings.Contains(got, "--cursor 2 continues") {
		t.Errorf("the header promises a read that cannot return those bytes:\n%s", got)
	}
	// And the readable part still arrives: a fragment at the end must not cost the
	// caller the text in front of it.
	if body := replyBody(got); !strings.Contains(body, "AB") {
		t.Errorf("the window withheld the text along with the fragment:\n%s", got)
	}
}
