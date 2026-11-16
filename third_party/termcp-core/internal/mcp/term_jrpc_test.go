package mcp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

// devtoolsPeer is a fake browser: a socket that answers by id, pushes one event
// before every answer, and REFUSES `initialize` the way DevTools does. A door
// that sends a handshake therefore fails loudly instead of quietly working.
func devtoolsPeer(t *testing.T) *httptest.Server {
	t.Helper()
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
			if frame["method"] == "initialize" {
				_ = conn.WriteJSON(map[string]any{
					"id":    frame["id"],
					"error": map[string]any{"code": -32601, "message": "'initialize' wasn't found"},
				})
				continue
			}
			// An event first: no id, so it is not the answer.
			_ = conn.WriteJSON(map[string]any{
				"method": "Page.lifecycleEvent",
				"params": map[string]any{"name": "load"},
			})
			_ = conn.WriteJSON(map[string]any{"id": frame["id"], "result": map[string]any{"ok": true}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCDPDoorSendsNoHandshake is the reason cdp is its own entry rather than a
// flag on mcp.
//
// DevTools has no initialize: it answers one with "method not found" and a
// caller reading that reply concludes the browser is broken. The fake peer here
// refuses it the same way, so a door that sends one cannot pass this test by
// accident.
func TestCDPDoorSendsNoHandshake(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	srv := devtoolsPeer(t)

	out := termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL})
	h := handleFrom(t, out)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	body := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Target.getTargets"}`})
	if !strings.Contains(body, `"ok":true`) {
		t.Fatalf("the first cdp call did not reach the peer:\n%s", body)
	}
	if strings.Contains(body, "wasn't found") {
		t.Fatalf("a handshake was sent to a DevTools peer:\n%s", body)
	}
}

// TestCDPEventSurvivesTheCallThatDisplacedIt pins the event path: the lifecycle
// event arrives while the call waits for its own answer, and must still be
// readable afterwards.
func TestCDPEventSurvivesTheCallThatDisplacedIt(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	srv := devtoolsPeer(t)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Page.enable"}`})
	event := termText(t, s, map[string]any{"cmd": "read " + h})
	if !strings.Contains(event, "Page.lifecycleEvent") {
		t.Fatalf("the event pushed during the call was lost:\n%s", event)
	}
	// recv is a consumer, like read: the second one has nothing to give. It is
	// asked with timeout 0 so the check is a peek rather than another wait.
	again := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": float64(0)})
	if strings.Contains(again, "Page.lifecycleEvent") {
		t.Fatalf("recv re-delivered the same event:\n%s", again)
	}
}

// TestRecvWaitsForAnEventThatFiresAfterTheCall is the regression test for the
// bug a REAL browser found and the fake peers could not.
//
// `Page.navigate` answers first; `Page.loadEventFired` fires afterwards, on the
// peer's own schedule. A recv that only looked at what the previous call happened
// to catch answered 204 and the caller concluded the page never loaded — which is
// the single most common thing a CDP caller does. recv therefore reads the socket
// itself and waits out the call's window.
func TestRecvWaitsForAnEventThatFiresAfterTheCall(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	srv := delayedEventPeer(t, 250*time.Millisecond)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// The call answers at once and pushes nothing: the event is still 250ms
	// away, which is exactly the gap that used to lose it.
	termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Page.navigate"}`})
	out := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": float64(3)})
	if !strings.Contains(out, "Page.loadEventFired") {
		t.Fatalf("recv did not wait for the event that fired after the call:\n%s", out)
	}
}

// TestRecvHonoursItsWindow pins that the wait is bounded by the call's own
// timeout parameter, and that an expired window is a 204 rather than a failure.
func TestRecvHonoursItsWindow(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)                 // gateways need a session to tunnel through
	srv := delayedEventPeer(t, time.Hour) // never inside the window
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	start := time.Now()
	out := termText(t, s, map[string]any{"cmd": "read " + h, "timeout": float64(0.3)})
	waited := time.Since(start)
	if strings.Contains(out, "connection_failed") {
		t.Fatalf("an expired window was reported as a failure:\n%s", out)
	}
	if waited > 5*time.Second {
		t.Fatalf("recv waited %v for a 0.3s window", waited)
	}

	// timeout 0 is the non-blocking peek and must return at once.
	start = time.Now()
	termText(t, s, map[string]any{"cmd": "read " + h, "timeout": float64(0)})
	if peek := time.Since(start); peek > 2*time.Second {
		t.Fatalf("timeout 0 blocked for %v", peek)
	}
}

// delayedEventPeer answers every call at once and pushes one event after delay,
// which is the ordering a real page produces and a fake that pushes first cannot.
func delayedEventPeer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
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
			_ = conn.WriteJSON(map[string]any{"id": frame["id"], "result": map[string]any{"ok": true}})
			go func() {
				time.Sleep(delay)
				_ = conn.WriteJSON(map[string]any{"method": "Page.loadEventFired", "params": map[string]any{}})
			}()
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCDPResolvesTheDevtoolsEndpoint pins that a caller may pass the http
// endpoint and let the door read /json/version, instead of being told to fetch a
// JSON document and paste a URL out of it.
func TestCDPResolvesTheDevtoolsEndpoint(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	srv := devtoolsPeer(t)
	out := termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL})
	if !strings.Contains(out, "ws://") {
		t.Fatalf("the http endpoint was not resolved to its socket:\n%s", out)
	}
	h := handleFrom(t, out)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	if body := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"Target.getTargets"}`}); !strings.Contains(body, `"ok":true`) {
		t.Fatalf("the resolved socket did not work:\n%s", body)
	}
}

// TestJrpcDoorSpeaksAnyEnvelope pins that the door adds nothing: a frame with no
// jsonrpc field and no handshake is passed through, which is what a private RPC
// or an LSP peer needs.
func TestJrpcDoorSpeaksAnyEnvelope(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	srv := devtoolsPeer(t)
	// A socket, because the fake peer upgrades: the jrpc door takes either scheme
	// and this is the one a private RPC or an LSP server listens on.
	ws := "ws" + strings.TrimPrefix(srv.URL, "http")
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal jrpc " + ws}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// No "jsonrpc" key, and no initialize before it.
	body := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"method":"textDocument/hover","params":{}}`})
	if !strings.Contains(body, `"ok":true`) {
		t.Fatalf("a bare envelope did not reach the peer:\n%s", body)
	}
}

// TestMcpDoorStillHandshakes pins that unifying the transport did not lose the
// one thing the MCP door adds.
func TestMcpDoorStillHandshakes(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	ts := httptest.NewServer(s.StreamableHTTPHandler())
	defer ts.Close()

	// THE HANDSHAKE AND THE TOOL LIST ARRIVE WITH open. `mcp tools <handle>` used to be
	// a second call for it; discovery now runs as part of opening, so the reply that
	// hands back the handle is also the one that says what the peer offers — a caller
	// that has never read the MCP spec can make its next call from this alone.
	opened := termText(t, s, map[string]any{"cmd": "open internal mcp " + ts.URL})
	h := handleFrom(t, opened)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	if !strings.Contains(opened, "tools (") || !strings.Contains(opened, "term") {
		t.Fatalf("the mcp door did not report its tools:\n%s", opened)
	}
}

// TestToolsIsRefusedOnTheDoorsWithoutOne pins that a subcommand belonging to
// another protocol is a sentence, not a frame sent into the void.
func TestToolsIsRefusedOnTheDoorsWithoutOne(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s) // gateways need a session to tunnel through
	srv := devtoolsPeer(t)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// `tools` is a subcommand of the mcp door, not of a DevTools socket: a cdp peer
	// has no tool list to ask for, and saying so is the whole point.
	res := termCall(t, s, map[string]any{"cmd": "call " + h + ` {"jsonrpc":"2.0","id":1,"method":"tools/list"}`})
	body := toolResultText(res)
	if !res.IsError && !strings.Contains(body, "tools") {
		t.Fatalf("a cdp peer answered a tools/list without explaining itself:\n%s", body)
	}
}

// TestStdioFormIsUnchanged pins the half of `mcp` that never touched the
// registry: a handle naming a SHELL is still a process to type at.
func TestStdioFormIsUnchanged(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, "cat")
	frame := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	if out := termText(t, s, map[string]any{"cmd": "call " + h + " --send " + frame}); !statusLineIs(out, "202") {
		t.Fatalf("the stdio form stopped accepting a frame:\n%s", out)
	}
}

// TestUnknownHandleOnASocketDoorIsNotAStdioFallback pins that the registry
// lookup does not silently route a typo at `cdp` into the shell form.
func TestUnknownHandleOnASocketDoorIsNotAStdioFallback(t *testing.T) {
	s := newTestServer(t)
	// AN UNKNOWN HANDLE IS NOT A SHELL. It used to fall back to the stdio form, which
	// types the frame at a channel by that name — so a typo in a peer handle silently
	// became a command in a terminal. A shell has to be asked for with --frame, and a
	// handle that resolves to nothing says so.
	res := termCall(t, s, map[string]any{"cmd": `call nosuch {"method":"x"}`})
	body := toolResultText(res)
	if !res.IsError {
		t.Fatalf("an unknown handle was accepted:\n%s", body)
	}
	if !strings.Contains(body, "not found") {
		t.Errorf("the refusal should say the handle is unknown:\n%s", body)
	}
}

// TestThreeDoorsAreInHelp pins that the doors are discoverable: a silent
// directive is one nobody uses.
func TestThreeDoorsAreInHelp(t *testing.T) {
	s := newTestServer(t)
	body := termText(t, s, map[string]any{"cmd": "help"})
	// The three doors are KINDS OF `open`, so the shape a caller reads is
	// `open <session> mcp|cdp|jrpc <url>` rather than three separate verbs.
	for _, want := range []string{"mcp|cdp|jrpc"} {
		if !strings.Contains(body, want) {
			t.Errorf("`help` does not name %q", want)
		}
	}
}

// TestCdpOpenRefusesAURLItCannotResolve pins the failure a caller gets from a
// dead endpoint: a sentence naming the problem, not an empty handle.
//
// The session comes first in the call because it is the larger decision — it says
// which network the URL is resolved on — so this test opens one and then hands cdp a
// scheme no transport can carry.
func TestCdpOpenRefusesAURLItCannotResolve(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	res := termCall(t, s, map[string]any{"cmd": "open internal cdp ftp://example.com"})
	if !res.IsError || !strings.Contains(toolResultText(res), "devtools endpoint") {
		t.Fatalf("cdp open accepted a scheme it cannot read:\n%s", toolResultText(res))
	}
}

// TestGatewayOpenRequiresASession pins the new first argument.
//
// A bare URL used to mean "connect from this process", which is a different network
// from the one the target host sees: localhost:9222 is a devtools port on the machine
// termcp runs on, not on the machine being controlled. Naming the session is what
// makes the URL mean what it looks like, so a missing session is refused with the two
// spellings that fix it rather than defaulted.
func TestGatewayOpenRequiresASession(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	for _, op := range []string{"mcp", "cdp", "jrpc"} {
		// A bare URL leaves the kind unreadable: `open http://…` has no session to
		// tunnel through, and the refusal has to name the shape that works.
		res := termCall(t, s, map[string]any{"cmd": "open " + op + " http://127.0.0.1:9/"})
		body := toolResultText(res)
		if !res.IsError {
			t.Errorf("open %s accepted a URL with no session:\n%s", op, body)
		}
		if !strings.Contains(body, "open <session>") {
			t.Errorf("open %s must show the shape that works:\n%s", op, body)
		}
	}
}

// TestGatewayTunnelsThroughTheSession pins that the dial actually travels the SSH
// connection rather than this process's network.
//
// The peer is a server bound to 127.0.0.1 inside the test process, and the session is
// the built-in internal loopback — which is also this process. So the assertion is not
// "the address is unreachable from here"; it is that the request ARRIVED, having gone
// out through a session that was named. A gateway that ignored the session would still
// pass a reachability check, which is why the earlier tests could not tell the two
// apart.
func TestGatewayTunnelsThroughTheSession(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	ts := httptest.NewServer(s.StreamableHTTPHandler())
	defer ts.Close()

	out := termText(t, s, map[string]any{"cmd": "open internal mcp " + ts.URL})
	h, ok := headerValue(t, out, "Handle")
	if !ok {
		t.Fatalf("mcp open did not return a handle, so the tunnel did not carry it:\n%s", out)
	}
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	if !strings.Contains(out, "tools") {
		t.Errorf("the tunnel reached the peer but discovery found no tools:\n%s", out)
	}
}
