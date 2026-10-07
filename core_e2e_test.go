package main

import (
	"encoding/base64"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	guiConfig "github.com/open-mcp-ai/termcp/gui/internal/config"
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
	var version struct {
		Version string `json:"version"`
	}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/version", nil, http.StatusOK), &version)
	if want := "v" + guiConfig.CoreVersion; version.Version != want {
		t.Fatalf("embedded Core version = %q, want %q", version.Version, want)
	}
	var daemon struct {
		Daemon    bool   `json:"daemon"`
		Version   string `json:"version"`
		StartedAt string `json:"started_at"`
	}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/daemon", nil, http.StatusOK), &daemon)
	if daemon.Daemon || daemon.Version != version.Version || daemon.StartedAt == "" {
		t.Fatalf("embedded Core daemon metadata = %+v", daemon)
	}
	if snapshot, err := app.GetSnapshot(); err != nil || snapshot.Core.Version != version.Version {
		t.Fatalf("snapshot Core version = %q, error = %v", snapshot.Core.Version, err)
	}

	assertAPIStatus(t, app, "GET", "/api/connections", nil, http.StatusOK)
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
	activitySocket, _, err := websocket.DefaultDialer.Dial("ws://127.0.0.1:"+strconv.Itoa(port)+"/api/ui/ws", nil)
	if err != nil {
		t.Fatalf("connect to Core UI WebSocket: %v", err)
	}
	defer activitySocket.Close()
	if err := activitySocket.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var firstEvent struct {
		Type string `json:"type"`
	}
	if err := activitySocket.ReadJSON(&firstEvent); err != nil || firstEvent.Type != "sessions" {
		t.Fatalf("Core WebSocket initial event = %+v, %v", firstEvent, err)
	}
	inputPath := "/api/shells/" + url.PathEscape(session.ShellID) + "/input"
	marksPath := "/api/shells/" + url.PathEscape(session.ShellID) + "/marks"
	assertAPIStatus(t, app, "POST", inputPath, map[string]any{"text": "echo ", "press_enter": false}, http.StatusOK)
	assertAPIStatus(t, app, "POST", inputPath, map[string]any{"text": "indexed-", "press_enter": false}, http.StatusOK)
	var partialMarks struct {
		Marks []model.HistorySpan `json:"marks"`
	}
	decodeResponse(t, assertAPIStatus(t, app, "GET", marksPath, nil, http.StatusOK), &partialMarks)
	for _, mark := range partialMarks.Marks {
		if mark.Status == "i" {
			t.Fatal("partial command created an input mark before submission")
		}
	}
	assertAPIStatus(t, app, "POST", inputPath, map[string]any{"text": "history", "press_enter": true}, http.StatusOK)
	for index, submit := range []bool{false, false, true} {
		var event struct {
			Type    string `json:"type"`
			ShellID string `json:"shell_id"`
			Source  string `json:"src"`
			Submit  bool   `json:"submit"`
		}
		for {
			if err := activitySocket.ReadJSON(&event); err != nil {
				t.Fatalf("read shell activity event %d: %v", index, err)
			}
			if event.Type == "shell_activity" {
				break
			}
		}
		if event.ShellID != session.ShellID || event.Source != "api" || event.Submit != submit {
			t.Fatalf("shell activity event %d = %+v, want shell=%q src=api submit=%t", index, event, session.ShellID, submit)
		}
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		output := assertAPIStatus(t, app, "GET", "/api/shells/"+url.PathEscape(session.ShellID)+"/output-range?tail=1&max=4096", nil, http.StatusOK)
		if strings.Contains(output.Body, "indexed-history") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("streamed command did not reach terminal output")
		}
		time.Sleep(20 * time.Millisecond)
	}
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
	withForward, err := app.GetSnapshot()
	if err != nil || len(withForward.Forwards) != 1 || withForward.Forwards[0].ForwardID != forwardInfo.ID {
		t.Fatalf("forward missing from snapshot: %+v, %v", withForward.Forwards, err)
	}
	assertAPIStatus(t, app, "DELETE", "/api/forwards/"+url.PathEscape(forwardInfo.ID), nil, http.StatusOK)
	assertAPIStatus(t, app, "GET", "/api/notifications?session_id="+url.QueryEscape(session.SessionID), nil, http.StatusOK)

	assertAPIStatus(t, app, "POST", "/api/sessions/"+url.PathEscape(session.SessionID)+"/terminate", nil, http.StatusNoContent)
	snapshot, err := app.GetSnapshot()
	if err != nil || len(snapshot.History) != 1 || snapshot.History[0].ID != session.SessionID {
		t.Fatalf("terminated session missing from history: %+v, %v", snapshot.History, err)
	}
	spans, err := app.GetConversationIndex(session.SessionID, session.ShellID)
	if err != nil || len(spans) == 0 {
		t.Fatalf("conversation index missing: %+v, %v", spans, err)
	}
	var coreMarks struct {
		Marks     []model.HistorySpan `json:"marks"`
		SessionID string              `json:"session_id"`
	}
	decodeResponse(t, assertAPIStatus(t, app, "GET", marksPath, nil, http.StatusOK), &coreMarks)
	if coreMarks.SessionID != session.SessionID || !reflect.DeepEqual(spans, coreMarks.Marks) {
		t.Fatalf("GUI index differs from Core marks: session=%q, gui=%+v, core=%+v", coreMarks.SessionID, spans, coreMarks.Marks)
	}
	inputMarks := 0
	for _, mark := range spans {
		if mark.Status == "i" {
			inputMarks++
			if mark.End != mark.Start {
				t.Fatalf("input mark owns output bytes: %+v", mark)
			}
		}
	}
	if inputMarks != 1 {
		t.Fatalf("streamed command generated %d input marks, want one submitted-line mark", inputMarks)
	}
	assertAPIStatus(t, app, "GET", "/api/shells/"+url.PathEscape(session.ShellID)+"/output-range?start=0&max=4096", nil, http.StatusOK)
	assertAPIStatus(t, app, "DELETE", "/api/sessions/"+url.PathEscape(session.SessionID), nil, http.StatusNoContent)
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
