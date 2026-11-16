package buffer

import (
	"strings"
	"testing"
)

// TestTheBufferAppliesTheCallersRuleAndNothingElse pins the seam.
//
// The buffer used to carry a UTF-8 rule of its own. That was the wrong layer twice
// over: a byte log has no business knowing what a character is, and the rule only
// covered one charset — so a GBK session kept the bug the rule was added to fix, and
// kept it silently. Now the rule arrives with the call, and this file pins that the
// buffer obeys it exactly: trims what it says, keeps those bytes UNREAD, and does
// nothing at all when it says nothing.
func TestTheBufferAppliesTheCallersRuleAndNothingElse(t *testing.T) {
	b := New(0)
	// a, then a 3-byte rune, then b and a newline.
	whole := append([]byte{'a'}, append([]byte("日"), 'b', '\n')...)
	if err := b.Write(whole); err != nil {
		t.Fatal(err)
	}
	id, err := b.NewReaderFromStart()
	if err != nil {
		t.Fatal(err)
	}

	// A rule that says "the last byte of a 3-byte window is unfinished" — the buffer
	// must hand back 2 and leave 1 unread.
	seen := 0
	trim := func(window []byte) int {
		seen++
		if len(window) == 3 {
			return 1
		}
		return 0
	}
	first, err := b.ReadLimited(t.Context(), id, 0, 3, 0, trim)
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if len(first) != 2 {
		t.Fatalf("first window = % x (%d bytes), want 2: the rule said withhold 1", first, len(first))
	}
	if seen == 0 {
		t.Error("the rule was never consulted")
	}

	// The withheld byte is still there, and the cursor did not skip it.
	rest, err := b.ReadLimited(t.Context(), id, 0, 0, 0, trim)
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if got, want := string(first)+string(rest), string(whole); got != want {
		t.Errorf("the reads reassemble to %q, want %q", got, want)
	}
	if !strings.HasPrefix(string(rest), string(whole[2:3])) {
		t.Errorf("the withheld byte was skipped: rest starts % x, want the byte after the window", rest)
	}
}

// TestNoRuleMeansCutExactlyWhereIAsked is the other half: a caller reading raw bytes
// gets its byte count, whatever falls in the middle. The browser repaints a terminal
// from these bytes and a terminal emulator is byte-exact, so a withheld byte would be
// a character that never appeared.
func TestNoRuleMeansCutExactlyWhereIAsked(t *testing.T) {
	b := New(0)
	if err := b.Write(append([]byte{'a'}, append([]byte("日"), 'b')...)); err != nil {
		t.Fatal(err)
	}
	id, err := b.NewReaderFromStart()
	if err != nil {
		t.Fatal(err)
	}
	got, err := b.ReadLimited(t.Context(), id, 0, 2, 0, nil)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("window = % x (%d bytes), want exactly 2", got, len(got))
	}
}
