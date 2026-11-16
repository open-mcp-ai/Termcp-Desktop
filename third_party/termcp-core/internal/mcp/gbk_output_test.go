package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

// gbkBytes is 以下 in GBK — two two-byte sequences, and NOT valid UTF-8.
const gbkBytes = "\xd2\xd4\xcf\xc2"

// gbkSession starts a channel whose OUTPUT is GBK bytes, whatever the platform.
//
// It writes the four bytes rather than printing a string, because the point of the
// issue is that a session's output is not necessarily UTF-8, and a Go string literal
// in the test would be UTF-8 by construction. A pipe session is used rather than a
// login shell so that nothing depends on a profile, a prompt, or how long a shell
// takes to start: the program writes four bytes and exits, and those bytes are the
// channel's output.
func gbkSession(t *testing.T, s *Server) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		// QUOTED, because the directive line is shell syntax: the parentheses and the
		// brackets of a PowerShell expression are not words otherwise.
		return openPipe(t, s, `pwsh -NoLogo -NoProfile -Command '[Console]::OpenStandardOutput().Write([byte[]](0xD2,0xD4,0xCF,0xC2,0x0A),0,5)'`)
	}
	// THE BYTES COME FROM A FILE, because the directive line is TEXT and the parser
	// refuses a cmd that is not valid UTF-8 — which is the right rule for a command
	// language and exactly what this test needs to get around: its whole subject is a
	// session whose OUTPUT is GBK. `printf '\322…'` does not work either: the escaping
	// rules of the DSL and of sh's printf would have to agree about backslashes, and that
	// agreement is what this cycle changed. A file has no escaping in it at all.
	script := writeScript(t, "cat "+gbkFixture(t))
	return openPipe(t, s, "sh "+script)
}

// gbkFixture writes 以下 in GBK (D2 D4 CF C2) plus a newline, and returns the path.
func gbkFixture(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "gbk.txt")
	if err := os.WriteFile(path, []byte{0xD2, 0xD4, 0xCF, 0xC2, 0x0A}, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

// waitForGbkOutput polls until the channel's window decodes, without consuming it.
//
// `waitForContent` cannot be used here: it looks for its needle in the reply, and the
// whole point of the case is that an undeclared non-UTF-8 window has NO reply to look
// in — the bytes only become text once the encoding is named.
func waitForGbkOutput(t *testing.T, s *Server, h string) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		got := termText(t, s, map[string]any{"cmd": "read " + h + " --from-line 1 --encoding gbk"})
		if strings.Contains(got, "以下") {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("the GBK window never became readable on %s", h)
}

// TestGbkOutputIsReadableWhenTheCallerNamesTheEncoding is issue #149, items 1 and 2.
//
// On a Chinese Windows the shell writes GBK, and the daemon assumed UTF-8: the read
// refused the whole window, and the refusal named two remedies that do not exist on a
// channel (`mode=hex`, `mode=file` are file reads), so the session's own output could
// not be read at all. The frame case is the same failure one layer out — an MCP
// extension that prints GBK had no readable reply.
//
// `--encoding` is the answer rather than a guess: the bytes are decoded as what the
// caller says they are, so the reply is text, `--grep` compares characters, and the
// byte offsets in Content-Range still refer to the buffer.
func TestGbkOutputIsReadableWhenTheCallerNamesTheEncoding(t *testing.T) {
	s := newTestServer(t)
	h := gbkSession(t, s)
	waitForGbkOutput(t, s, h)

	// Without the encoding the window is refused, and the refusal has to be
	// actionable: it names what the bytes are, checked by a round trip, not guessed.
	plain := termText(t, s, map[string]any{"cmd": "read " + h + " --tail 20"})
	if !strings.HasPrefix(plain, "TERM/1 4") {
		t.Fatalf("a GBK window was accepted as UTF-8:\n%s", plain)
	}
	if !strings.Contains(plain, "--encoding") {
		t.Errorf("the refusal does not name a flag this build answers:\n%s", plain)
	}
	if !strings.Contains(plain, "gbk") {
		t.Errorf("the refusal does not name the charset the bytes confirm:\n%s", plain)
	}
	if strings.Contains(plain, "mode=hex") {
		t.Errorf("the refusal still points at a file-read mode that no channel read has:\n%s", plain)
	}

	// With it, the same window is text.
	got := termText(t, s, map[string]any{"cmd": "read " + h + " --tail 20 --encoding gbk"})
	if !strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("--encoding gbk did not read the window:\n%s", got)
	}
	if !strings.Contains(got, "以下") {
		t.Errorf("the decoded window does not hold the characters that were written:\n%s", got)
	}

	// An unknown charset is a caller mistake, named as one, not a silent UTF-8 read.
	bad := termText(t, s, map[string]any{"cmd": "read " + h + " --tail 20 --encoding klingon"})
	if !strings.HasPrefix(bad, "TERM/1 4") {
		t.Errorf("an unknown charset was accepted:\n%s", bad)
	}
	if !strings.Contains(bad, "klingon") {
		t.Errorf("the error does not name the charset that was refused:\n%s", bad)
	}
}

// TestGrepOnGbkOutputMatchesCharacters is issue #149, item 3.
//
// The report was the quietest of the three: `--grep 以下` compared a UTF-8 pattern
// against GBK bytes, scanned 282 real lines, matched none, and answered 200 — a
// success that cannot be told apart from "that string is not there". Decoding before
// matching is what makes the answer mean what it says.
func TestGrepOnGbkOutputMatchesCharacters(t *testing.T) {
	s := newTestServer(t)
	h := gbkSession(t, s)
	waitForGbkOutput(t, s, h)

	got := termText(t, s, map[string]any{"cmd": "read " + h + " --grep 以下 --encoding gbk"})
	if !strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("grep with the encoding did not answer:\n%s", got)
	}
	if !strings.Contains(got, "以下") {
		t.Errorf("grep did not match the characters that are in the transcript:\n%s", got)
	}
}
