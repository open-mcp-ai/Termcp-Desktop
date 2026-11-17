package webui

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// browserProfileDir is the isolated Chrome profile directory for one probe.
//
// It is NOT t.TempDir(). Chrome runs crashpad as a separate process, and that
// helper keeps CrashpadMetrics-active.pma open for seconds after the browser it
// belongs to has exited. t.TempDir()'s cleanup runs the instant the test returns
// and then fails the TEST with `unlinkat … CrashpadMetrics-active.pma: Access is
// denied` — a flake whose message says nothing about what was being tested.
//
// The filename is Chrome's own (derived from the process type and its state) and
// is not configurable, so renaming it is not an option: the problem was never the
// name but the extra process holding a handle. --disable-crash-reporter,
// --disable-breakpad and --disable-features=Crashpad were all tried and none of
// them stops the file being created. So the fix is to stop racing it: retry the
// removal while the handle drains, and sweep leftovers from earlier runs at the
// start of the next one, which bounds what a run leaves behind to one directory
// instead of an ever-growing pile.
func browserProfileDir(t *testing.T) string {
	t.Helper()
	sweepStaleBrowserProfiles()
	dir, err := os.MkdirTemp("", "termcp-chrome-")
	if err != nil {
		t.Fatalf("creating the browser profile: %v", err)
	}
	t.Cleanup(func() {
		// Measured on Windows: usually gone at once, occasionally held for several
		// seconds. Give it a real window, then leave it for the sweep rather than
		// failing a passing test.
		deadline := time.Now().Add(15 * time.Second)
		for {
			if err := os.RemoveAll(dir); err == nil || time.Now().After(deadline) {
				return
			}
			time.Sleep(200 * time.Millisecond)
		}
	})
	return dir
}

// sweepStaleBrowserProfiles removes profile directories left by earlier runs.
//
// Only directories older than a minute are touched: a concurrent `go test` (the
// suite runs with -shuffle and -race, and packages run in parallel) may be using
// its own profile right now, and deleting that would break a passing test to
// tidy up after a finished one.
func sweepStaleBrowserProfiles() {
	root := os.TempDir()
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	cutoff := time.Now().Add(-time.Minute)
	for _, e := range entries {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "termcp-chrome-") {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		_ = os.RemoveAll(filepath.Join(root, e.Name()))
	}
}

// Chrome's virtual-time dump can advance timers without drawing another frame.
// Use its DevTools protocol with real time, and wait for the page's result. The
// isolated profile and browser lifetime belong to this probe on every OS.
func renderedProbeDOM(t *testing.T, chrome, path string) string {
	t.Helper()
	return renderedProbeDOMSized(t, chrome, path, 0, 0)
}

// renderedProbeDOMSized is renderedProbeDOM at a viewport the test chooses.
//
// THE SIZE IS PART OF THE QUESTION for anything about layout: the page has a
// desktop frame and a narrow stacked one, and which of the two is under test is
// decided by the width the browser happens to have. Left to the default window, a
// test about the desktop columns was measuring the phone layout — and would have
// passed or failed for reasons that had nothing to do with the code.
//
// A zero width leaves the browser's own window size alone.
func renderedProbeDOMSized(t *testing.T, chrome, path string, width, height int) string {
	t.Helper()
	// The budget covers launching Chrome, loading the page and rendering the
	// result. 30s was measured on a warm local machine and expired on a loaded CI
	// runner (the read deadline fired at Page.navigate before the probe had
	// produced anything), which is a slow machine, not a broken probe.
	//
	// 60s rather than more: the package has to fit inside the workflow's -timeout
	// 240s together with every other test here, so a probe that needs longer than
	// this would be a probe measuring the machine rather than the rendering.
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	profile := browserProfileDir(t)
	cmd := exec.CommandContext(ctx, chrome, "--headless=new", "--disable-gpu", "--no-sandbox",
		"--no-first-run", "--no-default-browser-check",
		// THE DEBUG PORT MUST NOT LISTEN ON EVERY INTERFACE. Without this flag
		// Chrome binds 0.0.0.0, and Windows asks the human to approve a firewall
		// rule for a port this test uses for a few seconds and then closes.
		"--remote-debugging-address=127.0.0.1", "--remote-debugging-port=0",
		"--user-data-dir="+profile, "about:blank")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	done := make(chan struct{})
	var waitErr error
	go func() {
		waitErr = cmd.Wait()
		close(done)
	}()
	var conn *websocket.Conn
	defer func() {
		if conn != nil {
			_ = conn.SetWriteDeadline(time.Now().Add(time.Second))
			_ = conn.WriteJSON(map[string]any{"id": 999, "method": "Browser.close"})
			_ = conn.Close()
		}
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			cancel()
			<-done
		}
	}()
	// The process is joined before returning, ahead of TempDir's profile cleanup.
	var endpoint string
	for endpoint == "" {
		data, err := os.ReadFile(filepath.Join(profile, "DevToolsActivePort"))
		lines := strings.Split(strings.TrimSpace(string(data)), "\n")
		if err == nil && len(lines) == 2 {
			endpoint = "ws://127.0.0.1:" + strings.TrimSpace(lines[0]) + strings.TrimSpace(lines[1])
			break
		}
		select {
		case <-done:
			t.Fatalf("headless browser exited before its debugger started: %v", waitErr)
		case <-ctx.Done():
			t.Fatal("headless browser debugger did not start")
		case <-time.After(20 * time.Millisecond):
		}
	}
	var err error
	conn, _, err = websocket.DefaultDialer.DialContext(ctx, endpoint, nil)
	if err != nil {
		t.Fatal(err)
	}
	deadline, _ := ctx.Deadline()
	_ = conn.SetReadDeadline(deadline)
	_ = conn.SetWriteDeadline(deadline)
	var id int
	var sessionID string
	loaded := make(map[string]bool)
	read := func() (int, json.RawMessage) {
		var reply struct {
			ID     int             `json:"id"`
			Method string          `json:"method"`
			Result json.RawMessage `json:"result"`
			Params struct {
				Name     string `json:"name"`
				LoaderID string `json:"loaderId"`
			} `json:"params"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if err := conn.ReadJSON(&reply); err != nil {
			t.Fatalf("headless browser protocol: %v", err)
		}
		if reply.Error != nil {
			t.Fatalf("headless browser protocol: %s", reply.Error.Message)
		}
		if reply.Method == "Page.lifecycleEvent" && reply.Params.Name == "load" {
			loaded[reply.Params.LoaderID] = true
		}
		return reply.ID, reply.Result
	}
	call := func(method string, params any) json.RawMessage {
		id++
		request := map[string]any{"id": id, "method": method, "params": params}
		if sessionID != "" {
			request["sessionId"] = sessionID
		}
		if err := conn.WriteJSON(request); err != nil {
			t.Fatal(err)
		}
		for {
			got, result := read()
			if got == id {
				return result
			}
		}
	}
	var target struct{ TargetID string }
	if err := json.Unmarshal(call("Target.createTarget", map[string]any{"url": "about:blank"}), &target); err != nil {
		t.Fatal(err)
	}
	var attachment struct{ SessionID string }
	if err := json.Unmarshal(call("Target.attachToTarget", map[string]any{"targetId": target.TargetID, "flatten": true}), &attachment); err != nil {
		t.Fatal(err)
	}
	sessionID = attachment.SessionID
	call("Page.enable", map[string]any{})
	if width > 0 && height > 0 {
		call("Emulation.setDeviceMetricsOverride", map[string]any{
			"width": width, "height": height, "deviceScaleFactor": 1, "mobile": false,
		})
	}
	call("Page.setLifecycleEventsEnabled", map[string]any{"enabled": true})
	var navigation struct {
		LoaderID  string `json:"loaderId"`
		ErrorText string `json:"errorText"`
	}
	if err := json.Unmarshal(call("Page.navigate", map[string]any{"url": "file:///" + filepath.ToSlash(path)}), &navigation); err != nil || navigation.ErrorText != "" {
		t.Fatalf("headless browser navigation failed: %s (%v)", navigation.ErrorText, err)
	}
	for !loaded[navigation.LoaderID] {
		read()
	}
	result := call("Runtime.evaluate", map[string]any{
		"expression": `new Promise(resolve => {
  function finish() {
    if (!document.title.startsWith('RESULT::')) return;
    observer.disconnect();
    resolve(document.documentElement.outerHTML);
  }
  const observer = new MutationObserver(finish);
  observer.observe(document.documentElement, {subtree: true, childList: true, characterData: true});
  finish();
})`,
		"awaitPromise": true, "returnByValue": true,
	})
	var dom struct {
		Result           struct{ Value string }
		ExceptionDetails json.RawMessage
	}
	if err := json.Unmarshal(result, &dom); err != nil || len(dom.ExceptionDetails) != 0 || dom.Result.Value == "" {
		t.Fatalf("headless browser probe failed: %s (%v)", result, err)
	}
	return dom.Result.Value
}
