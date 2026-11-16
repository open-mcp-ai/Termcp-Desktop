package mcp

import (
	"path/filepath"
	"strings"
	"testing"
)

// TestWriteDecodesTheHexPrefixTheHelpPagePromises pins issue #140.
//
// `help write` documents `--data hex:00ff` as the way to write binary, but nothing
// decoded the prefix: the ten characters `hex:414243` went into the file as text and
// the reply said 200 with an honest-looking `bytes_written: 10`.
//
// That is the one failure worth spending code on in this surface, because it is
// SILENT. Every other bad argument here is refused by name; this one produces a file
// that is neither the intended bytes nor recognisably wrong, and the caller has no
// reason to look.
func TestWriteDecodesTheHexPrefixTheHelpPagePromises(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sleep 30")
	path := filepath.Join(t.TempDir(), "p4.bin")

	wrote := termText(t, s, map[string]any{"cmd": "write " + h + " file " + path + " --data hex:414243"})
	if code := statusCode(t, wrote); code >= 400 {
		t.Fatalf("write was refused (%d):\n%s", code, wrote)
	}
	// The count has to be the DECODED length, or the receipt is the only thing that
	// looks right while the file is wrong.
	if !strings.Contains(wrote, "bytes_written: 3") {
		t.Errorf("bytes_written is not the decoded length:\n%s", wrote)
	}
	// ABC is ASCII, so a text read proves the CONTENT: had the prefix been written
	// literally the body would be `hex:414243` and the size 10.
	read := termText(t, s, map[string]any{"cmd": "read " + h + " file " + path})
	if !strings.Contains(read, "ABC") || strings.Contains(read, "hex:") {
		t.Errorf("the file does not contain the decoded bytes: %s", strings.ReplaceAll(read, "\n", " | "))
	}

	// A text payload that happens to start with the same letters is still text: the
	// prefix is only a prefix when something follows it.
	text := filepath.Join(t.TempDir(), "p5.txt")
	termText(t, s, map[string]any{"cmd": "write " + h + " file " + text + " --data hex:"})
	read = termText(t, s, map[string]any{"cmd": "read " + h + " file " + text})
	if !strings.Contains(read, "total_size: 0") {
		t.Errorf("`hex:` alone was not decoded as an empty payload:\n%s", read)
	}

	// --mode hex still means "this payload IS hex", so it must not also read a prefix
	// off the front and choke on the letters.
	explicit := filepath.Join(t.TempDir(), "p6.bin")
	got := termText(t, s, map[string]any{"cmd": "write " + h + " file " + explicit + " --mode hex --data 414243"})
	if !strings.Contains(got, "bytes_written: 3") {
		t.Errorf("--mode hex stopped working:\n%s", got)
	}
}
