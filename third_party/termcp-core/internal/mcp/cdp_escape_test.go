package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
)

// The frame log keeps the bytes as they arrived, and a peer is free to escape
// every non-ASCII character — Chrome's DevTools protocol does exactly that. So a
// Chinese page's title arrives as \u817e\u8baf\u4e91, and the same bytes `call`
// renders as text read back from the log as an opaque run of escapes.
//
// That made a big answer worse than a small one: over the inline limit a caller
// must page through --at, and --at was the one form that could not be read.
func TestPagedFrameDecodesJSONEscapes(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	srv := escapedDevtools(t)

	// Open the connection first: `cdp@T1` names a page on a connection, and there
	// is no connection until one is opened.
	conn := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + conn})
	attach := termText(t, s, map[string]any{"cmd": "open " + conn + " --target T1"})
	if !strings.Contains(attach, "attached") {
		t.Fatalf("could not attach the fake page:\n%s", attach)
	}

	got := termText(t, s, map[string]any{"cmd": `call ` + conn + `@T1 {"method":"Runtime.evaluate"}`})
	// The receipt names the WINDOW that stopped it now, not a constant (issue #204).
	if !strings.Contains(got, "so it is a receipt instead") {
		t.Fatalf("the answer should have been receipted, not inlined:\n%s", got)
	}
	frame, ok := headerValue(t, got, "Frame")
	if !ok {
		t.Fatalf("the receipt does not name the frame:\n%s", got)
	}

	// THE DEFAULT IS READABLE. The peer sent escapes; the page hands back text.
	page := termText(t, s, map[string]any{
		"cmd": `read ` + conn + `@T1 --at ` + frame + ` --offset 0 --bytes 400`,
	})
	if !strings.Contains(page, "腾讯云") {
		t.Errorf("a paged frame should decode the peer's escapes:\n%s", page)
	}
	if !strings.Contains(page, `\u817e`) {
		// Guard against the test passing because the fake stopped escaping.
		t.Log("the page no longer contains the raw escape, which is the point")
	}
	if !strings.Contains(page, "Served: decoded") {
		t.Errorf("the reply must say what it served:\n%s", page)
	}

	// --raw IS THE BYTES AS THEY ARRIVED, which is what to compare when a reply
	// looks wrong. It used to be accepted on this path and never read.
	raw := termText(t, s, map[string]any{
		"cmd": `read ` + conn + `@T1 --at ` + frame + ` --offset 0 --bytes 400 --raw`,
	})
	if !strings.Contains(raw, `\u817e`) {
		t.Errorf("--raw must return the peer's own bytes:\n%s", raw)
	}
	if strings.Contains(raw, "腾讯云") {
		t.Errorf("--raw must not decode:\n%s", raw)
	}
	if !strings.Contains(raw, "Served: raw") {
		t.Errorf("--raw must say it served the bytes:\n%s", raw)
	}

	// A PAGE THAT STARTS MID-STRING. This is the page a caller actually makes: the
	// receipt names offset 512, which lands inside the value's string literal, and a
	// decoder that tracked "am I inside a string?" from the start of the window would
	// decide it was outside one and decode nothing.
	mid := termText(t, s, map[string]any{
		"cmd": `read ` + conn + `@T1 --at ` + frame + ` --offset 300 --bytes 300`,
	})
	if !strings.Contains(mid, "腾讯云") {
		t.Errorf("a page starting inside a string must still decode:\n%s", mid)
	}

	// THE SEAM BETWEEN PAGES LANDS ON A BOUNDARY. Next-Offset is a byte count this
	// code picks, so a count that fell inside a \uXXXX escape would open the next
	// page with the leftover hex digits.
	small := termText(t, s, map[string]any{
		"cmd": `read ` + conn + `@T1 --at ` + frame + ` --offset 0 --bytes 11`,
	})
	nextSmall, ok := headerValue(t, small, "Next-Offset")
	if !ok {
		t.Fatalf("a partial page must say where to continue:\n%s", small)
	}
	// Every seam of a chained walk must open on a character, never on "17e".
	at := nextSmall
	for i := 0; i < 12; i++ {
		p := termText(t, s, map[string]any{
			"cmd": `read ` + conn + `@T1 --at ` + frame + ` --offset ` + at + ` --bytes 11`,
		})
		body := p[strings.Index(p, "\n\n")+2:]
		if strings.HasPrefix(body, "7e") || strings.HasPrefix(body, "e ") || strings.HasPrefix(body, `e\u`) {
			t.Errorf("page seam at %s opens on leftover hex digits: %q", at, body)
		}
		nextAt, ok := headerValue(t, p, "Next-Offset")
		if !ok {
			break
		}
		at = nextAt
	}

	// Paging still works: the offsets are offsets into the STORED frame, so the
	// second page starts where the first ended even though the rendering is shorter.
	next, ok := headerValue(t, page, "Next-Offset")
	if !ok {
		t.Fatalf("a partial page must say where to continue:\n%s", page)
	}
	tail := termText(t, s, map[string]any{
		"cmd": `read ` + conn + `@T1 --at ` + frame + ` --offset ` + next + ` --bytes 200`,
	})
	if !strings.Contains(tail, "TERM/1 200") {
		t.Errorf("continuing from Next-Offset should answer:\n%s", tail)
	}
}

// escapedDevtools answers every request with a frame big enough to be receipted,
// carrying its non-ASCII text the way Chrome does: escaped, not as UTF-8.
func escapedDevtools(t *testing.T) *httptest.Server {
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
			id, _ := json.Marshal(frame["id"])
			method, _ := frame["method"].(string)
			var body string
			if method == "Target.attachToTarget" {
				body = `{"id":` + string(id) + `,"result":{"sessionId":"SID-T1"}}`
			} else {
				// Hand-built, because WriteJSON would encode this text as UTF-8 and
				// the escaping is exactly what the test is about.
				body = `{"id":` + string(id) + `,"result":{"result":{"type":"string","value":"` +
					strings.Repeat(`\u817e\u8baf\u4e91 `, 2000) + `"}}}`
			}
			if err := conn.WriteMessage(websocket.TextMessage, []byte(body)); err != nil {
				return
			}
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// A PAGE THAT MAKES NO PROGRESS IS A LOOP. Next-Offset is the only thing telling a
// caller where to continue, so a page whose end backs up to where that page started
// would spin forever. A three-byte window is the way to land on an escape boundary
// often, and walking the whole frame proves the walk terminates whatever the layout.
func TestPagedFrameAlwaysAdvances(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	srv := escapedDevtools(t)
	conn := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + conn})
	termText(t, s, map[string]any{"cmd": "open " + conn + " --target T1"})

	got := termText(t, s, map[string]any{"cmd": `call ` + conn + `@T1 {"method":"Runtime.evaluate"}`})
	frame := regexp.MustCompile(`frame (\d+) is`).FindStringSubmatch(got)
	if frame == nil {
		t.Fatalf("no receipt to walk:\n%s", got)
	}

	at := 0
	for i := 0; i < 100000; i++ {
		p := termText(t, s, map[string]any{
			"cmd": `read ` + conn + `@T1 --at ` + frame[1] + ` --offset ` + strconv.Itoa(at) + ` --bytes 3`,
		})
		if strings.Contains(p, "Error-Code") {
			t.Fatalf("the walk failed at offset %d:\n%s", at, p)
		}
		if body := p[strings.Index(p, "\n\n")+2:]; body == "" {
			t.Fatalf("page at offset %d came back empty, so a caller learns nothing", at)
		}
		next, ok := headerValue(t, p, "Next-Offset")
		if !ok {
			return // the last page: the walk finished
		}
		n, err := strconv.Atoi(next)
		if err != nil {
			t.Fatalf("Next-Offset %q is not a number", next)
		}
		if n <= at {
			t.Fatalf("page at offset %d ends at %d, so following Next-Offset never advances", at, n)
		}
		at = n
	}
	t.Fatal("the walk never reached the end of the frame")
}

// THE TWO DOORS MUST AGREE ABOUT ONE FRAME. A receipt measures the frame to decide
// whether to inline it, and the pager measures it again to serve it. When those two
// measurements were taken from different renderings, the same header name meant the
// log's size on one door and the text's size on the other — a caller had no way to
// tell which number it was holding.
func TestFrameSizeHeadersAgreeAcrossBothDoors(t *testing.T) {
	s := newTestServer(t)
	internalSession(t, s)
	srv := escapedDevtools(t)
	conn := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + conn})
	termText(t, s, map[string]any{"cmd": "open " + conn + " --target T1"})

	receipt := termText(t, s, map[string]any{"cmd": `call ` + conn + `@T1 {"method":"Runtime.evaluate"}`})
	frame := regexp.MustCompile(`frame (\d+) is`).FindStringSubmatch(receipt)
	if frame == nil {
		t.Fatalf("no receipt:\n%s", receipt)
	}
	rBytes, okB := headerValue(t, receipt, "Bytes")
	rDecoded, okD := headerValue(t, receipt, "Decoded-Bytes")
	if !okB || !okD {
		t.Fatalf("the receipt must report both sizes:\n%s", receipt)
	}
	// The peer escaped this payload, so the log's copy is the larger one.
	if rBytes == rDecoded {
		t.Errorf("an escaped frame should differ between Bytes and Decoded-Bytes, both said %s", rBytes)
	}

	page := termText(t, s, map[string]any{
		"cmd": `read ` + conn + `@T1 --at ` + frame[1] + ` --offset 0 --bytes 40`,
	})
	pBytes, _ := headerValue(t, page, "Bytes")
	pDecoded, _ := headerValue(t, page, "Decoded-Bytes")
	if pBytes != rBytes || pDecoded != rDecoded {
		t.Errorf("the receipt says %s/%s and the pager says %s/%s", rBytes, rDecoded, pBytes, pDecoded)
	}

	// --raw changes what is served, not how big the frame is.
	raw := termText(t, s, map[string]any{
		"cmd": `read ` + conn + `@T1 --at ` + frame[1] + ` --offset 0 --bytes 40 --raw`,
	})
	if got, _ := headerValue(t, raw, "Bytes"); got != rBytes {
		t.Errorf("--raw reported Bytes %s, the receipt said %s", got, rBytes)
	}
	if got, _ := headerValue(t, raw, "Decoded-Bytes"); got != rDecoded {
		t.Errorf("--raw reported Decoded-Bytes %s, the receipt said %s", got, rDecoded)
	}

	// A PEER THAT DOES NOT ESCAPE gets two equal numbers, which is the honest answer
	// rather than a header that is absent for some frames and present for others.
	_, plainSrv := fakeDevtools(t)
	plain := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal cdp " + plainSrv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + plain})
	termText(t, s, map[string]any{"cmd": "open " + plain + " --target T1"})
	plainReceipt := termText(t, s, map[string]any{"cmd": `call ` + plain + `@T1 {"method":"Runtime.evaluate"}`})
	pf := regexp.MustCompile(`frame (\d+) is`).FindStringSubmatch(plainReceipt)
	if pf == nil {
		t.Fatalf("no receipt from the plain peer:\n%s", plainReceipt)
	}
	plainPage := termText(t, s, map[string]any{
		"cmd": `read ` + plain + `@T1 --at ` + pf[1] + ` --offset 0 --bytes 40`,
	})
	pb, _ := headerValue(t, plainPage, "Bytes")
	pd, _ := headerValue(t, plainPage, "Decoded-Bytes")
	if pb != pd {
		t.Errorf("a peer that escapes nothing should report one size, got %s and %s", pb, pd)
	}
}
