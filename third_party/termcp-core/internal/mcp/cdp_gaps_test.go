package mcp

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// These fill the cdp surface the earlier tests did not reach: the discovery failure,
// the protocol document, the oversized-frame receipt, and the page form of `read`.
//
// Each one is a place where the door has a decision to make that a fake browser can
// see from the inside — the method it was asked for, the params it received — rather
// than a place where a real Chrome is needed.

// devtoolsWithoutSocket answers /json/version with a document that has no socket in
// it, which is what a caller gets from a URL that is a web page rather than a
// DevTools endpoint.
func devtoolsWithoutSocket(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"Browser":"Chrome/1.0"}`))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestCdpOpenNamesTheEndpointItCouldNotFind pins the discovery failure.
//
// A caller pasting the wrong port gets "cannot find DevTools websocket endpoint at
// <the URL they wrote>" — naming their own input, not a stack. Without it the failure
// is a dial error three layers down that says nothing about which URL was tried.
func TestCdpOpenNamesTheEndpointItCouldNotFind(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	srv := devtoolsWithoutSocket(t)
	got := termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL})
	if !strings.HasPrefix(got, "TERM/1 4") {
		t.Fatalf("a URL with no DevTools socket was accepted:\n%s", got)
	}
	if !strings.Contains(got, "cannot find DevTools websocket endpoint") {
		t.Errorf("the failure does not say what was missing:\n%s", got)
	}
	if !strings.Contains(got, srv.URL) {
		t.Errorf("the failure does not repeat the URL that was tried:\n%s", got)
	}
}

// TestCdpProtocolIsReadOffTheEndpoint pins `get <peer> protocol`, including the
// argument form.
//
// The protocol document is the only non-guessing source for a method signature, so
// the door has to reach it on the same host and port as the socket — that is what
// httpBaseOf exists for, and a ws:// address is the case where it has to derive the
// http one rather than remember it.
func TestCdpProtocolIsReadOffTheEndpoint(t *testing.T) {
	b, srv := fakeDevtools(t)
	srv.Config.Handler = withProtocolDoc(srv.Config.Handler)
	s := newTestServer(t)
	internalSession(t, s)
	handle := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	_ = b

	all := termText(t, s, map[string]any{"cmd": "get " + handle + " protocol"})
	if !strings.HasPrefix(all, "TERM/1 200") {
		t.Fatalf("the protocol document was not readable:\n%s", all)
	}
	if !strings.Contains(all, "Page") || !strings.Contains(all, "Runtime") {
		t.Errorf("the domain table does not list the domains the document holds:\n%s", all)
	}
	if !strings.Contains(all, "1 commands") {
		t.Errorf("the domain table does not count the commands:\n%s", all)
	}

	one := termText(t, s, map[string]any{"cmd": "get " + handle + " protocol Page"})
	if !strings.HasPrefix(one, "TERM/1 200") {
		t.Fatalf("a single domain was not readable:\n%s", one)
	}
	if strings.Contains(one, "Runtime.evaluate") {
		t.Errorf("asking for one domain returned another:\n%s", one)
	}

	missing := termText(t, s, map[string]any{"cmd": "get " + handle + " protocol Nosuch"})
	if !strings.HasPrefix(missing, "TERM/1 4") {
		t.Errorf("an unknown domain was answered as if it existed:\n%s", missing)
	}
	if !strings.Contains(missing, "Nosuch") {
		t.Errorf("the refusal does not name the domain:\n%s", missing)
	}
}

// frameNumberIn reads the frame number out of a receipt.
func frameNumberIn(t *testing.T, receipt string) string {
	t.Helper()
	m := regexp.MustCompile(`frame (\d+) is`).FindStringSubmatch(receipt)
	if m == nil {
		t.Fatalf("the receipt does not say which frame holds the bytes:\n%s", receipt)
	}
	return m[1]
}

// withProtocolDoc adds /json/protocol to a handler, the way DevTools publishes it.
func withProtocolDoc(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/json/protocol" {
			_, _ = w.Write([]byte(`{"version":{"major":"1","minor":"3"},"domains":[
				{"domain":"Page","commands":[{"name":"navigate","parameters":[{"name":"url","type":"string"}]}]},
				{"domain":"Runtime","commands":[{"name":"evaluate","parameters":[{"name":"expression","type":"string"}]}]}]}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

// TestAnOversizedFrameIsAReceiptAndPagesBack pins the path a big answer takes.
//
// A frame too large to inline becomes a RECEIPT: it says which frame number holds the
// bytes, how big it is, and how to read it in slices. The alternative — truncating it
// into the reply — is the failure this shape exists to prevent, because a caller
// cannot tell a complete answer from a cut one.
func TestAnOversizedFrameIsAReceiptAndPagesBack(t *testing.T) {
	b, srv := fakeDevtools(t)
	s := newTestServer(t)
	internalSession(t, s)
	handle := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))

	// Runtime.evaluate answers with 36 KB of multibyte text.
	got := termText(t, s, map[string]any{"cmd": `call internal cdp@T1 '{"method":"Runtime.evaluate","params":{"expression":"1"}}'`})
	if !strings.HasPrefix(got, "TERM/1 2") {
		t.Fatalf("the oversized answer was not accepted:\n%s", got)
	}
	// The receipt names the WINDOW that stopped it now, not a constant (issue #204).
	if !strings.Contains(got, "so it is a receipt instead") {
		t.Fatalf("a 36 KB answer was inlined instead of receipted:\n%s", got)
	}
	if !strings.Contains(got, "--at") {
		t.Errorf("the receipt does not say how to read the frame:\n%s", got)
	}

	// And the frame pages back, whole and valid UTF-8 at the slice boundary.
	// THE FRAME NUMBER COMES FROM THE RECEIPT, not from guessing: the peer pushes
	// events between answers, so which slot holds the bytes is a fact only the receipt
	// knows. A caller that hardcodes a number is reading somebody else's frame.
	at := frameNumberIn(t, got)
	first := termText(t, s, map[string]any{"cmd": "read " + handle + " --at " + at + " --bytes 100"})
	if !strings.HasPrefix(first, "TERM/1 2") {
		t.Fatalf("the frame did not page back:\n%s", first)
	}
	if !strings.Contains(first, "日本語") {
		t.Errorf("the page does not hold the text the frame holds:\n%s", first)
	}
	second := termText(t, s, map[string]any{"cmd": "read " + handle + " --at " + at + " --offset 100 --bytes 100"})
	if strings.Contains(second, "\ufffd") {
		t.Errorf("a slice boundary cut a character in half:\n%s", second)
	}
	if !strings.Contains(second, "日本語") {
		t.Errorf("the second page does not hold text:\n%s", second)
	}
	_ = b
}

// targetIDFromList reads the one target a fake browser publishes.
func targetIDFromList(t *testing.T, s *Server, handle string) string {
	t.Helper()
	list := termText(t, s, map[string]any{"cmd": "get " + handle})
	if !strings.Contains(list, "T1") {
		t.Fatalf("no target to address:\n%s", list)
	}
	return "T1"
}

// TestProtocolWorksForASocketOpenedByHand pins the http base derived from a ws://
// address.
//
// A caller that opened the socket directly (which is what DevTools' own banner prints)
// has no http URL to paste, and the protocol document lives on the same host and port.
// Deriving it is exact, not a guess — and it is the only way `get <peer> protocol` can
// work for that caller at all.
func TestProtocolWorksForASocketOpenedByHand(t *testing.T) {
	_, srv := fakeDevtools(t)
	srv.Config.Handler = withProtocolDoc(srv.Config.Handler)
	s := newTestServer(t)
	internalSession(t, s)
	ws := "ws" + strings.TrimPrefix(srv.URL, "http") + "/devtools/browser/x"
	handle := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + ws}))
	defer termText(t, s, map[string]any{"cmd": "close " + handle})

	got := termText(t, s, map[string]any{"cmd": "get " + handle + " protocol"})
	if !strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("the protocol document was not reachable from a socket address:\n%s", got)
	}
	if !strings.Contains(got, "Page") {
		t.Errorf("the derived http endpoint did not serve the protocol:\n%s", got)
	}
}

// TestAttachWithoutASessionIdSaysSo pins the reply a browser gives when it accepts an
// attach but names no session.
//
// The failure has to carry the browser's own words: "attach failed" would send a caller
// looking at the browser, while the answer the browser actually gave is the one thing
// that explains it.
func TestAttachWithoutASessionIdSaysSo(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	srv := attachWithoutSessionPeer(t)

	// A page is attached on an OPEN channel: the address is `<session> cdp:<n>`, and
	// the target is what is added to it.
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	got := termText(t, s, map[string]any{"cmd": "open " + h + " --target T1"})
	if !strings.HasPrefix(got, "TERM/1 5") && !strings.HasPrefix(got, "TERM/1 4") {
		t.Fatalf("an attach that returned no sessionId was accepted:\n%s", got)
	}
	if !strings.Contains(got, "sessionId") {
		t.Errorf("the failure does not say what was missing:\n%s", got)
	}
	if !strings.Contains(got, "Target.attachToTarget") {
		t.Errorf("the failure does not name the method that failed:\n%s", got)
	}
}

// attachWithoutSessionPeer answers every attach with an empty result.
func attachWithoutSessionPeer(t *testing.T) *httptest.Server {
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
			_ = conn.WriteJSON(map[string]any{"id": frame["id"], "result": map[string]any{}})
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestGetOnAPeerListsWhatIsAttached pins the branch a caller uses to find out where the
// pages are.
//
// Without an attach the peer has nothing to list and says so with the command that
// attaches one; with an attach it names the child indices, which are what
// `<peer>:<n>@<targetId>` is built from. Both halves are the answer to "what can I
// address", so a peer that listed neither would leave the caller guessing.
func TestGetOnAPeerListsWhatIsAttached(t *testing.T) {
	_, srv := fakeDevtools(t)
	s := newTestServer(t)
	internalSession(t, s)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	empty := termText(t, s, map[string]any{"cmd": "get " + h})
	if !strings.HasPrefix(empty, "TERM/1 2") {
		t.Fatalf("get on a peer did not answer:\n%s", empty)
	}
	if !strings.Contains(empty, "no attached targets") {
		t.Errorf("a peer with nothing attached does not say so:\n%s", empty)
	}

	if got := termText(t, s, map[string]any{"cmd": "open " + h + " --target T1"}); !strings.HasPrefix(got, "TERM/1 2") {
		t.Fatalf("attaching a page failed:\n%s", got)
	}
	attached := termText(t, s, map[string]any{"cmd": "get " + h})
	if !strings.Contains(attached, "attached targets") {
		t.Errorf("an attached page is not listed:\n%s", attached)
	}
	if !strings.Contains(attached, "T1") {
		t.Errorf("the attached page is listed without the target it came from:\n%s", attached)
	}
}
