package core

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/open-mcp-ai/termcp/internal/approval"
)

func TestIntegratedApprovalExecutesFileAndForwardOperations(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("TERMCP_DATA_DIR", dataDir)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	service := New("127.0.0.1", port)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	profile := []byte("kind = \"internal\"\ndefault_approval = true\n")
	req, _ := http.NewRequest(http.MethodPut, service.BaseURL()+"/api/connections/internal", bytes.NewReader(profile))
	req.Header.Set("Content-Type", "text/plain")
	checkApprovalStatus(t, req, http.StatusNoContent)
	created := checkApprovalStatus(t, newApprovalRequest(t, http.MethodPost, service.BaseURL()+"/api/sessions", map[string]any{"ssh_config": "internal", "mode": "pty"}), http.StatusOK)
	var result struct {
		SessionID string `json:"session_id"`
		ShellID   string `json:"shell_id"`
	}
	if err := json.NewDecoder(created.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}
	_ = created.Body.Close()
	sess := service.sessions.Get(result.SessionID)
	if sess == nil || !sess.ApprovalEnabled() {
		t.Fatal("new session did not inherit connection approval")
	}
	shell := sess.GetChildShell(result.ShellID)
	if shell == nil {
		t.Fatal("new session has no primary shell")
	}
	if err := shell.StageForApproval("mcp", "echo approval-shell-marker"); err != nil {
		t.Fatal(err)
	}
	shellRequestID, err := shell.CommitStagedForApproval("mcp", "enter", 1)
	if err != nil {
		t.Fatal(err)
	}
	outputURL := service.BaseURL() + "/api/shells/" + result.ShellID + "/output-range?tail=1&max=4096"
	before := checkApprovalStatus(t, newApprovalRequest(t, http.MethodGet, outputURL, nil), http.StatusOK)
	beforeBytes, _ := io.ReadAll(before.Body)
	_ = before.Body.Close()
	if strings.Contains(string(beforeBytes), "approval-shell-marker") {
		t.Fatal("shell command ran before approval")
	}
	checkApprovalStatus(t, newApprovalRequest(t, http.MethodPost, service.BaseURL()+"/api/approvals/"+shellRequestID+"/approve", map[string]any{}), http.StatusOK).Body.Close()
	deadline := time.Now().Add(3 * time.Second)
	for {
		response := checkApprovalStatus(t, newApprovalRequest(t, http.MethodGet, outputURL, nil), http.StatusOK)
		data, _ := io.ReadAll(response.Body)
		_ = response.Body.Close()
		if strings.Contains(string(data), "approval-shell-marker") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("approved shell input did not reach the terminal")
		}
		time.Sleep(20 * time.Millisecond)
	}

	filePath := filepath.Join(dataDir, "approved.txt")
	filePayload, _ := json.Marshal(map[string]any{"tool": "file_write", "args": map[string]any{"remote_path": filePath, "data": "approved content", "mode": "text"}})
	fileID, err := sess.SubmitOperation(approval.Submission{Kind: approval.KindFileWrite, Source: "mcp", Summary: "write approved.txt", Payload: filePayload})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Fatalf("file write happened before approval: %v", err)
	}
	checkApprovalStatus(t, newApprovalRequest(t, http.MethodPost, service.BaseURL()+"/api/approvals/"+fileID+"/approve", map[string]any{}), http.StatusOK).Body.Close()
	content, err := os.ReadFile(filePath)
	if err != nil || string(content) != "approved content" {
		t.Fatalf("approved file write failed: %q, %v", content, err)
	}

	forwardPayload, _ := json.Marshal(map[string]any{"tool": "forward", "args": map[string]any{"action": "dynamic", "local_port": 0}})
	forwardID, err := sess.SubmitOperation(approval.Submission{Kind: approval.KindForwardOpen, Source: "mcp", Summary: "start SOCKS5 forward", Payload: forwardPayload})
	if err != nil {
		t.Fatal(err)
	}
	checkApprovalStatus(t, newApprovalRequest(t, http.MethodPost, service.BaseURL()+"/api/approvals/"+forwardID+"/approve", map[string]any{}), http.StatusOK).Body.Close()
	forwards := checkApprovalStatus(t, newApprovalRequest(t, http.MethodGet, service.BaseURL()+"/api/forwards", nil), http.StatusOK)
	var listed struct {
		Forwards []json.RawMessage `json:"forwards"`
	}
	if err := json.NewDecoder(forwards.Body).Decode(&listed); err != nil {
		t.Fatal(err)
	}
	_ = forwards.Body.Close()
	if len(listed.Forwards) != 1 {
		t.Fatalf("approved forward missing: %d", len(listed.Forwards))
	}

	checkApprovalStatus(t, newApprovalRequest(t, http.MethodPatch, service.BaseURL()+"/api/sessions/"+result.SessionID+"/approval", map[string]any{"enabled": false}), http.StatusOK).Body.Close()
	if sess.ApprovalEnabled() {
		t.Fatal("session approval was not disabled")
	}
}

func newApprovalRequest(t *testing.T, method, url string, body any) *http.Request {
	t.Helper()
	var data []byte
	if body != nil {
		data, _ = json.Marshal(body)
	}
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req
}

func checkApprovalStatus(t *testing.T, req *http.Request, want int) *http.Response {
	t.Helper()
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != want {
		defer resp.Body.Close()
		var body bytes.Buffer
		_, _ = body.ReadFrom(resp.Body)
		t.Fatalf("%s %s = %d, want %d: %s", req.Method, req.URL, resp.StatusCode, want, body.String())
	}
	return resp
}
