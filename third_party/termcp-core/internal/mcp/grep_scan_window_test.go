package mcp

import (
	"runtime"
	"strings"
	"testing"
)

// TestAGrepSaysWhenItDidNotScanEverything is the last finding in issue #204's review.
//
// One --grep reads at most grepMaxScanBytes of transcript. That bound is a legitimate
// protection — "the last 20 errors" should not sweep a 200 MB log — but a match count
// from a PARTIAL scan reads exactly like one from a complete scan: "3 matches" in a
// channel twice the scan window means three matches in the part that was looked at, and
// the caller has no way to know. A cap that announces itself is a different thing from
// one that hides, which is the useful half of the issue's rule.
func TestAGrepSaysWhenItDidNotScanEverything(t *testing.T) {
	var script string
	if runtime.GOOS == "windows" {
		script = `1..400 | ForEach-Object { "line-$_" }; Start-Sleep -m 300`
	} else {
		script = `seq 1 400 | sed 's/^/line-/'; sleep 0.3`
	}
	s, _, shellID := startPipeStdioSession(t, script)
	waitForOutput(t, s, shellID, "line-400")

	// A window far smaller than the transcript, so the scan cannot reach the end.
	saved := grepMaxScanBytes
	grepMaxScanBytes = 200
	t.Cleanup(func() { grepMaxScanBytes = saved })

	got := termText(t, s, map[string]any{"cmd": "read " + shellID + " --grep line-", "timeout": 1})
	if !strings.Contains(got, "Scan-Window:") {
		t.Fatalf("a scan that stopped after %d bytes of a %d byte channel must say so:\n%.600s",
			grepMaxScanBytes, len(strings.Repeat("line-400\n", 40)), got)
	}
	if !strings.Contains(got, "was not scanned") {
		t.Errorf("Scan-Window must say the rest was not searched:\n%s", got)
	}

	// And the whole transcript, scanned, carries no such header: it would be noise on
	// every ordinary grep, and noise is how a real one gets skimmed past.
	grepMaxScanBytes = 8 << 20
	clean := termText(t, s, map[string]any{"cmd": "read " + shellID + " --grep line-", "timeout": 1})
	if strings.Contains(clean, "Scan-Window:") {
		t.Errorf("a complete scan must not claim it was partial:\n%.600s", clean)
	}
}
