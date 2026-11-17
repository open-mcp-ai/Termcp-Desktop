package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestWritingIntoAMissingDirectoryNamesTheDirectory pins issue #147 end to end.
//
// The refusal was `open remote file: file does not exist` — the file being the one
// thing the caller was trying to CREATE. Read literally it says "the file you asked
// me to write does not exist", which is a loop, and it sends the caller looking for
// a path bug in a path that is perfectly well formed.
//
// The directory is what is missing, so the directory is what the message names.
// The status stays 404 (the path really is not there), which is why the original
// error has to stay in the chain rather than be replaced by the better sentence.
func TestWritingIntoAMissingDirectoryNamesTheDirectory(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "sleep 30")

	dir := filepath.Join(t.TempDir(), "nodir_zz")
	target := filepath.Join(dir, "p2.txt")
	got := termText(t, s, map[string]any{"cmd": "write " + h + " file " + target + " --data x"})

	if code := statusCode(t, got); code != 404 {
		t.Errorf("status = %d, want 404 (the path is not there):\n%s", code, got)
	}
	if !strings.Contains(got, "nodir_zz") {
		t.Errorf("the refusal does not name the directory that is missing:\n%s", got)
	}
	if !strings.Contains(got, "does not exist") {
		t.Errorf("the refusal does not say what is wrong:\n%s", got)
	}

	// And the directory it names has to be the one to create: making it turns the
	// same write into a success. Without this the message could name anything.
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	ok := termText(t, s, map[string]any{"cmd": "write " + h + " file " + target + " --data x"})
	if code := statusCode(t, ok); code >= 400 {
		t.Errorf("creating the named directory did not make the write work (%d):\n%s", code, ok)
	}
}
