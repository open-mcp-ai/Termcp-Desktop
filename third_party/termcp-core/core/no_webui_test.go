//go:build no_webui

package core

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/open-mcp-ai/termcp/config"
)

func TestNoWebUIBuildRetainsManagementAPI(t *testing.T) {
	cfg := config.Default()
	cfg.DataDir = t.TempDir()
	srv, err := New(Options{Config: cfg, Version: "v0.2.9"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { srv.Close("probe done") })

	for _, path := range []string{"/api.md", "/api/version", "/api/sessions", "/api/extensions", "/api/approvals", "/api/clients"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, rec.Code)
		}
	}
	for _, path := range []string{"/", "/static/app.js", "/themes/default.css", "/vendor/xterm/xterm.js"} {
		rec := httptest.NewRecorder()
		srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
	recWebSocket := httptest.NewRecorder()
	srv.Handler().ServeHTTP(recWebSocket, httptest.NewRequest(http.MethodGet, "/api/ui/ws", nil))
	if recWebSocket.Code != http.StatusBadRequest {
		t.Errorf("GET /api/ui/ws without Upgrade = %d, want 400", recWebSocket.Code)
	}

	rec := httptest.NewRecorder()
	body := strings.NewReader(`{"cmd":"ls ext"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/term", body)
	req.Header.Set("Content-Type", "application/json")
	srv.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"code":200`) {
		t.Fatalf("POST /api/term ls ext = %d %s", rec.Code, strings.TrimSpace(rec.Body.String()))
	}

	reply := srv.Term(context.Background(), "ls ext", nil)
	if len(reply.Slots) == 0 {
		t.Fatal("ls ext returned no slots")
	}
	if reply.Slots[0].Code != http.StatusOK {
		t.Fatalf("Term(ls ext) = %d %s", reply.Slots[0].Code, reply.Slots[0].Body)
	}
	if srv.Embedded() {
		t.Fatal("no_webui build reports the browser UI as embedded")
	}
	if srv.webui == nil {
		t.Fatal("no_webui build did not construct the management handler")
	}
	if srv.webui.ExecuteOperation == nil {
		t.Fatal("approval execution callback is not wired")
	}

	ts := httptest.NewServer(srv.Handler())
	t.Cleanup(ts.Close)
	wsURL := "ws" + strings.TrimPrefix(ts.URL, "http") + "/api/ui/ws"
	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("connect /api/ui/ws: %v", err)
	}
	defer conn.Close()
	var event struct {
		Type string `json:"type"`
	}
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("read initial WebSocket event: %v", err)
	}
	if event.Type != "sessions" {
		t.Fatalf("initial WebSocket event = %q, want sessions", event.Type)
	}
}
