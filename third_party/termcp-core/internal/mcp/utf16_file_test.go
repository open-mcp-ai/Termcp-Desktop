package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A WINDOWS LOG IS UTF-16 UNTIL SOMETHING SAYS OTHERWISE.
//
// `read <session> file <path>` answered `\xff\xfeP\x00S\x00...` with `mode: text` for a
// file PowerShell's own redirection had written, and `--encoding` had no utf-16 to
// offer: the one flag that exists to name an encoding did not list the encoding the
// platform writes by default (issue #177). This is the end-to-end half of the fix —
// the table and the BOM sniffing are pinned in internal/encoding.
func TestAUTF16FileIsReadable(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	// A file verb addresses the SESSION, not a channel: the same session id with no
	// index is the file namespace of the tree.
	session := strings.SplitN(h, ":", 2)[0]

	// What `'PSModulePath' | Out-File -Encoding Unicode` writes.
	raw := []byte{0xFF, 0xFE}
	for _, r := range "PSModulePath" {
		raw = append(raw, byte(r), 0)
	}
	path := filepath.Join(t.TempDir(), "u16.txt")
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}

	got := termText(t, s, map[string]any{
		"cmd": "read " + session + " file " + filepath.ToSlash(path) + " --encoding utf-16",
	})
	if !strings.Contains(got, "PSModulePath") {
		t.Fatalf("a UTF-16 file is still unreadable:\n%s", firstLine(got))
	}
}
