package mcp

import (
	"strings"
	"testing"
)

// TestFoldSaysWhatItHid pins feedback §5: a fold used to answer 206 with an empty
// body and a range, which is exactly what "no new output" looks like too. The caller
// could not tell "folded on purpose" from "nothing there", and had to trust bytes
// existed somewhere it could not name.
func TestFoldSaysWhatItHid(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{"seq 1 50"}, "timeout": 2})
	// WAIT FOR THE BYTES, DO NOT ASSUME THE WINDOW WAS ENOUGH. A pty has no "the command
	// finished" signal, so `run` returns whatever arrived in its window; on a loaded
	// machine that can be fewer than five lines, in which case nothing overflows and
	// the fold this test is about never happens. The same race `waitForGrepContent`
	// documents — passed alone, failed in the full run.
	waitForGrepContent(t, s, h, "\n50\n")

	got := termText(t, s, map[string]any{
		"cmd":               "read " + h + " --from-line 1",
		"lines_limit":       float64(5),
		"on_lines_overflow": "fold",
		"timeout":           float64(1),
	})
	if !strings.Contains(got, "Folded:") {
		t.Fatalf("a folded reply must say what it withheld:\n%s", got)
	}
	if !strings.Contains(got, "read --cursor") {
		t.Errorf("the Folded header must name the way to fetch the rest:\n%s", got)
	}
}

// TestNoFoldHeaderWhenNothingWasFolded pins the other side: a reply that returned
// everything must not carry the header, or it stops meaning anything.
func TestNoFoldHeaderWhenNothingWasFolded(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	awaitPtyReady(t, s, h)

	// THE MARKER IS NOT IN THE ECHO. `echo SHORT` puts SHORT in the transcript twice —
	// once as the typed line and once as the output — so a wait for it is satisfied by
	// the echo and synchronises nothing. Running a script file keeps the command line
	// (`sh /tmp/…/probe.sh`) free of the word the script prints.
	script := writeScript(t, "printf 'SHORT\\n'")
	termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{"sh '" + script + "'"}, "timeout": 2})
	// WAIT FOR THE OUTPUT, DO NOT ASSUME THE WINDOW WAS ENOUGH. A pty has no "the
	// command finished" signal, so `run` returns whatever arrived inside its window and
	// on a loaded machine that can be nothing at all — the read below then sees an empty
	// transcript, contains no Folded header (correctly), and fails on the second
	// assertion for a reason that has nothing to do with folding. This is the third
	// instance of the same race in #189: passed alone, failed in the full run.
	waitForGrepContent(t, s, h, "SHORT")
	got := termText(t, s, map[string]any{"cmd": "read " + h + " --from-line 1", "timeout": float64(1)})
	if strings.Contains(got, "Folded:") {
		t.Errorf("nothing was folded, so the header must be absent:\n%s", got)
	}
	if !strings.Contains(got, "SHORT") {
		t.Errorf("the bytes should be there:\n%s", got)
	}
}
