package extensions

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestURLTemplateCreatesRelayChannelWithoutExplicitMode(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var env map[string]any
		_ = json.NewDecoder(r.Body).Decode(&env)
		id := env["id"]
		w.Header().Set("Content-Type", "application/json")
		switch env["method"] {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo":      map[string]any{"name": "relay-test", "version": "1"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}})
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"tools": []any{map[string]any{"name": "ping", "inputSchema": map[string]any{"type": "object"}}},
			}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
		}
	}))
	defer upstream.Close()

	reconciler := startReconciler(t)

	ext := &Extension{
		Name:   "url-without-mode",
		Target: "internal",
		URL:    upstream.URL,
	}
	if err := SaveStruct(reconciler.Store(), ext); err != nil {
		t.Fatal(err)
	}
	status, err := reconciler.Acquire(ext.Name, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if status.SessionID == "" || status.ShellID == "" {
		t.Fatalf("URL relay status has no channel: %+v", status)
	}
	sess := reconciler.sessMgr.Get(status.SessionID)
	if sess == nil {
		t.Fatal("URL relay session is missing")
	}
	shell := sess.GetChildShell(status.ShellID)
	if shell == nil {
		t.Fatal("URL relay channel is not reachable through its session")
	}
	if got := shell.Info().Mode; got != "pipe" {
		t.Fatalf("URL relay mode = %q, want pipe", got)
	}
}
