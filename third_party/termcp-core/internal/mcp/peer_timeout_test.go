package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// TestPeerCallTimesOutWith504AndNamesTheRemedy pins item 2 of #158.
//
// A peer that never answers used to hang until the HOST's watchdog aborted the
// turn — a 60s wait that ended in a generic `AbortSignal` with no hint who was to
// blame. The call now enforces its own deadline (default 15s, or the caller's
// --timeout), and answers 504 `peer_timeout` pointing at `--send` or `--timeout`.
func TestPeerCallTimesOutWith504AndNamesTheRemedy(t *testing.T) {
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://` + r.Host + `/devtools/browser/x"}`))
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			var frame map[string]any
			if err := conn.ReadJSON(&frame); err != nil {
				return
			}
			// NEVER ANSWER: simulate a hung method
		}
	}))
	defer srv.Close()

	s := newTestServer(t)
	internalSession(t, s)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	start := time.Now()
	// Ask for a short window so the test is quick; default is 15s.
	got := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Page.navigate"}`, "timeout": 0.2})
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Fatalf("call waited %v, over the 200ms window", elapsed)
	}
	if !strings.HasPrefix(got, "TERM/1 504") {
		t.Fatalf("expected 504 on hung peer, got:\n%s", got)
	}
	if !strings.Contains(got, "peer_timeout") {
		t.Errorf("expected Error-Code: peer_timeout, got:\n%s", got)
	}
	if !strings.Contains(got, "--send") {
		t.Errorf("refusal does not mention --send as an alternative:\n%s", got)
	}
}
