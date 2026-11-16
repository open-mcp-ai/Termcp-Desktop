package mcp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const cjkTitle = "中文网页标题测试"

func targetIDFromTargets(t *testing.T, payload string) string {
	t.Helper()
	var doc struct {
		Result struct {
			TargetID string `json:"targetId"`
		} `json:"result"`
	}
	for _, line := range strings.Split(payload, "\n") {
		if strings.HasPrefix(line, "{") {
			if err := json.Unmarshal([]byte(line), &doc); err == nil && doc.Result.TargetID != "" {
				return doc.Result.TargetID
			}
		}
	}
	t.Fatalf("no targetId in:\n%s", payload)
	return ""
}

func latestFrom(t *testing.T, text string) string {
	t.Helper()
	if v, ok := headerValue(t, text, "Latest"); ok {
		return v
	}
	t.Fatalf("no Latest header in:\n%s", text)
	return ""
}

func findChrome(t *testing.T) string {
	for _, n := range []string{"chrome", "google-chrome", "chromium", "msedge"} {
		if p, err := exec.LookPath(n); err == nil {
			return p
		}
	}
	for _, p := range []string{
		`C:\Program Files\Google\Chrome\Application\chrome.exe`,
		`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
	} {
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return ""
}

// TestRealBrowserThroughTheCdpDoor is the check a fake peer cannot be: a real
// browser, reached through the real door, answering the calls a page probe
// actually starts with. The fakes pin the protocol's shape; this pins that the
// shape is Chrome's.
func TestRealBrowserThroughTheCdpDoor(t *testing.T) {
	chrome := findChrome(t)
	if chrome == "" {
		t.Skip("no Chrome or Edge installed")
	}
	// Named, because "never wrote DevToolsActivePort" on a runner with both Chrome
	// and Edge installed does not say which one was asked.
	t.Logf("driving %s", chrome)
	// NOT t.TempDir(): Chrome's crashpad helper holds CrashpadMetrics-active.pma
	// for seconds after the browser exits, and TempDir's cleanup runs the instant
	// the test returns, so it fails the TEST with an "Access is denied" unlink
	// that says nothing about what was being tested. MkdirTemp plus a tolerant
	// cleanup is the same treatment internal/webui's browser probe uses.
	profile, err := os.MkdirTemp("", "termcp-cdp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for deadline := time.Now().Add(15 * time.Second); ; {
			if err := os.RemoveAll(profile); err == nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
	cmd := exec.Command(chrome, "--headless=new", "--disable-gpu", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check",
		"--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0",
		"--user-data-dir="+profile, "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	// WAIT FOR THE PROCESS AS WELL AS FOR ITS FILE, and report which happened.
	//
	// The loop below watches for DevToolsActivePort; on a loaded windows runner it
	// reported "browser never wrote DevToolsActivePort" after 30s, which is true and
	// useless -- the browser may have exited instantly with a message, or still be
	// starting. cmd.Wait in a goroutine separates those two: an early exit is a
	// broken launch and its error is the answer, while a live process that is merely
	// slow gets the whole budget. internal/webui's browser probe learned this the
	// same way and is the shape copied here.
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	defer func() {
		_ = cmd.Process.Kill()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
		}
	}()

	// DevToolsActivePort: line 1 is the port the browser chose, which is what
	// makes --remote-debugging-port=0 usable without guessing a free one.
	//
	// 60s, not 30s: launching a browser on a cold runner is the slowest thing in this
	// package, and the same 30s was measured on a warm machine before it expired on
	// CI in internal/webui (see renderedProbeDOM). Not more, because the package has
	// to fit the workflow's -timeout 240s alongside every other test here.
	var port string
	deadline := time.Now().Add(60 * time.Second)
	for port == "" {
		if data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			if lines := strings.Split(strings.TrimSpace(string(data)), "\n"); len(lines) == 2 {
				port = strings.TrimSpace(lines[0])
				break
			}
		}
		select {
		case <-done:
			t.Fatalf("browser exited before its debugger started: %v", waitErr)
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			t.Fatalf("browser never wrote DevToolsActivePort in 60s (still running: %v)", waitErr == nil)
		}
	}

	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp http://127.0.0.1:" + port}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// THE PEER IS VISIBLE IN THE LISTING. `ls <session>` is where a caller finds what
	// an instance holds, so a peer named only in its own open reply is a name you lose.
	// The address printed there must be the one the other verbs accept.
	if listing := termText(t, s, map[string]any{"cmd": "ls internal"}); !strings.Contains(listing, h) {
		t.Fatalf("ls <session> does not name the open peer %q:\n%s", h, listing)
	}
	// TWO BROWSERS ON ONE MACHINE ARE TWO CONNECTIONS. The index is what makes that
	// representable at all: a second open must not collide with the first, and both
	// must appear in a listing with the address the other verbs accept.
	second := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp http://127.0.0.1:" + port}))
	if second == h {
		t.Fatalf("the second open reused the first address %q", h)
	}
	if both := termText(t, s, map[string]any{"cmd": "ls internal cdp"}); !strings.Contains(both, h) || !strings.Contains(both, second) {
		t.Fatalf("ls does not name both connections (%q, %q):\n%s", h, second, both)
	}
	if got := termText(t, s, map[string]any{"cmd": "close " + second}); !statusLineIs(got, "204") {
		t.Fatalf("closing the second connection failed:\n%s", got)
	}

	targets := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Target.getTargets"}`})
	if !strings.Contains(targets, "targetInfos") {
		t.Fatalf("Target.getTargets failed:\n%s", targets)
	}

	created := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Target.createTarget","params":{"url":"about:blank"}}`})
	if !strings.Contains(created, "targetId") {
		t.Fatalf("Target.createTarget failed:\n%s", created)
	}

	// ---- the door's own surface, against the real browser ----

	// `ls <peer>` lists tabs without being asked for a frame.
	list := termText(t, s, map[string]any{"cmd": "ls " + h})
	if !strings.Contains(list, "target(s)") || !strings.Contains(list, "open "+h+" --target") {
		t.Fatalf("ls did not list the tabs and the next step:\n%s", list)
	}

	targetID := targetIDFromTargets(t, created)
	attached := termText(t, s, map[string]any{"cmd": "open " + h + " --target " + targetID})
	child := handleFrom(t, attached)
	if !strings.Contains(attached, "Page.enable") {
		t.Fatalf("the attach reply did not say what to do next:\n%s", attached)
	}
	if !strings.HasPrefix(child, h+"@") {
		t.Fatalf("the page address %q does not hang off %q", child, h)
	}

	// Enable a domain, then evaluate WITHOUT writing a sessionId: the child handle
	// carries it, which is the whole point.
	if got := termText(t, s, map[string]any{"cmd": `call ` + child + ` {"method":"Page.enable"}`}); !statusLineIs(got, "200") {
		t.Fatalf("Page.enable through the child handle failed:\n%s", got)
	}

	// NON-ASCII COMES BACK AS CHARACTERS, not as the peer's \u escapes.
	evalCmd := `call ` + child + ` {"method":"Runtime.evaluate","params":{"expression":"document.title='` + cjkTitle + `'; document.title","returnByValue":true}}`
	evaluated := termText(t, s, map[string]any{"cmd": evalCmd})
	if strings.Contains(evaluated, `\u`) {
		t.Fatalf("the reply still carries the peer's unicode escapes:\n%s", evaluated)
	}
	if !strings.Contains(evaluated, cjkTitle) {
		t.Fatalf("the evaluated title did not come back readable:\n%s", evaluated)
	}

	// A PAGE-LEVEL METHOD SENT TO THE BROWSER PROCESS IS AN ERROR, NOT A SILENT NO-OP.
	wrongScope := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Page.enable"}`})
	if code := statusCode(t, wrongScope); code != 400 {
		t.Fatalf("a page-level method at browser scope was reported as success:\n%s", wrongScope)
	}

	// --since reads the frame history WITHOUT consuming it, twice in a row.
	before := latestFrom(t, termText(t, s, map[string]any{"cmd": "get " + h}))
	termText(t, s, map[string]any{"cmd": `call ` + child + ` {"method":"Page.navigate","params":{"url":"about:blank"}}`})
	// Events arrive on their own schedule; the window is what makes this not a sleep.
	var events string
	for deadline := time.Now().Add(10 * time.Second); time.Now().Before(deadline); {
		events = termText(t, s, map[string]any{"cmd": "read " + h + " --since " + before})
		if strings.Contains(events, "Page.loadEventFired") || strings.Contains(events, "Page.frameNavigated") {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !strings.Contains(events, "Page.") {
		t.Fatalf("--since never saw the page events:\n%s", events)
	}
	if again := termText(t, s, map[string]any{"cmd": "read " + h + " --since " + before}); again != events && !strings.Contains(again, "Page.") {
		t.Fatalf("--since consumed the frames it returned:\n%s", again)
	}
	if after := termText(t, s, map[string]any{"cmd": "read " + h + " --since " + before}); !strings.Contains(after, "Page.") {
		t.Fatalf("a second --since from the same cursor came back empty:\n%s", after)
	}

	// A PAGE ADDRESS NARROWS THE SAME WINDOW. One socket carries every attached page's
	// frames interleaved, tagged with their sessionId — so a page-scoped read must show
	// that page's frames and no other's. A browser-level answer carries no tag at all,
	// which makes it the negative case: it must appear unscoped and not in the page.
	scopedBefore := latestFrom(t, termText(t, s, map[string]any{"cmd": "get " + h}))
	termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Target.getTargets"}`})
	if scoped := termText(t, s, map[string]any{"cmd": "read " + child + " --since " + scopedBefore}); strings.Contains(scoped, "targetInfos") {
		t.Fatalf("a page-scoped read showed a browser-level answer:\n%s", scoped)
	}
	if wide := termText(t, s, map[string]any{"cmd": "read " + h + " --since " + scopedBefore}); !strings.Contains(wide, "targetInfos") {
		t.Fatalf("the unscoped read lost the browser-level answer:\n%s", wide)
	}

	// The live protocol document, read from the browser rather than from a table.
	protocol := termText(t, s, map[string]any{"cmd": "get " + h + " protocol"})
	if !strings.Contains(protocol, "Page") || !strings.Contains(protocol, "domains") {
		t.Fatalf("get protocol did not list the domains:\n%s", protocol)
	}
	pageDomain := termText(t, s, map[string]any{"cmd": "get " + h + " protocol Page"})
	if !strings.Contains(pageDomain, "navigate") || !strings.Contains(pageDomain, "captureScreenshot") {
		t.Fatalf("get protocol Page did not show Page's commands:\n%s", pageDomain)
	}

	// The ONE-SHOT form: attach, call, detach, with no handle to keep. It must
	// work on a target that has no session of its own.
	oneShot := termText(t, s, map[string]any{"cmd": `call ` + h + `@` + targetID + ` {"method":"Page.reload"}`})
	if code := statusCode(t, oneShot); code >= 400 {
		t.Fatalf("the transient form failed:\n%s", oneShot)
	}

	// `close <peer>:N` detaches; the tab stays.
	if got := termText(t, s, map[string]any{"cmd": "close " + child}); !statusLineIs(got, "204") {
		t.Fatalf("detach through the child handle failed:\n%s", got)
	}
	if after := termText(t, s, map[string]any{"cmd": "get " + h}); strings.Contains(after, child) {
		t.Fatalf("a detached session is still listed:\n%s", after)
	}
}

// startHeadlessChrome launches a throwaway browser and returns its devtools port.
func startHeadlessChrome(t *testing.T) string {
	t.Helper()
	chrome := findChrome(t)
	if chrome == "" {
		t.Skip("no Chrome or Edge installed")
	}
	// Named, because "never wrote DevToolsActivePort" on a runner with both Chrome
	// and Edge installed does not say which one was asked.
	t.Logf("driving %s", chrome)
	// NOT t.TempDir(): Chrome holds CrashpadMetrics-active.pma for seconds after it
	// exits, and TempDir's cleanup runs the instant the test returns.
	profile, err := os.MkdirTemp("", "termcp-cdp-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for deadline := time.Now().Add(15 * time.Second); ; {
			if err := os.RemoveAll(profile); err == nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
	cmd := exec.Command(chrome, "--headless=new", "--disable-gpu", "--no-sandbox",
		"--no-first-run", "--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0",
		"--user-data-dir="+profile, "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	for deadline := time.Now().Add(30 * time.Second); time.Now().Before(deadline); {
		if data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort")); err == nil {
			lines := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(lines) == 2 {
				return strings.TrimSpace(lines[0])
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("browser never wrote DevToolsActivePort")
	return ""
}

// TestLargeBundleInjectsThroughAUrl is how a big script gets into a page.
//
// `call` carries its frame on the command line, so a 200 KB bundle would have to be
// emitted by the caller — the one thing that does not scale. The browser fetches it
// instead: serve the file from the session's own network, then append a <script>
// element pointing at the URL. Only the URL crosses the tool, and the page ends up
// with the code.
//
// No verb is involved that does not already exist: the "server" is what `open
// <session> --argv` starts, and the injection is an ordinary Runtime.evaluate. Raw
// CDP has no addScriptTag — that is Puppeteer's helper — which is why the frame
// builds the element itself.
func TestLargeBundleInjectsThroughAUrl(t *testing.T) {
	port := startHeadlessChrome(t)
	const bundleBytes = 200 * 1024
	// A block comment so the padding cannot swallow the statement after it.
	bundle := strings.Repeat("/* filler for size */", 8000) +
		"window.__injected = " + fmt.Sprint(bundleBytes) + ";"

	var hits int32
	static := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&hits, 1)
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(bundle))
	}))
	defer static.Close()

	s := newTestServer(t)
	internalSession(t, s)
	conn := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp http://127.0.0.1:" + port}))
	defer termText(t, s, map[string]any{"cmd": "close " + conn})
	created := termText(t, s, map[string]any{"cmd": `call ` + conn + ` {"method":"Target.createTarget","params":{"url":"about:blank"}}`})
	page := handleFrom(t, termText(t, s, map[string]any{"cmd": "open " + conn + " --target " + targetIDFromTargets(t, created)}))

	loader := "var s=document.createElement('script');s.src='" + static.URL +
		"/app.js';document.head.appendChild(s)"
	inject := termText(t, s, map[string]any{"cmd": `call ` + page +
		` {"method":"Runtime.evaluate","params":{"expression":"` + loader + `"}}`})
	if code := statusCode(t, inject); code >= 400 {
		t.Fatalf("the loader was refused:\n%s", inject)
	}
	var injected string
	for deadline := time.Now().Add(15 * time.Second); time.Now().Before(deadline); {
		injected = termText(t, s, map[string]any{"cmd": `call ` + page +
			` {"method":"Runtime.evaluate","params":{"expression":"String(window.__injected)","returnByValue":true}}`})
		if strings.Contains(injected, fmt.Sprint(bundleBytes)) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if !strings.Contains(injected, fmt.Sprint(bundleBytes)) {
		t.Fatalf("a 200 KB bundle served from the page's own network never ran (server hits=%d):\n%s",
			atomic.LoadInt32(&hits), injected)
	}
}
