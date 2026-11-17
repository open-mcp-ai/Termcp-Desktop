package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"

	"github.com/open-mcp-ai/termcp/pkg/api"
)

// TestRefuseDoorChecksTargetSessionNotEverySession pins issue #159 item B.
//
// The door used to walk every session in the caller's namespace and refuse as soon
// as ANY of them lacked the capability — so one restricted session anywhere in the
// instance (an extension template defaults to shell-only) refused `open ... mcp`
// and `open ... cdp` for everybody, and granting `--allow cdp.open` on the session
// actually being used changed nothing. The door is opened THROUGH a session; that
// session's template is whose whitelist applies.
func TestRefuseDoorChecksTargetSessionNotEverySession(t *testing.T) {
	s := newTestServer(t)

	// A restricted session exists and is NOT the one being opened through. This is
	// the whole point of the test: its presence must not matter.
	locked := guardTestSession(t, s, []string{"shell_input", "shell_output"}, nil)

	plain := createTestSession(t, s, api.ModePipe)

	srv := devtoolsStub(t)

	// Opening through the unrestricted session SUCCEEDS despite the restricted
	// sibling. Before the fix this was 403 "does not grant cdp.open".
	ok := termText(t, s, map[string]any{"cmd": "open " + plain + " cdp " + srv})
	if strings.Contains(ok, "does not grant") {
		t.Fatalf("an unrestricted session was refused because a sibling is restricted:\n%s", ok)
	}
	if !strings.HasPrefix(ok, "TERM/1 2") {
		t.Fatalf("open through an unrestricted session failed:\n%s", ok)
	}

	// And the restricted session itself is still refused: the gate moved, it did
	// not disappear.
	no := termText(t, s, map[string]any{"cmd": "open " + locked + " cdp " + srv})
	if !strings.Contains(no, "403") || !strings.Contains(no, "does not grant cdp.open") {
		t.Fatalf("the restricted session must still be refused 403:\n%s", no)
	}
}

// devtoolsStub is a DevTools endpoint that advertises a socket, enough for the
// door's discovery step to succeed without a browser.
func devtoolsStub(t *testing.T) string {
	t.Helper()
	up := websocket.Upgrader{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/version" {
			_, _ = w.Write([]byte(`{"webSocketDebuggerUrl":"ws://` + r.Host + `/devtools/browser/stub"}`))
			return
		}
		conn, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv.URL
}
