package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestACallInlinesWhatTheCallerAskedFor is issue #204.
//
// A peer reply over 4096 bytes became a receipt, and 4096 was a constant in two
// separate files — so `call … --bytes 32768` answered with a 512-byte preview and no
// header saying why, on a tool whose own documentation promises the opposite ("a larger
// value is honoured", see term.go). The size of a reply is the caller's decision: it is
// the party paying for the tokens and the only one that knows whether it is reading a
// title or a DOM dump.
//
// The receipt is still the fallback — a frame bigger than the window cannot be inlined
// without producing truncated JSON, which is the thing the receipt exists to avoid. What
// changed is who picks the number, and this asserts it in the only way that can tell the
// two implementations apart: the SAME frame, two windows, two answers.
func TestACallInlinesWhatTheCallerAskedFor(t *testing.T) {
	const payloadBytes = 10000
	big := strings.Repeat("M", payloadBytes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		switch req["method"] {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]any{"name": "mock", "version": "1.0"},
					"capabilities":    map[string]any{"tools": map[string]any{}},
				},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]any{"tools": []map[string]any{{
					"name": "dump", "description": "returns a big payload",
					"inputSchema": map[string]any{"type": "object"},
				}}},
			})
		case "tools/call":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": req["id"],
				"result": map[string]any{
					"content": []map[string]any{{"type": "text", "text": big}},
				},
			})
		default:
			http.Error(w, "unknown method", 404)
		}
	}))
	defer srv.Close()

	s := newTestServer(t)
	internalSession(t, s)
	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal mcp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// The caller's own window is too small: a receipt, and it says so.
	small := termText(t, s, map[string]any{"cmd": `call ` + h + ` dump {}`, "bytes_limit": 4096})
	if !strings.Contains(small, "receipt") {
		t.Fatalf("a %d byte payload cannot fit a 4096 byte window:\n%s", payloadBytes, small)
	}
	if strings.Contains(small, big) {
		t.Error("the receipt inlined the whole body anyway")
	}
	if !strings.Contains(small, "4096") && !strings.Contains(small, "4.0 KB") {
		t.Errorf("the receipt must name the window that stopped it, not a mystery constant:\n%s", small)
	}

	// The same frame, a window that holds it: the text, in full.
	large := termText(t, s, map[string]any{"cmd": `call ` + h + ` dump {}`, "bytes_limit": 32768})
	if strings.Contains(large, "receipt") {
		t.Fatalf("a 32768 byte window was not honoured for a %d byte payload:\n%s", payloadBytes, large)
	}
	if !strings.Contains(large, big) {
		t.Fatalf("the payload was not inlined whole (%d bytes of M expected):\n%.400s", payloadBytes, large)
	}
}
