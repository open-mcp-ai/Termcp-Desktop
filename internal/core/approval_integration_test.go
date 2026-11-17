package core

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestIntegratedApprovalGatesMCPInputAndExecutesOperations(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("TERMCP_DATA_DIR", dataDir)
	service := New("127.0.0.1", freePort(t))
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	socket, _, err := websocket.DefaultDialer.Dial("ws"+strings.TrimPrefix(service.BaseURL(), "http")+"/api/ui/ws", nil)
	if err != nil {
		t.Fatalf("connect approval event stream: %v", err)
	}
	defer socket.Close()
	var initial struct {
		Type string `json:"type"`
	}
	if err := socket.ReadJSON(&initial); err != nil || initial.Type != "sessions" {
		t.Fatalf("initial Core event = %+v, %v", initial, err)
	}

	req := request(t, http.MethodPut, service.BaseURL()+"/api/connections/internal", strings.NewReader("kind = \"internal\"\ndefault_approval = true\n"))
	req.Header.Set("Content-Type", "text/plain")
	checkStatus(t, req, http.StatusNoContent).Body.Close()

	created := checkStatus(t, request(t, http.MethodPost, service.BaseURL()+"/api/sessions", strings.NewReader(`{"ssh_config":"internal","name":"approval-e2e","mode":"pty","rows":24,"cols":80}`)), http.StatusOK)
	var session struct {
		SessionID string `json:"session_id"`
		ShellID   string `json:"shell_id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&session); err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()
	if session.SessionID == "" || session.ShellID == "" {
		t.Fatalf("session create returned %+v", session)
	}

	approval := checkStatus(t, request(t, http.MethodGet, service.BaseURL()+"/api/sessions/"+session.SessionID+"/approval", nil), http.StatusOK)
	var approvalState struct {
		Enabled bool `json:"approval_mode"`
	}
	if err := json.NewDecoder(approval.Body).Decode(&approvalState); err != nil {
		t.Fatal(err)
	}
	_ = approval.Body.Close()
	if !approvalState.Enabled {
		t.Fatal("session did not inherit default approval")
	}

	mcpSession := initializeDesktopMCP(t, service.BaseURL())
	mcpTerm(t, service.BaseURL(), mcpSession, map[string]any{
		"cmd":  "input " + session.ShellID + " $1",
		"args": []any{"echo approval-shell-marker"},
	})
	mcpTerm(t, service.BaseURL(), mcpSession, map[string]any{
		"cmd": "key " + session.ShellID + " enter",
	})
	if err := socket.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	for {
		var event struct {
			Type    string `json:"type"`
			Request struct {
				ID string `json:"id"`
			} `json:"request"`
		}
		if err := socket.ReadJSON(&event); err != nil {
			t.Fatalf("read approval event: %v", err)
		}
		if event.Type == "approval" {
			if event.Request.ID == "" {
				t.Fatal("approval WebSocket event has no request id")
			}
			break
		}
	}
	shellApproval := firstApproval(t, service.BaseURL(), "shell")
	checkStatus(t, request(t, http.MethodPost, service.BaseURL()+"/api/approvals/"+shellApproval.ID+"/approve", strings.NewReader(`{"reason":"desktop integration test"}`)), http.StatusOK).Body.Close()
	waitForOutput(t, service.BaseURL(), session.ShellID, "approval-shell-marker")

	filePath := filepath.Join(dataDir, "approval-file.txt")
	mcpTerm(t, service.BaseURL(), mcpSession, map[string]any{
		"cmd":  "write " + session.SessionID + " file $1 --data $2",
		"args": []any{filePath, "approved content"},
	})
	fileApproval := firstApproval(t, service.BaseURL(), "file")
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("MCP file write happened before approval: %v", err)
	}
	checkStatus(t, request(t, http.MethodPost, service.BaseURL()+"/api/approvals/"+fileApproval.ID+"/approve", strings.NewReader(`{"reason":"desktop integration test"}`)), http.StatusOK).Body.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		content, err := os.ReadFile(filePath)
		if err == nil && string(content) == "approved content" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("approved file write did not execute: %q, %v", content, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
	checkStatus(t, request(t, http.MethodGet, service.BaseURL()+"/api/approvals/"+fileApproval.ID, nil), http.StatusOK).Body.Close()

	checkStatus(t, request(t, http.MethodPatch, service.BaseURL()+"/api/sessions/"+session.SessionID+"/approval", strings.NewReader(`{"enabled":false}`)), http.StatusOK).Body.Close()
}

type pendingApproval struct {
	ID   string `json:"id"`
	Kind string `json:"kind"`
}

func firstApproval(t *testing.T, baseURL, kind string) pendingApproval {
	t.Helper()
	resp := checkStatus(t, request(t, http.MethodGet, baseURL+"/api/approvals", nil), http.StatusOK)
	defer resp.Body.Close()
	var listed struct {
		Approvals []struct {
			Request pendingApproval `json:"request"`
		} `json:"approvals"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	for _, row := range listed.Approvals {
		approval := row.Request
		if approval.ID != "" && strings.Contains(strings.ToLower(approval.Kind), strings.ToLower(kind)) {
			return approval
		}
	}
	t.Fatalf("approval kind %q not found in %+v", kind, listed.Approvals)
	return pendingApproval{}
}

func waitForOutput(t *testing.T, baseURL, shellID, marker string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for {
		resp := checkStatus(t, request(t, http.MethodGet, baseURL+"/api/shells/"+shellID+"/output-range?tail=1&max=4096", nil), http.StatusOK)
		data, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if strings.Contains(string(data), marker) {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("shell output did not contain %q: %s", marker, data)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func initializeDesktopMCP(t *testing.T, baseURL string) string {
	t.Helper()
	body := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","clientInfo":{"name":"desktop-e2e","version":"1"}}}`
	req := request(t, http.MethodPost, baseURL+"/stream", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	resp := checkStatus(t, req, http.StatusOK)
	_, _ = io.Copy(io.Discard, resp.Body)
	_ = resp.Body.Close()
	sessionID := resp.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		t.Fatal("MCP initialize returned no session id")
	}
	return sessionID
}

func mcpTerm(t *testing.T, baseURL, sessionID string, arguments map[string]any) string {
	t.Helper()
	payload, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "term",
			"arguments": arguments,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	req := request(t, http.MethodPost, baseURL+"/stream", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("Mcp-Session-Id", sessionID)
	resp := checkStatus(t, req, http.StatusOK)
	data, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	raw := string(data)
	idx := strings.Index(raw, `{"jsonrpc"`)
	if idx < 0 {
		t.Fatalf("MCP term response has no JSON-RPC object: %s", raw)
	}
	var result struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error any `json:"error"`
	}
	if err := json.Unmarshal([]byte(raw[idx:]), &result); err != nil {
		t.Fatalf("decode MCP term response: %v\n%s", err, raw)
	}
	if result.Error != nil || result.Result.IsError {
		t.Fatalf("MCP term failed: %s", raw)
	}
	if len(result.Result.Content) == 0 {
		t.Fatalf("MCP term returned no content: %s", raw)
	}
	return result.Result.Content[0].Text
}

func request(t *testing.T, method, endpoint string, body io.Reader) *http.Request {
	t.Helper()
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		t.Fatal(err)
	}
	return req
}

func checkStatus(t *testing.T, req *http.Request, want int) *http.Response {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != want {
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("%s %s = %d, want %d: %s", req.Method, req.URL, resp.StatusCode, want, data)
	}
	return resp
}
