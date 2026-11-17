package webui

import (
	"strings"
	"testing"
)

// A CONTAINER THAT CHANGED SIZE MUST REFIT THE GRID, NOT ONLY TELL THE PTY (issue #165).
//
// The observer on a channel's container called scheduleShellRemoteResize only: the
// remote side was told the new geometry while the LOCAL row count stayed where it was.
// The xterm element then stayed taller than the box around it, and the scrollbar
// belongs to the inner viewport — so it could be scrolled to ITS bottom while the last
// rows sat outside the visible area. That is the reported shape exactly: "the
// scrollbar is at the bottom but the content is not", with no scroll position that
// reaches the tail.
//
// MEASURED IN A REAL BROWSER, before this change: shrinking the terminal window to
// 300px left the container at 264px, the viewport's content at 408px and its client
// height at 228px, with rows still 26 — a 180px band of rows nothing could show. After
// it: rows 14, and the viewport's content equals its client height. Growing back gives
// rows 36.
//
// The assertion is on the SOURCE rather than on a browser, because this suite runs on
// three operating systems and only one of them has Chrome: a probe that skipped itself
// elsewhere would let the regression back in on two of three CI legs.
func TestAContainerResizeRefitsTheGrid(t *testing.T) {
	src := readAssetLF(t, "static/js/terminal-view.js")
	body := between(t, src, "function attachShellTerminalResizeObserverForChannel", "\n}\n")

	if !strings.Contains(body, "fitShellTerminal(") {
		t.Fatalf("the container's ResizeObserver does not refit the grid, so a shrunk "+
			"container leaves the terminal taller than its box and its last rows "+
			"unreachable:\n%s", body)
	}
	// syncRemote must stay true: the refit is what queues the PTY resize now, so
	// dropping it would fix the local grid and stop telling the remote side.
	if !strings.Contains(body, "fitShellTerminal(term, termEl, win, true)") {
		t.Fatalf("the refit does not sync the remote side, so the PTY keeps the old "+
			"geometry:\n%s", body)
	}
	// And the old call must not be the whole callback: posting a resize without
	// refitting is exactly the bug.
	if !strings.Contains(body, "ResizeObserver") {
		t.Fatalf("this is no longer a ResizeObserver callback:\n%s", body)
	}
}

// THE FIT IS COALESCED PER TERMINAL, NOT PER WINDOW.
//
// A drag emits a box per frame and term.resize is expensive, so the callback defers
// through requestAnimationFrame. The flag has to live on the TERM: on the window, two
// channels resizing in the same frame would cancel each other's fit and one of them
// would keep a stale grid — the same bug, reached by a different road.
func TestTheRefitIsCoalescedPerTerminal(t *testing.T) {
	src := readAssetLF(t, "static/js/terminal-view.js")
	body := between(t, src, "function attachShellTerminalResizeObserverForChannel", "\n}\n")

	if !strings.Contains(body, "term._termcpFitRaf") {
		t.Fatalf("the coalescing flag is not per-terminal:\n%s", body)
	}
	if strings.Contains(body, "win._termcpFitRaf") {
		t.Fatalf("the coalescing flag lives on the window, so two channels resizing in "+
			"one frame drop each other's fit:\n%s", body)
	}
}
