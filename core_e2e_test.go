package main

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"testing"

	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
)

func TestIntegratedCoreResourceLifecycle(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("TERMCP_DATA_DIR", dataDir)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	service := corepkg.New("127.0.0.1", port)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	app := newApp(service)

	assertAPIStatus(t, app, "GET", "/api/connections", nil, http.StatusOK)
	assertAPIStatus(t, app, "GET", "/api/connection-templates", nil, http.StatusOK)
	profile := "kind = \"internal\"\ndescription = \"integration profile\"\n"
	assertAPIStatus(t, app, "POST", "/api/connections/test", profile, http.StatusOK, "text/plain")
	assertAPIStatus(t, app, "PUT", "/api/connections/integration", profile, http.StatusNoContent, "text/plain")
	assertAPIStatus(t, app, "GET", "/api/connections/integration", nil, http.StatusOK)

	created := assertAPIStatus(t, app, "POST", "/api/sessions", map[string]any{
		"ssh_config": "integration",
		"name":       "GUI integration",
		"command":    "sh",
		"mode":       "pty",
		"rows":       24,
		"cols":       80,
	}, http.StatusOK)
	var session struct {
		SessionID string `json:"session_id"`
		ShellID   string `json:"shell_id"`
	}
	decodeResponse(t, created, &session)
	if session.SessionID == "" || session.ShellID == "" {
		t.Fatalf("missing resource ids: %+v", session)
	}
	assertAPIStatus(t, app, "GET", "/api/sessions/"+url.PathEscape(session.SessionID)+"/shells", nil, http.StatusOK)
	assertAPIStatus(t, app, "GET", "/api/shells/"+url.PathEscape(session.ShellID)+"/output-range?tail=1&max=1024", nil, http.StatusOK)
	assertAPIStatus(t, app, "PATCH", "/api/sessions/"+url.PathEscape(session.SessionID), map[string]any{"name": "GUI renamed"}, http.StatusOK)

	remoteDir := filepath.Join(dataDir, "e2e-files")
	filePath := filepath.Join(remoteDir, "hello.txt")
	renamedPath := filepath.Join(remoteDir, "renamed.txt")
	base := "/api/sessions/" + url.PathEscape(session.SessionID) + "/files"
	assertAPIStatus(t, app, "POST", base+"/dir?path="+url.QueryEscape(remoteDir), nil, http.StatusOK)
	assertAPIStatus(t, app, "POST", base+"/upload?path="+url.QueryEscape(filePath), "hello from gui", http.StatusOK, "application/octet-stream")
	assertAPIStatus(t, app, "GET", base+"?path="+url.QueryEscape(remoteDir), nil, http.StatusOK)
	assertAPIStatus(t, app, "PUT", base+"?from="+url.QueryEscape(filePath)+"&to="+url.QueryEscape(renamedPath), nil, http.StatusOK)
	download := assertAPIStatus(t, app, "GET", base+"/download?path="+url.QueryEscape(renamedPath), nil, http.StatusOK)
	downloaded := download.Body
	if download.Base64 != "" {
		decoded, err := base64.StdEncoding.DecodeString(download.Base64)
		if err != nil {
			t.Fatal(err)
		}
		downloaded = string(decoded)
	}
	if downloaded != "hello from gui" {
		t.Fatalf("download body = %q (content-type %q)", downloaded, download.ContentType)
	}
	assertAPIStatus(t, app, "DELETE", base+"?path="+url.QueryEscape(renamedPath), nil, http.StatusOK)

	forward := assertAPIStatus(t, app, "POST", "/api/sessions/"+url.PathEscape(session.SessionID)+"/forwards", map[string]any{
		"direction":  "dynamic",
		"local_port": 0,
	}, http.StatusCreated)
	var forwardInfo struct {
		ID string `json:"forward_id"`
	}
	decodeResponse(t, forward, &forwardInfo)
	if forwardInfo.ID == "" {
		t.Fatal("missing forward id")
	}
	assertAPIStatus(t, app, "GET", "/api/sessions/"+url.PathEscape(session.SessionID)+"/forwards", nil, http.StatusOK)
	assertAPIStatus(t, app, "DELETE", "/api/forwards/"+url.PathEscape(forwardInfo.ID), nil, http.StatusOK)
	assertAPIStatus(t, app, "GET", "/api/notifications?session_id="+url.QueryEscape(session.SessionID), nil, http.StatusOK)

	assertAPIStatus(t, app, "POST", "/api/sessions/"+url.PathEscape(session.SessionID)+"/terminate", nil, http.StatusNoContent)
	assertAPIStatus(t, app, "GET", "/api/history", nil, http.StatusOK)
	assertAPIStatus(t, app, "PATCH", "/api/history/"+url.PathEscape(session.SessionID), map[string]any{"notes": "verified", "tags": []string{"e2e"}}, http.StatusOK)
	assertAPIStatus(t, app, "GET", "/api/history/"+url.PathEscape(session.SessionID)+"/transcript?format=text", nil, http.StatusOK)
	screenshot := assertAPIStatus(t, app, "GET", "/api/history/"+url.PathEscape(session.SessionID)+"/screenshot?lines=10&cols=80&theme=dark", nil, http.StatusOK)
	if screenshot.Base64 == "" {
		t.Fatal("history screenshot was not returned as binary data")
	}
	assertAPIStatus(t, app, "DELETE", "/api/history/"+url.PathEscape(session.SessionID), nil, http.StatusNoContent)
	assertAPIStatus(t, app, "DELETE", "/api/connections/integration", nil, http.StatusNoContent)
}

func assertAPIStatus(t *testing.T, app *App, method, path string, body any, want int, contentTypes ...string) model.APIResponse {
	t.Helper()
	var raw string
	contentType := ""
	if body != nil {
		if text, ok := body.(string); ok {
			raw = text
		} else {
			data, err := json.Marshal(body)
			if err != nil {
				t.Fatal(err)
			}
			raw = string(data)
			contentType = "application/json"
		}
	}
	if len(contentTypes) > 0 {
		contentType = contentTypes[0]
	}
	response, err := app.API(model.APIRequest{Method: method, Path: path, Body: raw, ContentType: contentType})
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	if response.Status != want {
		t.Fatalf("%s %s status = %d, want %d, body=%q", method, path, response.Status, want, response.Body)
	}
	return response
}

func decodeResponse(t *testing.T, response model.APIResponse, target any) {
	t.Helper()
	if err := json.Unmarshal([]byte(response.Body), target); err != nil {
		t.Fatalf("decode %q: %v", response.Body, err)
	}
}
