package core

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestServiceStartsIntegratedCore(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	port := freePort(t)

	service := New("127.0.0.1", port)
	if err := service.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	status := service.Status()
	if !status.Running || !status.Managed || status.State != "running" {
		t.Fatalf("unexpected status: %+v", status)
	}
	resp, err := http.Get(service.BaseURL() + "/api/connections")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var body struct {
		Connections []struct {
			Name string `json:"name"`
			Kind string `json:"kind"`
		} `json:"connections"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if len(body.Connections) == 0 || body.Connections[0].Name != "internal" {
		t.Fatalf("integrated Core did not expose the internal connection: %+v", body)
	}

	createResponse, err := http.Post(service.BaseURL()+"/api/sessions", "application/json", strings.NewReader(`{"ssh_config":"internal","name":"cwd-test","mode":"pty","rows":24,"cols":80}`))
	if err != nil {
		t.Fatal(err)
	}
	defer createResponse.Body.Close()
	if createResponse.StatusCode != http.StatusCreated && createResponse.StatusCode != http.StatusOK {
		t.Fatalf("create internal session returned %s", createResponse.Status)
	}
	var created struct {
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(createResponse.Body).Decode(&created); err != nil {
		t.Fatal(err)
	}

	term := postTerm(t, service.BaseURL(), map[string]any{
		"cmd": "get " + created.SessionID + " file . --op getwd",
	})
	if term.Code != http.StatusOK {
		t.Fatalf("getwd TERM code = %d, body = %s", term.Code, term.Body)
	}
	if !strings.Contains(term.Body, "directory:") {
		t.Fatalf("getwd reply has no directory: %s", term.Body)
	}

	var version struct {
		Version string `json:"version"`
	}
	resp, err = http.Get(service.BaseURL() + "/api/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(&version); err != nil {
		t.Fatal(err)
	}
	if version.Version != expectedVersion() {
		t.Fatalf("Core version = %q, want %q", version.Version, expectedVersion())
	}
}

func TestServiceAttachDoesNotOwnExistingCore(t *testing.T) {
	server := readyMockCore(t)
	service := serviceForURL(t, server.URL)
	if err := service.Attach(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	status := service.Status()
	if !status.Running || status.Managed {
		t.Fatalf("expected an unmanaged attachment, got %+v", status)
	}
	if err := service.Stop(); err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get(server.URL + "/api/version")
	if err != nil {
		t.Fatalf("detaching the GUI stopped the external Core: %v", err)
	}
	_ = resp.Body.Close()
}

func TestAttachRejectsWrongVersion(t *testing.T) {
	server := readyMockCoreVersion(t, "v0.2.8")
	service := serviceForURL(t, server.URL)
	err := service.Attach(250 * time.Millisecond)
	if err == nil || !strings.Contains(err.Error(), expectedVersion()) {
		t.Fatalf("Attach error = %v, want required version %s", err, expectedVersion())
	}
	status := service.Status()
	if status.Running || status.Managed || status.State != "error" {
		t.Fatalf("wrong-version Attach changed ownership: %+v", status)
	}
}

func TestAttachWaitsForServiceWithoutStartingEmbeddedCore(t *testing.T) {
	port := freePort(t)
	service := New("127.0.0.1", port)
	if err := service.Attach(120 * time.Millisecond); err == nil {
		t.Fatal("Attach succeeded without a service endpoint")
	}
	status := service.Status()
	if status.Running || status.Managed || status.State != "error" {
		t.Fatalf("Attach started or reported an embedded Core: %+v", status)
	}
	listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("Attach left a listener behind: %v", err)
	}
	_ = listener.Close()
}

func TestServicePortConflictDoesNotAttachOrLeak(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	occupied, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := occupied.Addr().(*net.TCPAddr).Port
	service := New("127.0.0.1", port)
	err = service.Start()
	if err == nil || !strings.Contains(err.Error(), "address already in use") {
		t.Fatalf("Start with an occupied port = %v", err)
	}
	status := service.Status()
	if status.Running || status.Managed || status.State != "error" {
		t.Fatalf("port conflict reported a running Core: %+v", status)
	}
	if err := occupied.Close(); err != nil {
		t.Fatal(err)
	}
	if err := service.Start(); err != nil {
		t.Fatalf("Start after the port became free: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })
}

func TestServiceRestartClosesWebSocketAndReleasesPort(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	port := freePort(t)
	address := net.JoinHostPort("127.0.0.1", strconv.Itoa(port))
	service := New("127.0.0.1", port)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}

	conn, _, err := websocket.DefaultDialer.Dial("ws://"+address+"/api/ui/ws", nil)
	if err != nil {
		t.Fatalf("connect websocket: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatal(err)
	}
	var event map[string]any
	if err := conn.ReadJSON(&event); err != nil {
		t.Fatalf("initial websocket event: %v", err)
	}

	if err := service.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("WebSocket remained open after Core stop")
	}
	_ = conn.Close()

	listener, err := net.Listen("tcp", address)
	if err != nil {
		t.Fatalf("port was not released after Stop: %v", err)
	}
	_ = listener.Close()

	if err := service.Start(); err != nil {
		t.Fatalf("restart: %v", err)
	}
	if err := service.Stop(); err != nil {
		t.Fatalf("second stop: %v", err)
	}
}

func TestServiceUsesFixedDesktopDataDirectory(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("TERMCP_DATA_DIR", dataDir)
	t.Setenv("TERMCP_ASSETS_DIR", filepath.Join(t.TempDir(), "untrusted-upstream-assets"))
	t.Setenv("TERMCP_HEADLESS", "1")
	service := New("127.0.0.1", freePort(t))
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	req, err := http.NewRequest(http.MethodPut, service.BaseURL()+"/api/connections/data-dir-probe", strings.NewReader("kind = \"remote\"\nhost = \"example.test\"\nuser = \"tester\"\npassword = \"test\"\n"))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "text/plain")
	checkStatus(t, req, http.StatusNoContent).Body.Close()
	if _, err := os.Stat(filepath.Join(dataDir, "ssh_configs", "data-dir-probe", "config.toml")); err != nil {
		t.Fatalf("Core did not use the explicit Desktop data directory: %v", err)
	}
	for _, path := range []string{"/", "/static/app.js", "/themes/default.css"} {
		resp, err := http.Get(service.BaseURL() + path)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want no upstream WebUI asset", path, resp.StatusCode)
		}
	}
}

func serviceForURL(t *testing.T, rawURL string) *Service {
	t.Helper()
	u, err := url.Parse(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	host, portText, err := net.SplitHostPort(u.Host)
	if err != nil {
		t.Fatal(err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatal(err)
	}
	return New(host, port)
}

func freePort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()
	return port
}

func postTerm(t *testing.T, baseURL string, args map[string]any) envelopeSlot {
	t.Helper()
	data, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post(baseURL+"/api/term", "application/json", strings.NewReader(string(data)))
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var slot envelopeSlot
	if err := json.NewDecoder(resp.Body).Decode(&slot); err != nil {
		t.Fatal(err)
	}
	return slot
}

type envelopeSlot struct {
	Code int    `json:"code"`
	Body string `json:"body"`
}

func readyMockCore(t *testing.T) *httptest.Server {
	return readyMockCoreVersion(t, expectedVersion())
}

func readyMockCoreVersion(t *testing.T, version string) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/version":
			_, _ = w.Write([]byte(`{"version":"` + version + `"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/ui/ws":
			w.WriteHeader(http.StatusBadRequest)
			_, _ = w.Write([]byte(`{"error":"Upgrade required"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api.md":
			w.Header().Set("Content-Type", "text/markdown")
			_, _ = w.Write([]byte("# TERM/1 API\n"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/term":
			_, _ = w.Write([]byte(`{"protocol":"TERM/1","code":200}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/sessions":
			_, _ = w.Write([]byte(`{"sessions":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/connections":
			_, _ = w.Write([]byte(`{"connections":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/extensions":
			_, _ = w.Write([]byte(`{"extensions":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/approvals":
			_, _ = w.Write([]byte(`{"approvals":[],"count":0}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/clients":
			_, _ = w.Write([]byte(`{"webui":[],"mcp":[]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/activity":
			_, _ = w.Write([]byte(`{"activity":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}
