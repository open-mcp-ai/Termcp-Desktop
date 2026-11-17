package webui

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// The terminal is a surface where the user does not choose the colours: a
// program does, and it is free to paint one ANSI colour on top of another. `ls`
// does exactly that for an other-writable directory — 34;42, blue text on a
// green background. Eight of the sixteen ANSI slots are this theme's semantic
// UI tokens, chosen so each reads against the *page*, which says nothing about
// how they read against each other: in default-light that cell was a contrast
// ratio of 1.00, i.e. the glyph and its background were the same colour, and the
// user filed it as "the terminal text cannot be read" (#114).
//
// xterm can repair such a cell itself by moving the text colour's lightness
// until it clears a ratio, but it only does that when asked: the option
// defaults to 1, "never". So the fix is not fifteen retuned colours (which
// would move the same problem to whichever pair is missed next) — it is passing
// the ratio through to xterm, from CSS through terminal-view.js.
//
// This test runs the real bundled xterm in a real browser, paints every ANSI
// foreground on every ANSI background, and requires every cell to be legible in
// all four built-in themes. The palette is read out of the browser's own
// cascade, so a theme that changes a colour cannot quietly skip the check.
func TestTerminalCellsStayLegibleOnEveryAnsiPairing(t *testing.T) {
	chrome := findChrome()
	if chrome == "" {
		t.Skip("no Chrome or Edge installed; cannot render the terminal")
	}
	xterm, err := readAsset("static/xterm/xterm.js")
	if err != nil {
		t.Skipf("the bundled xterm is not readable: %v", err)
	}
	// The slice below is the same code the page runs: every function from the
	// theme reader down, so the probe asks the real terminal-options builder
	// rather than restating what it thinks the builder does. A ratio that is
	// dropped on the way from CSS to the xterm options therefore fails here.
	view := readAssetLF(t, "static/js/terminal-view.js")
	start := strings.Index(view, "function termcpXtermTheme()")
	if start < 0 {
		t.Fatal("terminal-view.js no longer defines termcpXtermTheme; the terminal palette source is gone")
	}

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "xterm.js"), []byte(xterm), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "terminal-view.js"), []byte(view[start:]), 0o600); err != nil {
		t.Fatal(err)
	}
	shared := readAssetLF(t, "static/css/tokens.css")
	terminalCSS := readAssetLF(t, "static/css/terminal-theme.css")
	// The default lives in CSS, so the test asserts the declaration exists rather
	// than supplying a fallback of its own.
	if !regexp.MustCompile(`--terminal-minimum-contrast-ratio:\s*[\d.]+`).MatchString(terminalCSS) {
		t.Fatal("terminal-theme.css no longer declares --terminal-minimum-contrast-ratio")
	}

	themes := []string{"default-light", "default-dark", "cyberpunk", "matrix"}
	// Delay the real parser beyond the old 500ms snapshot. A busy runner can
	// delay it just as long; correctness must depend on rendering, not a timer.
	themes = append(themes, "delayed-write")
	for _, theme := range themes {
		page := probePage
		palette := theme
		if theme == "delayed-write" {
			palette = "cyberpunk"
			page = strings.Replace(page, "var options =", `
var originalWrite = Terminal.prototype.write;
Terminal.prototype.write = function(data, callback) {
  var term = this;
  setTimeout(function() { originalWrite.call(term, data, callback); }, 750);
};
var options =`, 1)
		}
		// Only the palette layers are needed: the shared tokens, the theme's
		// tokens, and the theme's own sheet (which is where matrix and cyberpunk
		// state their ANSI colours). Concatenating them in cascade order gives
		// the browser the same custom-property values the real page resolves,
		// without copying or importing the rest of the theme package.
		css := shared + "\n" + readAssetLF(t, "themes/"+palette+"/assets/static/css/tokens.css") +
			"\n" + readAssetLF(t, "themes/"+palette+"/theme.css") + "\n" + terminalCSS
		if err := os.WriteFile(filepath.Join(dir, "palette.css"), []byte(css), 0o600); err != nil {
			t.Fatal(err)
		}
		path := filepath.Join(dir, "probe.html")
		if err := os.WriteFile(path, []byte(page), 0o600); err != nil {
			t.Fatal(err)
		}
		out := renderedProbeDOM(t, chrome, path)
		checkAnsiGridLegible(t, theme, probeResult(t, theme, out))
	}
}

// findChrome returns a browser that supports --headless=new --dump-dom, or "".
func findChrome() string {
	for _, name := range []string{"chrome", "google-chrome", "chromium", "msedge"} {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	for _, p := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

type probeCell struct {
	Inline *string `json:"inline"`
}

type probeOut struct {
	Error string            `json:"error"`
	Ratio float64           `json:"ratio"`
	Theme map[string]string `json:"theme"`
	Rows  [][]probeCell     `json:"rows"`
}

func probeResult(t *testing.T, theme, dom string) probeOut {
	t.Helper()
	m := regexp.MustCompile(`(?s)<title>(.*?)</title>`).FindStringSubmatch(dom)
	if m == nil {
		t.Fatalf("%s: the probe page produced no title", theme)
	}
	title := strings.TrimSpace(m[1])
	if !strings.HasPrefix(title, "RESULT::") {
		t.Fatalf("%s: the probe did not finish: %q", theme, title)
	}
	var out probeOut
	if err := json.Unmarshal([]byte(strings.TrimPrefix(title, "RESULT::")), &out); err != nil {
		t.Fatalf("%s: unreadable probe result: %v", theme, err)
	}
	if out.Error != "" {
		t.Fatalf("%s: %s", theme, out.Error)
	}
	return out
}

var ansiNames = []string{"black", "red", "green", "yellow", "blue", "magenta", "cyan", "white",
	"brightBlack", "brightRed", "brightGreen", "brightYellow", "brightBlue", "brightMagenta", "brightCyan", "brightWhite"}

// legibleFloor is WCAG AA for normal text: the ratio the user needs, which is a
// property of the person reading and not something a theme gets to choose. It is
// deliberately not taken from the page — a theme that asks xterm for a ratio of
// 1 ("never repair") would otherwise satisfy an assertion that only checked the
// page's own request, which is the bug this test exists to catch.
const legibleFloor = 4.5

// checkAnsiGridLegible requires every cell of the 16x16 ANSI pairing grid to be
// readable. Row index is the foreground, column index the background. A cell
// xterm could leave alone keeps its fg class; one it had to repair carries an
// inline colour instead, and both are accepted — what matters is the colour that
// ends up on screen.
func checkAnsiGridLegible(t *testing.T, theme string, out probeOut) {
	t.Helper()
	if out.Ratio < legibleFloor {
		t.Fatalf("%s: the page asked xterm for a contrast ratio of %v, below the %.1f "+
			"needed to read normal text; a program painting one ANSI colour over another "+
			"is left unreadable", theme, out.Ratio, legibleFloor)
	}
	unreadable := unreadableAnsiCells(t, theme, out)
	if len(unreadable) > 0 {
		t.Errorf("%s: %d of %d ANSI pairings are below %.1f, so the terminal draws the "+
			"text in its own background colour:\n  %s",
			theme, len(unreadable), 16*16, legibleFloor, strings.Join(unreadable, "\n  "))
	}
}

// unreadableAnsiCells names every cell of the rendered grid whose text is too
// close to the colour behind it. It is separate from the assertion so that the
// grid check itself can be shown to detect a bad grid rather than passing
// vacuously.
func unreadableAnsiCells(t *testing.T, theme string, out probeOut) []string {
	t.Helper()
	if len(out.Rows) != 16 {
		t.Fatalf("%s: the probe rendered %d rows, want 16", theme, len(out.Rows))
	}
	// The palette is read from the browser, so a colour the theme does not set
	// is a failure here rather than an inherited default.
	palette := make([][3]int, 16)
	for i, name := range ansiNames {
		palette[i] = parseHexColor(t, theme, out.Theme[name])
	}
	var unreadable []string
	for f, row := range out.Rows {
		if len(row) != 16 {
			t.Fatalf("%s: row %d has %d cells, want 16", theme, f, len(row))
		}
		for b, cell := range row {
			text := palette[f]
			if cell.Inline != nil {
				text = parseHexColor(t, theme, *cell.Inline)
			}
			if ratio := contrastRatio(text, palette[b]); ratio+1e-9 < legibleFloor {
				unreadable = append(unreadable, fmt.Sprintf(
					"%s on %s: %.2f", ansiNames[f], ansiNames[b], ratio))
			}
		}
	}
	return unreadable
}

// contrastRatio is the WCAG relative-luminance ratio, the same formula xterm
// uses to decide whether a cell needs repairing.
func contrastRatio(a, b [3]int) float64 {
	la, lb := relativeLuminance(a), relativeLuminance(b)
	if la < lb {
		la, lb = lb, la
	}
	return (la + 0.05) / (lb + 0.05)
}

func relativeLuminance(c [3]int) float64 {
	channel := func(v int) float64 {
		x := float64(v) / 255
		if x <= 0.03928 {
			return x / 12.92
		}
		return math.Pow((x+0.055)/1.055, 2.4)
	}
	return 0.2126*channel(c[0]) + 0.7152*channel(c[1]) + 0.0722*channel(c[2])
}

// parseHexColor reads a colour the browser resolved. Custom properties come
// back as `#rrggbb` when the theme wrote a literal and as `rgb(r, g, b)` when it
// wrote `rgb(var(--x-rgb))`, so both are the browser's own answer and both are
// accepted; anything else means the token did not resolve.
func parseHexColor(t *testing.T, theme, value string) [3]int {
	t.Helper()
	value = strings.TrimSpace(value)
	if rgb := regexp.MustCompile(`^rgba?\(\s*(\d+)[\s,]+(\d+)[\s,]+(\d+)`).FindStringSubmatch(value); rgb != nil {
		var out [3]int
		for i := 0; i < 3; i++ {
			v, err := strconv.Atoi(rgb[i+1])
			if err != nil || v < 0 || v > 255 {
				t.Fatalf("%s: %q is not a resolvable colour", theme, value)
			}
			out[i] = v
		}
		return out
	}
	if !strings.HasPrefix(value, "#") || len(value) < 7 {
		t.Fatalf("%s: %q is not a resolvable colour", theme, value)
	}
	var out [3]int
	for i := 0; i < 3; i++ {
		v, err := strconv.ParseUint(value[1+i*2:3+i*2], 16, 8)
		if err != nil {
			t.Fatalf("%s: %q is not a resolvable colour: %v", theme, value, err)
		}
		out[i] = int(v)
	}
	return out
}

const probePage = `<!doctype html><meta charset="utf-8"><title>waiting</title>
<link rel="stylesheet" href="palette.css">
<div id="t"></div>
<script src="xterm.js"></script>
<script>
/* terminal-view.js is a page script, not a module: it ends with an
   onThemeChange() call and refers to a few globals the rest of the UI owns.
   Only the theme-reading half is under test here, so the stubs below are the
   minimum that half touches. */
var _themeRevision = 0;
var _themeTerminal = { transparent: false };
function onThemeChange() {}
function allShellWins() { return []; }
function termcpMonoFontFamily() { return 'monospace'; }
</script>
<script src="terminal-view.js"></script>
<script>
/* Ask the real builder for the xterm options, then render the full ANSI
   foreground x ANSI background grid through the real xterm. */
var options = termcpTerminalOptions();
if (!isFinite(options.minimumContrastRatio) || options.minimumContrastRatio < 1) {
  document.title = 'RESULT::' + JSON.stringify({error: 'termcpTerminalOptions() returned minimumContrastRatio ' + JSON.stringify(options.minimumContrastRatio)});
} else {
  var NAMES = ['black','red','green','yellow','blue','magenta','cyan','white',
               'brightBlack','brightRed','brightGreen','brightYellow','brightBlue','brightMagenta','brightCyan','brightWhite'];
  var probe = getComputedStyle(document.documentElement);
  var theme = { foreground: probe.getPropertyValue('--xterm-foreground').trim(),
                background: probe.getPropertyValue('--xterm-background').trim() };
  NAMES.forEach(function (n) {
    var prop = '--xterm-' + n.replace(/[A-Z]/g, function (c) { return '-' + c.toLowerCase(); });
    theme[n] = probe.getPropertyValue(prop).trim();
  });
  var t = new Terminal({minimumContrastRatio: options.minimumContrastRatio, theme: theme});
  t.open(document.getElementById('t'));
  var lines = [];
  for (var f = 0; f < 16; f++) {
    var row = '';
    for (var b = 0; b < 16; b++) {
      var fg = f < 8 ? (30 + f) : (90 + f - 8);
      var bg = b < 8 ? (40 + b) : (100 + b - 8);
      row += '\u001b[' + fg + ';' + bg + 'mX';
    }
    lines.push(row + '\u001b[0m');
  }
  // Parsing and DOM rendering are asynchronous. A fixed timer can observe the
  // initial empty rows on a busy host, even if it fires after write() returns.
  var parsed = false;
  var rendered = t.onRender(function () {
    if (!parsed) return;
    var rows = document.getElementById('t').querySelectorAll('.xterm-rows > div');
    var grid = [];
    [].forEach.call(rows, function (r, fi) {
      if (fi >= 16) return;
      var cells = [];
      [].forEach.call(r.children, function (sp) {
        var m = /color:\s*([^;]+)/.exec(sp.getAttribute('style') || '');
        cells.push({inline: m ? m[1].trim() : null});
      });
      grid.push(cells);
    });
    document.title = 'RESULT::' + JSON.stringify({ratio: options.minimumContrastRatio, theme: theme, rows: grid});
    rendered.dispose();
  });
  t.write(lines.join('\r\n') + '\r\n', function () {
    parsed = true;
    t.refresh(0, 15);
  });
}
</script>
`

// TestAnsiGridCheckCatchesAnUnreadableCell pins the grid check itself: it is the
// part that decides whether a rendered terminal is legible, so a vacuous version
// of it would let the browser test above pass on any palette. The grid below is
// the real failure from #114 — blue text on a green background, the two colours
// the light theme uses for that pairing.
func TestAnsiGridCheckCatchesAnUnreadableCell(t *testing.T) {
	// The light theme's blue and green ANSI slots, which is exactly the pair
	// `ls` puts together for an other-writable directory (34;42).
	const blue, green = "#4470b2", "#3e7c57"
	if got := contrastRatio(parseHexColor(t, "fixture", blue), parseHexColor(t, "fixture", green)); got > 1.1 {
		t.Fatalf("the fixture pair is no longer the reported one: contrast %.2f", got)
	}
	// A palette of one mid grey: on its own every cell of the grid is as
	// unreadable as every other, which is what a real palette looks like before
	// xterm repairs it.
	theme := map[string]string{"background": "#ffffff", "foreground": "#292e36"}
	for _, name := range ansiNames {
		theme[name] = "#808080"
	}
	theme[ansiNames[4]], theme[ansiNames[2]] = blue, green

	// Stand in for xterm's repair: paint each cell in whichever of black or
	// white reads better on the colour behind it. One of the two always clears
	// 4.5, so this grid is legible by construction.
	rows := make([][]probeCell, 16)
	for f := range rows {
		rows[f] = make([]probeCell, 16)
		for b := range rows[f] {
			rows[f][b] = probeCell{Inline: ptr(bestReadable(parseHexColor(t, "fixture", theme[ansiNames[b]])))}
		}
	}
	if bad := unreadableAnsiCells(t, "legible", probeOut{Theme: theme, Rows: rows}); len(bad) != 0 {
		t.Fatalf("the check flagged a legible grid: %v", bad)
	}

	// Leave one cell unrepaired and it must be named. This is the exact cell
	// `ls` produces for an other-writable directory.
	rows[4][2] = probeCell{}
	bad := unreadableAnsiCells(t, "reported", probeOut{Theme: theme, Rows: rows})
	if len(bad) != 1 || !strings.Contains(bad[0], "blue on green") {
		t.Fatalf("the check missed the reported cell; got %v", bad)
	}
}

// bestReadable returns the colour xterm would shift a cell towards: black or
// white, whichever is further from the background.
func bestReadable(bg [3]int) string {
	if contrastRatio([3]int{255, 255, 255}, bg) >= contrastRatio([3]int{0, 0, 0}, bg) {
		return "#ffffff"
	}
	return "#000000"
}

func ptr(s string) *string { return &s }
