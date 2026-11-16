package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestFileReplaceEditsExactlyOneOccurrence pins feedback §2.3: the edit primitive a
// single-tool agent needs. Without it, changing one line of a config means reading
// the whole file and writing it back — losing anything that changed in the round
// trip, and paying for the whole file in tokens.
func TestFileReplaceEditsExactlyOneOccurrence(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	sessID := strings.Split(h, ":")[0]

	target := filepath.Join(t.TempDir(), "agent.conf")
	if err := os.WriteFile(target, []byte("port = 80\ntimeout = 5\nretries = 3\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := termText(t, s, map[string]any{"cmd": "write " + sessID +
		" file " + target +
		" --old-text 'timeout = 5' --new-text 'timeout = 30'"})
	if !strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("replace must succeed:\n%s", got)
	}
	if !strings.Contains(got, "bytes_written") {
		t.Errorf("replace must report bytes_written so the caller need not re-read:\n%s", got)
	}

	after, _ := os.ReadFile(target)
	want := "port = 80\ntimeout = 30\nretries = 3\n"
	if string(after) != want {
		t.Errorf("content:\n got %q\nwant %q", string(after), want)
	}
}

// TestFileReplaceRefusesAnAmbiguousMatch pins the uniqueness constraint. An agent
// that asks to change a line appearing three times must be told so and left with an
// untouched file: silently picking the first is how an edit lands in the wrong place
// and is noticed only when the wrong service misbehaves.
func TestFileReplaceRefusesAnAmbiguousMatch(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	sessID := strings.Split(h, ":")[0]

	target := filepath.Join(t.TempDir(), "multi.conf")
	original := "value = 1\nvalue = 1\nvalue = 1\n"
	if err := os.WriteFile(target, []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	got := termText(t, s, map[string]any{"cmd": "write " + sessID +
		" file " + target +
		" --old-text 'value = 1' --new-text 'value = 2'"})
	if strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("an ambiguous match must be refused:\n%s", got)
	}
	if !strings.Contains(got, "3") {
		t.Errorf("the error must state the match count:\n%s", got)
	}

	after, _ := os.ReadFile(target)
	if string(after) != original {
		t.Errorf("file was mutated on an ambiguous match: %q", string(after))
	}
}

// TestFileReplaceRefusesAMissingPattern pins the zero-match case, which is the
// other way a caller can be wrong about the file's contents.
func TestFileReplaceRefusesAMissingPattern(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	sessID := strings.Split(h, ":")[0]

	target := filepath.Join(t.TempDir(), "plain.conf")
	if err := os.WriteFile(target, []byte("alpha\nbeta\n"), 0644); err != nil {
		t.Fatal(err)
	}

	got := termText(t, s, map[string]any{"cmd": "write " + sessID +
		" file " + target +
		" --old-text 'gamma' --new-text 'delta'"})
	if strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("a missing pattern must be reported:\n%s", got)
	}
	if !strings.Contains(got, "gamma") {
		t.Errorf("the error must name the text that was not found:\n%s", got)
	}
}
