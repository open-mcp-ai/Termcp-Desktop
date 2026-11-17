package mcp

import (
	"runtime"
	"strings"
	"testing"
)

// TestAByteWindowDoesNotSplitACharacter pins the bug that made a byte window refuse
// itself, end to end.
//
// `bytes_limit` says how much of the reply the caller wants; it does not say "break a
// character in half". The window was cut wherever the count fell, so an incomplete rune
// reached the byte-fidelity check as INVALID UTF-8 — and a perfectly good UTF-8 session
// was answered with a 400 naming shift_jis. The advice (`--encoding shift_jis`) would
// have made it worse.
//
// The window below is chosen so the cut CAN land inside a character: a three-byte rune
// with a two-byte window. What must happen is that the first read returns the bytes that
// are whole, and the next read delivers the rest — nothing lost, nothing refused.
func TestAByteWindowDoesNotSplitACharacter(t *testing.T) {
	// "日" is 3 bytes, so a 2-byte window must stop before it. The bytes are written
	// directly on both platforms: a quoting layer that turned them into text would be
	// testing something else entirely.
	write := `printf 'a\346\227\245b'`
	if runtime.GOOS == "windows" {
		write = `[Console]::OpenStandardOutput().Write([byte[]](0x61,0xE6,0x97,0xA5,0x62),0,5)`
	}
	s, shellID := runPipeAndRead(t, write)

	first := termText(t, s, map[string]any{
		"cmd":         "read " + shellID,
		"bytes_limit": float64(2),
		"timeout":     float64(2),
	})
	if strings.Contains(first, "not valid UTF-8") {
		t.Fatalf("a 2-byte window over a 3-byte rune was refused instead of ending early:\n%s", first)
	}
	if !strings.Contains(first, "a") {
		t.Fatalf("the first window should have carried the leading ASCII byte:\n%s", first)
	}

	// The rest arrives whole, and together they are the original bytes.
	rest := termText(t, s, map[string]any{
		"cmd":         "read " + shellID,
		"bytes_limit": float64(64),
		"timeout":     float64(2),
	})
	if strings.Contains(rest, "not valid UTF-8") {
		t.Fatalf("the continuation was refused too:\n%s", rest)
	}
	for _, part := range []string{"日", "b"} {
		if !strings.Contains(rest, part) {
			t.Errorf("the withheld bytes did not arrive on the next read (missing %q):\n%s", part, rest)
		}
	}
}

// TestAByteWindowDoesNotSplitADeclaredCharsetCharacter is the same property for a
// session that is not UTF-8 — the case that was SILENT.
//
// A truncated multi-byte sequence in a declared charset does not fail the UTF-8 check:
// the decoder turns it into U+FFFD and the check passes, because U+FFFD is valid UTF-8.
// So the caller got text with a replacement character at the end, no error, and offsets
// that no longer matched the stream. The rule has to exist per charset, and it has to be
// applied where the cut happens — which is why the charset travels with the read.
func TestAByteWindowDoesNotSplitADeclaredCharsetCharacter(t *testing.T) {
	// "中" in GBK is 0xd6 0xd0; a 2-byte window ("a" + the first byte) must stop after "a".
	//
	// THE OCTAL ESCAPES ARE DOUBLED ON PURPOSE, so they are still backslashes in the Go
	// string and printf converts them at RUNTIME. A single backslash here is a Go escape:
	// the raw bytes 0xD6 0xD0 end up inside an argv element, and they do not survive that
	// trip — the internal SSH server renders them as the text `\xd6\xd0`, so the channel
	// carries ten bytes of literal escape instead of the four this test is about (CI on
	// Linux reported Content-Range 0-9/10). Nothing here needs a raw byte to cross it.
	write := "printf 'a\\326\\320b'"
	if runtime.GOOS == "windows" {
		write = `[Console]::OpenStandardOutput().Write([byte[]](0x61,0xD6,0xD0,0x62),0,4)`
	}
	s, shellID := runPipeAndRead(t, write)

	got := termText(t, s, map[string]any{
		"cmd":         "read " + shellID + " --encoding gbk",
		"bytes_limit": float64(2),
		"timeout":     float64(2),
	})
	if strings.Contains(got, "\ufffd") {
		t.Fatalf("the window ended in a replacement character; the cut split a GBK sequence:\n%s", got)
	}
	if strings.Contains(got, "not valid UTF-8") {
		t.Fatalf("a window that ends on a character boundary must not be refused:\n%s", got)
	}
	if !strings.Contains(got, "a") {
		t.Fatalf("the first window should have carried the leading ASCII byte:\n%s", got)
	}

	// The withheld bytes arrive whole on the next read.
	rest := termText(t, s, map[string]any{
		"cmd":         "read " + shellID + " --encoding gbk",
		"bytes_limit": float64(64),
		"timeout":     float64(2),
	})
	for _, part := range []string{"中", "b"} {
		if !strings.Contains(rest, part) {
			t.Errorf("the withheld bytes did not arrive on the next read (missing %q):\n%s", part, rest)
		}
	}
	if strings.Contains(rest, "\ufffd") {
		t.Errorf("the continuation carries a replacement character:\n%s", rest)
	}
}
