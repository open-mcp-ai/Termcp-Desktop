package mcp

import (
	"strings"
	"testing"
)

// TestTheLocalEntryNamesItsShell pins the column the second report needed.
//
// `local windows` narrowed it to the right OS and the caller still typed `ls -la`
// into PowerShell — a PATH-supplied `uname` answering `MINGW64_NT` confirmed the wrong
// guess. Which shell a local session gets is a fact only the server has (pwsh vs
// powershell vs cmd depends on PATH), and it decides whether the first command works.
func TestTheLocalEntryNamesItsShell(t *testing.T) {
	s := newTestServer(t)
	out := termText(t, s, map[string]any{"cmd": "ls host"})

	line := ""
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, "internal\t") {
			line = l
		}
	}
	if line == "" {
		t.Skipf("no internal profile in this environment:\n%s", out)
	}
	if !strings.Contains(line, "\tlocal\t") {
		t.Fatalf("the local profile must be marked local, got %q", line)
	}
	// A shell name, not just the OS: the third field is what the caller will be
	// typing into.
	fields := strings.Split(line, "\t")
	if len(fields) < 3 || len(strings.Fields(fields[2])) < 2 {
		t.Fatalf("the local entry names an OS but not a shell, so the first command is a guess: %q", line)
	}
}

// TestTheGuideSaysWhichFileVerbCarriesBytes pins the `get` vs `read` distinction.
//
// The second report read `get <session> file <path>` as "give me the file" and got
// upload/download URLs — both verbs looked like the two halves of one pair, and the
// four file verbs were listed with no hint of which does what.
func TestTheGuideSaysWhichFileVerbCarriesBytes(t *testing.T) {
	s := newTestServer(t)
	out := termText(t, s, map[string]any{})

	for _, l := range strings.Split(out, "\n") {
		if !strings.HasPrefix(l, "  file ") {
			continue
		}
		if !strings.Contains(out, "read/write carry BYTES") {
			t.Errorf("the file line must say which verbs carry bytes:\n%s", l)
		}
		return
	}
	t.Fatalf("the guide no longer names the file node:\n%s", out)
}
