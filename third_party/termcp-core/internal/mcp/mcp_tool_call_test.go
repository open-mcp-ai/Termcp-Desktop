package mcp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestMCPHighLevelToolCallFillsEnvelopeAndStripsResult pins issue #159 item A.
//
// A caller sending `call <session> mcp:1 <tool> <args>` gets the tools/call
// envelope filled in automatically, and reads the tool's TEXT back rather than
// the outer JSON-RPC envelope. The escape hatch (hand-crafted JSON starting with
// '{') stays untouched.
func TestMCPHighLevelToolCallFillsEnvelopeAndStripsResult(t *testing.T) {
	// A mock MCP server speaking streamable HTTP.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		id := req["id"]
		method, _ := req["method"].(string)
		w.Header().Set("Content-Type", "application/json")

		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": id,
				"result": map[string]any{
					"protocolVersion": "2024-11-05",
					"serverInfo":      map[string]any{"name": "mock", "version": "1.0"},
					"capabilities":    map[string]any{"tools": map[string]any{}},
				},
			})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": id,
				"result": map[string]any{
					"tools": []map[string]any{
						{
							"name":        "echo",
							"description": "Echo the input text back to the caller",
							"inputSchema": map[string]any{
								"type":       "object",
								"properties": map[string]any{"msg": map[string]any{"type": "string"}},
								"required":   []string{"msg"},
							},
						},
						{
							"name":        "fail_tool",
							"description": "Always reports isError",
							"inputSchema": map[string]any{"type": "object"},
						},
					},
				},
			})
		case "tools/call":
			params, _ := req["params"].(map[string]any)
			name, _ := params["name"].(string)
			args, _ := params["arguments"].(map[string]any)
			if name == "fail_tool" {
				_ = json.NewEncoder(w).Encode(map[string]any{
					"jsonrpc": "2.0", "id": id,
					"result": map[string]any{
						"isError": true,
						"content": []map[string]any{{"type": "text", "text": "permission denied on device"}},
					},
				})
				return
			}
			msg, _ := args["msg"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"jsonrpc": "2.0", "id": id,
				"result": map[string]any{
					"content": []map[string]any{
						{"type": "text", "text": "hello " + msg},
					},
				},
			})
		default:
			http.Error(w, "unknown method: "+method, 404)
		}
	}))
	defer srv.Close()

	s := newTestServer(t)
	internalSession(t, s)

	h := handleFrom(t, termText(t, s, map[string]any{"cmd": "open internal mcp " + srv.URL}))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// 1. tools list via high-level `get internal mcp:1 tools`
	list := termText(t, s, map[string]any{"cmd": "get " + h + " tools"})
	if !strings.Contains(list, "echo") || !strings.Contains(list, "required: msg") {
		t.Fatalf("tools list did not describe echo with its required keys:\n%s", list)
	}

	// 2. High-level call: no envelope, text directly in the reply.
	got := termText(t, s, map[string]any{"cmd": `call ` + h + ` echo {"msg":"world"}`})
	if !strings.HasPrefix(got, "TERM/1 200") {
		t.Fatalf("tool call failed:\n%s", got)
	}
	if strings.Contains(got, "jsonrpc") || strings.Contains(got, "content") {
		t.Errorf("envelope was not stripped from the text reply:\n%s", got)
	}
	if !strings.Contains(got, "hello world") {
		t.Errorf("expected tool output 'hello world', got:\n%s", got)
	}

	// 3. --json keeps the envelope intact.
	asJSON := termText(t, s, map[string]any{"cmd": `call ` + h + ` --json echo {"msg":"world"}`})
	if !strings.Contains(asJSON, `"jsonrpc"`) || !strings.Contains(asJSON, `"result"`) {
		t.Errorf("--json did not return the frame:\n%s", asJSON)
	}

	// 4. isError turns into a 500 failure with ERROR: prefix.
	failed := termText(t, s, map[string]any{"cmd": `call ` + h + ` fail_tool {}`})
	if !strings.HasPrefix(failed, "TERM/1 500") {
		t.Fatalf("a failing tool must answer 500, not 200:\n%s", failed)
	}
	if !strings.Contains(failed, "ERROR: permission denied on device") {
		t.Errorf("expected ERROR: prefix on failure, got:\n%s", failed)
	}

	// 5. Raw frame escape hatch still works bit-for-bit.
	raw := termText(t, s, map[string]any{"cmd": `call ` + h + ` {"jsonrpc":"2.0","id":99,"method":"tools/list"}`})
	if !strings.Contains(raw, `"tools"`) {
		t.Errorf("raw frame escape hatch failed:\n%s", raw)
	}
}
