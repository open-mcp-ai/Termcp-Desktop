// Command termdoc rewrites the generated blocks in docs/api.md from the code that
// defines them.
//
// It exists because a hand-kept copy of a table drifts, and it already did: the
// `structured_only` argument shipped in the tool schema and was missing from the
// parameter table in docs/api.md, so a caller reading the REST reference could not
// discover the switch that halves a large reply. The fix is not care, it is
// generation — the doc now has exactly one way to be wrong, and that way is
// forgetting to run this command (which the test catches).
//
// Usage:
//
//	go run ./cmd/termdoc          rewrite docs/api.md in place
//	go run ./cmd/termdoc -check   exit 1 if it is stale (used by the tests)
//
// The two copies of api.md (docs/ and internal/webui/assets/) are kept in step by
// `make sync-assets`, not by this command: this one owns the content of the
// generated block, sync-assets owns where the file lives.
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/open-mcp-ai/termcp/internal/mcp"
)

const docPath = "docs/api.md"

func main() {
	check := flag.Bool("check", false, "report whether docs are stale instead of rewriting them")
	flag.Parse()

	src, err := os.ReadFile(docPath)
	if err != nil {
		fail("read %s: %v", docPath, err)
	}
	updated, changed := mcp.RenderTermDoc(string(src))
	if !changed {
		// Nothing to rewrite. Reported rather than silent so a caller can tell
		// "already current" from "the markers moved and I found nothing to do".
		fmt.Println("docs are current")
		return
	}
	if *check {
		fail("%s is stale: run `go run ./cmd/termdoc`", docPath)
	}
	if err := os.WriteFile(docPath, []byte(updated), 0o644); err != nil {
		fail("write %s: %v", docPath, err)
	}
	fmt.Println("rewrote", docPath)
}

// fail prints a diagnostic and exits non-zero. Wrapped because main has no
// business returning the same two lines at every exit point.
func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "termdoc: "+format+"\n", args...)
	os.Exit(1)
}
