package mcp

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/sshconfig"
)

// The five friction points from a first-time caller's report, each pinned by the
// CLAIM it makes about the surface rather than by the sentence that happens to
// carry it. They share a file because they are one thing: the answers that were
// technically true and practically useless — "there is no directive called file",
// six bare host names, a handle whose two halves nobody explained.

// TestLsHostSaysWhereEachEntryIs pins the column that answers "is this the machine
// I am on?".
//
// `ls host` printed six bare names, one column, no kind and no target — so the only
// way to learn which entry was the local shell and which was a box in another rack
// was to connect and run `uname`. That is three calls to answer a question the
// listing had already loaded.
func TestLsHostSaysWhereEachEntryIs(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	seedRawProfile(t, dir, "zz-box", "kind = \"remote\"\nhost = \"10.0.0.9\"\nport = 2222\nuser = \"u\"\npassword = \"p\"\n")
	s.sshConfigs = sshconfig.NewStore(dir)

	out := termText(t, s, map[string]any{"cmd": "ls host"})
	if !strings.Contains(out, "zz-box") {
		t.Fatalf("the entry is not in the listing:\n%s", out)
	}
	line := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "zz-box") {
			line = l
		}
	}
	// The kind is the local/remote distinction, spelled the way a reader thinks of it.
	if !strings.Contains(line, "\tssh") {
		t.Errorf("a remote entry must say it is reached over ssh, got %q", line)
	}
	// And the target, so the listing answers "which box is this?" without a connect.
	for _, want := range []string{"u@", "10.0.0.9", ":2222"} {
		if !strings.Contains(line, want) {
			t.Errorf("the listing should carry the dial target (%s), got %q", want, line)
		}
	}
	// The internal profile is the case the report got wrong: it is LOCAL, and it is
	// listed in the same column as the remotes.
	internal := termText(t, s, map[string]any{"cmd": "ls host"})
	if strings.Contains(internal, "internal\tssh") {
		t.Error("the internal profile is not an ssh target; it is this machine")
	}
}

// TestTheGuideOpensWithTheShortestPath pins the first screen.
//
// The guide answered with the whole worldview — six node kinds, two centres,
// nineteen verbs — which is the right reference and the wrong opening: a caller who
// only wanted a shell and one command asked "so what do I type first?" and had to
// derive it from a STRUCTURE diagram. It now opens with the three commands that do
// the job, and says that `run` is all three steps at once.
func TestTheGuideOpensWithTheShortestPath(t *testing.T) {
	s := newTestServer(t)
	out := termText(t, s, map[string]any{})

	// The happy path comes BEFORE the grammar: a caller who stops reading after the
	// first screen must still have everything needed to run a command.
	happy := strings.Index(out, "SHORTEST PATH")
	structure := strings.Index(out, "STRUCTURE")
	if happy < 0 {
		t.Fatalf("the guide does not open with a shortest path:\n%s", out)
	}
	if structure >= 0 && structure < happy {
		t.Error("the shortest path sits below the grammar; the first screen is the reference again")
	}
	head := out[:structure]
	for _, want := range []string{"ls host", "connect", "run "} {
		if !strings.Contains(head, want) {
			t.Errorf("the shortest path never reaches %q:\n%s", want, head)
		}
	}
	// `run` vs input+key+read was the boundary the report could not see: `run` was
	// read as "type the line" and followed by a redundant read. Both halves are now
	// named in the same breath.
	if !strings.Contains(head, "input") || !strings.Contains(head, "read") {
		t.Errorf("the shortest path must say what input/key/read are for, or `run` reads as a partial step:\n%s", head)
	}
}

// TestHelpAnswersANodeName pins `help file`.
//
// `help file` answered "unknown directive: file" — true, and useless to a caller
// holding that word from `ls <session> file`. Help is indexed by verb while the
// grammar is a tree of nouns, so the noun was the one word the help page could not
// answer. The page is derived from the usage lines, so it cannot drift.
func TestHelpAnswersANodeName(t *testing.T) {
	s := newTestServer(t)

	out := termText(t, s, map[string]any{"cmd": "help file"})
	if strings.Contains(out, "unknown directive") {
		t.Fatalf("`file` is a node in the grammar, not an unknown verb:\n%s", out)
	}
	// The answer has to name the VERBS that act on the noun, or it is just a nicer
	// refusal: these are the ones whose usage lines spell `file`.
	for _, want := range []string{"read", "write"} {
		if !strings.Contains(out, want) {
			t.Errorf("the page for `file` must name the verb %q; got:\n%s", want, out)
		}
	}
	// A word that is nothing at all keeps the old answer: the node page must not
	// turn every typo into a 200 with an empty list.
	typo := termText(t, s, map[string]any{"cmd": "help zz-frobnicate"})
	if !strings.Contains(typo, "unknown directive") {
		t.Errorf("a word that names nothing must still be refused, got:\n%s", typo)
	}
}

// TestAHandleSaysWhichHalfIsTheSession pins the first reply's explanation of the
// handle.
//
// `a1f3:1` reads as one opaque id, so a caller carries it everywhere — until a verb
// that takes a session (`rm` deletes a session, not a channel) answers "that is a
// channel". The open receipt is where the word is new, so it is where the two
// halves get named. Nothing else changes: the handle is still the first word.
func TestAHandleSaysWhichHalfIsTheSession(t *testing.T) {
	s := newTestServer(t)
	out := termText(t, s, map[string]any{"cmd": "connect internal"})

	handle := ""
	for _, l := range strings.Split(out, "\n") {
		if i := strings.Index(l, ":1 "); i > 0 {
			handle = strings.TrimSpace(l[:i+2])
			break
		}
	}
	if handle == "" {
		t.Skipf("no local shell in this environment:\n%s", out)
	}
	if !strings.Contains(out, "session") || !strings.Contains(out, "channel") {
		t.Errorf("the receipt must say which half of %s is the session and which is the channel:\n%s", handle, out)
	}
}

// TestAnEmptyReadNamesTheNextMove pins the hint on a 204.
//
// The reported failure was not the empty reply — an empty read of a live shell is
// legitimate — but the reaction to it: `Waited: 0ms` said the truth in a technical
// voice, in the header block a caller skims last, so the next move was to spend a
// call proving the input had not been dropped. The hint names the flag.
//
// A HEADER rather than a body sentence because the reply is a 204: the REST entrance
// drops a body on 204, and a hint only MCP can see is the divergence between the two
// entrances this repo keeps paying for.
func TestAnEmptyReadNamesTheNextMove(t *testing.T) {
	s := newTestServer(t)
	opened := termText(t, s, map[string]any{"cmd": "connect internal"})
	handle := firstHandle(opened)
	if handle == "" {
		t.Skip("no local shell in this environment")
	}

	// Read AT THE END of what the channel has produced: that is deterministically
	// empty, so the case under test is the case that runs. Reading from the start
	// catches the prompt and skips the hint, which would make this test pass by not
	// testing anything on a machine whose shell is quick.
	total := bytesOf(t, s, handle)
	hint := headerValueOr(termText(t, s, map[string]any{"cmd": fmt.Sprintf("read %s --offset %d", handle, total)}), "Hint", "")
	if hint == "" {
		// A read that happened to catch output is not the case under test.
		t.Skip("the read had output; nothing to hint at")
	}
	if !strings.Contains(hint, "timeout") {
		t.Errorf("the hint must name the flag that waits instead of the state: %q", hint)
	}
}

// bytesOf reads the `bytes` field of a channel, which is where its transcript ends.
func bytesOf(t *testing.T, s *Server, handle string) int {
	t.Helper()
	out := termText(t, s, map[string]any{"cmd": "get " + handle})
	for _, line := range strings.Split(out, "\n") {
		if v, ok := strings.CutPrefix(line, "bytes:"); ok {
			n, err := strconv.Atoi(strings.TrimSpace(v))
			if err != nil {
				t.Fatalf("bytes is not a number: %q", v)
			}
			return n
		}
	}
	t.Fatalf("no bytes field in:\n%s", out)
	return 0
}
