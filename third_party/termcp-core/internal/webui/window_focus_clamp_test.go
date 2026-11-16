package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestFocusPullsAnOffscreenWindowBack runs the real focus path from util.js and
// the real clamp from shell-windows.js under node.
//
// The clamp existed but only ran on drag release, so anything else that moved
// the container out from under a window left it there. Shrinking the browser is
// the case that was reported: a window placed at 1400px sits entirely past the
// right edge at 900px, with no header left to grab, and only the tab bar can
// reach it. Clicking that tab is the reader asking for the window, so the focus
// path is where it has to come back.
//
// What must NOT happen is equally load-bearing. A hidden or minimized window
// measures 0x0 at 0,0, and clamping that rect would park it in the container's
// corner: it would come back from the tab bar somewhere the reader never left
// it. Full-screen and tiled windows own their geometry through the stylesheet,
// and an inline left/top written here would override it. A container measured
// mid-resize reports no area at all, and then nothing can be said about "inside".
func TestFocusPullsAnOffscreenWindowBack(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; cannot exercise the focus path")
	}
	util := readAssetLF(t, "static/js/util.js")
	shell := readAssetLF(t, "static/js/shell-windows.js")

	clamp := sliceFunc(t, shell, "function clampShellWindowIntoContainer(")
	headerBottom := sliceFunc(t, shell, "function appHeaderBottom(")

	script := `
const fs = require('fs');
const vm = require('vm');

const util = fs.readFileSync(process.argv[2], 'utf8');
const clamp = fs.readFileSync(process.argv[3], 'utf8');
const header = fs.readFileSync(process.argv[4], 'utf8');

// Slice the focus function out of util.js so the test runs the shipped code
// rather than a copy of its rule.
const fStart = util.indexOf('function bringShellWindowToFront(');
if (fStart < 0) { console.error('no bringShellWindowToFront in util.js'); process.exit(2); }
const fEnd = util.indexOf('\n}\n', fStart);
const focus = util.slice(fStart, fEnd + 2);

const sandbox = {};
vm.createContext(sandbox);

// Viewport and container are the two boxes the clamp measures. HEADER is the
// page header's lower edge, so the floor is a real number rather than 0.
vm.runInContext(
  'var window = { innerWidth: 900, innerHeight: 700 };\n' +
  'var HEADER = 56;\n' +
  'var document = { querySelector: function (sel) {\n' +
  '  if (sel !== ".app-header") return null;\n' +
  '  return { getBoundingClientRect: function () { return { bottom: HEADER }; } };\n' +
  '} };\n' +
  // The z-order half of the focus path, reduced to what it needs to not throw.
  'function shellZBand() { return { low: 100, high: 900 }; }\n' +
  'function allShellWins() { return []; }\n' +
  'function isTiledWin(w) { return !!w._tiled; }\n' +
  'function setActivePane() {}\n' +
  'function refreshSessionTabbar() {}\n' +
  'function fullscreenZ() { return 500; }\n' +
  header + '\n' + clamp + '\n' + focus, sandbox);

// HEADER lives on the sandbox (it is a var inside the vm context), so read it
// out rather than restating the number the test is asserting against.
const HEADER = sandbox.HEADER;
let bad = 0;

function win(opts) {
  opts = opts || {};
  const r = opts.rect || { left: 0, top: 0, width: 640, height: 480 };
  const w = {
    style: {},
    classList: {
      // Every path in the focus function starts by asking what this window is,
      // so the fake has to answer like a real one.
      _c: { 'shell-window': true },
      contains: function (n) { return !!this._c[n]; },
      add: function (n) { this._c[n] = true; },
      remove: function (n) { delete this._c[n]; },
    },
    parentNode: {
      clientWidth: opts.areaW === undefined ? 900 : opts.areaW,
      clientHeight: opts.areaH === undefined ? 644 : opts.areaH,
      getBoundingClientRect: function () {
        return { left: 0, top: HEADER, width: this.clientWidth, height: this.clientHeight };
      },
    },
    getBoundingClientRect: function () { return r; },
  };
  if (opts.fullscreen) w.classList.add('win-fullscreen');
  if (opts.tiled) w._tiled = true;
  return w;
}

function focusWin(w) {
  sandbox.W = w;
  vm.runInContext('bringShellWindowToFront(W)', sandbox);
}

// 1. Past the right edge at this width: 1400 was fine at 1600px wide.
let w = win({ rect: { left: 1400, top: 200, width: 640, height: 480 } });
focusWin(w);
const wantL = 900 - 640; // container right edge minus the window
if (parseFloat(w.style.left) !== wantL) {
  console.log('off-right: left = ' + w.style.left + ', want ' + wantL + 'px'); bad++;
}

// 2. Above the header floor, which is where a window released under it ends up.
w = win({ rect: { left: 100, top: 4, width: 640, height: 480 } });
focusWin(w);
if (parseFloat(w.style.top) !== HEADER) {
  console.log('above-floor: top = ' + w.style.top + ', want ' + HEADER + 'px'); bad++;
}

// 3. Already inside: untouched, so focus does not shuffle a window the reader
//    placed. The fractional values are deliberate — they must survive.
w = win({ rect: { left: 120.5, top: 200.25, width: 640, height: 480 } });
focusWin(w);
if (w.style.left !== undefined || w.style.top !== undefined) {
  console.log('inside: moved a window that was already fine (' + w.style.left + ', ' + w.style.top + ')'); bad++;
}

// 4. A hidden window measures 0x0 at 0,0. It must come back where it was, so
//    focus must not write any geometry at all.
w = win({ rect: { left: 0, top: 0, width: 0, height: 0 } });
w.classList.add('win-hidden');
focusWin(w);
if (w.style.left !== undefined || w.style.top !== undefined) {
  console.log('hidden: parked a hidden window at ' + w.style.left + ', ' + w.style.top); bad++;
}

// 5. A minimized window measures the same way.
w = win({ rect: { left: 0, top: 0, width: 0, height: 0 } });
w.classList.add('win-minimized');
focusWin(w);
if (w.style.left !== undefined || w.style.top !== undefined) {
  console.log('minimized: parked a minimized window'); bad++;
}

// 6. Full-screen: the stylesheet owns left/top, and applyWindowViewportMode
//    cleared the inline values precisely so nothing overrides it.
w = win({ fullscreen: true, rect: { left: 1400, top: 200, width: 640, height: 480 } });
focusWin(w);
if (w.style.left !== undefined || w.style.top !== undefined) {
  console.log('fullscreen: wrote inline geometry over the stylesheet'); bad++;
}

// 7. Tiled: the grid lays it out.
w = win({ tiled: true, rect: { left: 1400, top: 200, width: 640, height: 480 } });
focusWin(w);
if (w.style.left !== undefined || w.style.top !== undefined) {
  console.log('tiled: wrote inline geometry over the grid'); bad++;
}

// 8. A container with no area yet (measured mid-resize) says nothing about
//    where "inside" is, so nothing may move.
w = win({ areaW: 0, areaH: 0, rect: { left: 1400, top: 200, width: 640, height: 480 } });
focusWin(w);
if (w.style.left !== undefined) {
  console.log('no-area: clamped against a container that has no size'); bad++;
}

if (!bad) console.log('FOCUS CLAMP OK');
process.exit(bad ? 1 : 0);
`
	tmp := t.TempDir()
	writeFile := func(name, body string) string {
		p := filepath.Join(tmp, name)
		if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	out, err := exec.Command(node,
		writeFile("focus_test.js", script),
		writeFile("util.js", util),
		writeFile("clamp.js", clamp),
		writeFile("header.js", headerBottom),
	).CombinedOutput()
	got := string(out)
	if err != nil {
		t.Fatalf("focus did not keep windows on screen:\n%s", got)
	}
	if !strings.Contains(got, "FOCUS CLAMP OK") {
		t.Fatalf("harness did not run to completion:\n%s", got)
	}
}

// sliceFunc returns the body of the named top-level function, so a test can run
// the shipped implementation instead of a copy of its rule.
func sliceFunc(t *testing.T, body, sig string) string {
	t.Helper()
	start := strings.Index(body, sig)
	if start < 0 {
		t.Fatalf("the asset no longer defines %q", strings.TrimSuffix(sig, "("))
	}
	end := strings.Index(body[start:], "\n}\n")
	if end < 0 {
		t.Fatalf("could not delimit %q", strings.TrimSuffix(sig, "("))
	}
	return body[start : start+end+2]
}
