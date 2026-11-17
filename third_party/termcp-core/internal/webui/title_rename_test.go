package webui

import (
	"strings"
	"testing"
)

// TestTheWindowTitleIsTheRenameControl pins where a session is renamed from.
//
// The session plate's card renamed on a click of its name. The plate is gone, and
// the rename survived only as an item in the dock tab's RIGHT-CLICK menu: a
// gesture with no meaning on a phone (the viewport this page is used from most),
// and one nothing on the page announces. The window's title is where the name is
// read and it is on screen on every viewport, so that is where the control goes.
//
// The assertions are about reachability, not markup: a title that is not wired,
// or wired to a second implementation of the PATCH, would both pass a screenshot
// and fail a user.
func TestTheWindowTitleIsTheRenameControl(t *testing.T) {
	js := readAssetLF(t, "static/js/shell-windows.js")

	fn := between(t, js, "function bindWindowTitleRename(", "\n}\n")
	if fn == "" {
		t.Fatal("bindWindowTitleRename is gone — the title is not a rename control")
	}
	// The one implementation: the tab menu's, which already owns the prompt, the
	// empty-name guard, the PATCH and the optimistic local rename. A second copy
	// here would be a second set of rules for the same act.
	if !strings.Contains(fn, "renameSessionById(") {
		t.Error("the title must rename through renameSessionById, not its own PATCH")
	}
	// Resolved through the snapshot, not off the window: `_sid` is not always a
	// session id (a dial has none, a status card carries an extension's name), and
	// a PATCH against either 404s.
	if !strings.Contains(fn, "sessionForTabWindow(") {
		t.Error("the title must resolve its session through the snapshot")
	}
	if !strings.Contains(fn, "if (!s) return;") {
		t.Error("a window with no session behind it must offer no rename")
	}
	// The header drag handler starts on mousedown, and the header's dblclick
	// maximizes. A press on the name must reach neither, or renaming a window would
	// also drag it and a double tap would full-screen it.
	if !strings.Contains(fn, "addEventListener('mousedown'") {
		t.Error("the name must swallow mousedown, or pressing it drags the window")
	}
	// Keyboard: the name is a control, so Enter and Space have to work.
	if !strings.Contains(fn, "e.key !== 'Enter'") || !strings.Contains(fn, "' '") {
		t.Error("the name must be operable from the keyboard")
	}

	// It is called on the window's own init path, which both the connected and the
	// just-finalized-dial windows run through — wiring it in only one leaves the
	// other without the control.
	panels := readAssetLF(t, "static/js/terminal-panels.js")
	init := between(t, panels, "function _initShellWindowUI(", "\n}\n")
	if !strings.Contains(init, "bindWindowTitleRename(win)") {
		t.Error("_initShellWindowUI must wire the title, or no window gets the control")
	}
}

// TestTheTitleNameFollowsTheSessionNotTheLabel pins what the title shows.
//
// A window is opened from a label (`connLabel`), and that label stops being the
// session's name the moment anyone renames it — here, from a tab's menu, or from
// an agent over MCP. So the title reads the snapshot, and the repaint hangs off the
// session frame rather than off the click that made the change.
func TestTheTitleNameFollowsTheSessionNotTheLabel(t *testing.T) {
	js := readAssetLF(t, "static/js/shell-windows.js")
	fn := between(t, js, "function windowTitleName(", "\n}\n")
	if fn == "" {
		t.Fatal("windowTitleName is gone")
	}
	// A default name (`session-…`) IS the profile's, which the label already says —
	// the same rule sessionDockLabel applies, so the title and the tab cannot
	// disagree about what one session is called.
	if !strings.Contains(fn, "indexOf('session-')") {
		t.Error("a default name must fall back to the profile, as the dock's label does")
	}

	sess := readAssetLF(t, "static/js/sessions.js")
	frame := between(t, sess, "function applySessionsSnapshot(", "\n}\n")
	if frame == "" {
		t.Fatal("applySessionsSnapshot is gone")
	}
	if !strings.Contains(frame, "paintWindowTitleName(") {
		t.Error("the session frame must repaint open windows' titles, or a rename made elsewhere never reaches them")
	}
}

// TestTheRenameControlIsVisibleWithoutHover pins the affordance.
//
// The pencil may dim at rest, but it must not be hidden until hover: a control
// that only appears under a mouse pointer is exactly the context menu this
// replaces, and a phone has no pointer to hover with.
func TestTheRenameControlIsVisibleWithoutHover(t *testing.T) {
	css := readAssetLF(t, "static/css/terminal.css")
	for _, want := range []string{
		".sw-conn-pencil {",
		".sw-conn[role=\"button\"]:hover .sw-conn-pencil",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("terminal.css has lost %s", want)
		}
	}
	// The one failure mode worth pinning: `display: none` at rest with a hover rule
	// to reveal it. That reads as "the affordance is there" in a diff and as
	// nothing at all on a touch screen.
	rest := between(t, css, ".sw-conn-pencil {", "}")
	if rest == "" {
		t.Fatal(".sw-conn-pencil has no resting rule")
	}
	if strings.Contains(rest, "display: none") {
		t.Error("the pencil must not be hidden until hover — touch has no hover")
	}
}
