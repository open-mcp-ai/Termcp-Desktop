package mcp

import (
	"fmt"
	"strings"
	"testing"
)

// A generated reply cannot be paged — it has no cursor — so the one thing a caller
// can be told is how big it would have been. That has to hold on the reply a caller
// actually reads: an earlier version of this fix patched the helper's caller instead
// of the helper, which left eight of the nine places that render a generated reply
// untouched — including the help page, the very reply the issue was filed about.
//
// So these drive `help` through the term entry and read the rendered headers, not the
// slot a helper returns.
func TestGeneratedReplyReportsItsFullSize(t *testing.T) {
	s := newTestServer(t)

	// BOTH WINDOWS ARE RAISED: the default lines_limit is 200, and `read`'s page has
	// grown past it. The subject here is the size report, so the reply under test has to
	// be the whole reply — with the default line window this is a CUT reply, and the
	// body it delivers is not the number the header promises.
	full := termText(t, s, map[string]any{"cmd": "help read --all", "bytes_limit": 32768, "lines_limit": 100000})
	if got, ok := headerValue(t, full, "Content-Range"); ok {
		t.Errorf("a reply delivered whole carries no range, got %q", got)
	}

	// The body starts after the header block's blank line.
	sep := strings.Index(full, "\n\n")
	if sep < 0 {
		t.Fatal("no blank line between headers and body")
	}
	want := fmt.Sprintf("bytes */%d", len(full)-(sep+2))

	// The documented cheap form: "the channel only, no bytes". Its whole purpose is
	// to let a caller look before spending, so it has to answer with the size.
	zero := termText(t, s, map[string]any{"cmd": "help read --all", "bytes_limit": 0})
	gotZero, ok := headerValue(t, zero, "Content-Range")
	if !ok {
		t.Fatal("bytes_limit 0 must report the size; it is the only form that can ask for free")
	}
	if gotZero != want {
		t.Errorf("zero window: Content-Range = %q, want %q", gotZero, want)
	}

	// A window that cuts says the same number: the size is a property of the reply,
	// not of the window that sampled it.
	cut := termText(t, s, map[string]any{"cmd": "help read --all", "bytes_limit": 2000})
	gotCut, ok := headerValue(t, cut, "Content-Range")
	if !ok {
		t.Fatal("a cut generated reply must report its full size")
	}
	if gotCut != want {
		t.Errorf("cut window: Content-Range = %q, want %q", gotCut, want)
	}
	if _, ok := headerValue(t, cut, "Truncated"); !ok {
		t.Error("a cut generated reply must still say what was withheld")
	}
}

// The same has to be true of a listing, not only of the document that was reported.
func TestGeneratedListingReportsItsFullSize(t *testing.T) {
	s := newTestServer(t)

	full := termText(t, s, map[string]any{"cmd": "help", "bytes_limit": 32768})
	sep := strings.Index(full, "\n\n")
	if sep < 0 {
		t.Fatal("no blank line between headers and body")
	}
	want := fmt.Sprintf("bytes */%d", len(full)-(sep+2))

	zero := termText(t, s, map[string]any{"cmd": "help", "bytes_limit": 0})
	if got, _ := headerValue(t, zero, "Content-Range"); got != want {
		t.Errorf("Content-Range = %q, want %q", got, want)
	}
}
