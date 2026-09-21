package main

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
)

func TestArgumentValue(t *testing.T) {
	previous := os.Args
	os.Args = []string{"Termcp", "--system-service-action", "install", "--autostart"}
	t.Cleanup(func() { os.Args = previous })
	if value, ok := argumentValue("--system-service-action"); !ok || value != "install" {
		t.Fatalf("argument value = %q, %v", value, ok)
	}
	if _, ok := argumentValue("--autostart"); ok {
		t.Fatal("flag without a value was treated as a value argument")
	}
}

func TestConfigureTermcpDataDirUsesHomeDirectory(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	if err := configureTermcpDataDir(""); err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := os.Getenv("TERMCP_DATA_DIR"), filepath.Join(home, ".termcp"); got != want {
		t.Fatalf("TERMCP_DATA_DIR = %q, want %q", got, want)
	}
}

func TestAPIProxiesTextJSONAndBinary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/sessions":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sessions":[]}`))
		case "/api/example":
			if r.Method != http.MethodPatch {
				t.Fatalf("method = %s", r.Method)
			}
			body, _ := io.ReadAll(r.Body)
			if string(body) != `{"name":"updated"}` {
				t.Fatalf("body = %q", body)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true}`))
		case "/api/image":
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write([]byte{0x89, 'P', 'N', 'G'})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	service := serviceForURL(t, server.URL)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	app := newApp(service)

	jsonResponse, err := app.API(model.APIRequest{Method: "PATCH", Path: "/api/example", Body: `{"name":"updated"}`, ContentType: "application/json"})
	if err != nil {
		t.Fatal(err)
	}
	if jsonResponse.Body != `{"ok":true}` || jsonResponse.Status != http.StatusOK {
		t.Fatalf("unexpected JSON response: %+v", jsonResponse)
	}
	binaryResponse, err := app.API(model.APIRequest{Method: "GET", Path: "/api/image"})
	if err != nil {
		t.Fatal(err)
	}
	if binaryResponse.Base64 != "iVBORw==" || binaryResponse.Body != "" {
		t.Fatalf("unexpected binary response: %+v", binaryResponse)
	}
}

func TestAPIRejectsRequestsOutsideCoreAPI(t *testing.T) {
	app := NewApp()
	for _, candidate := range []string{"https://example.test/api/sessions", "/stream", "api/sessions"} {
		if _, err := app.API(model.APIRequest{Method: "GET", Path: candidate}); err == nil {
			t.Fatalf("API accepted %q", candidate)
		}
	}
	if _, err := app.API(model.APIRequest{Method: "TRACE", Path: "/api/sessions"}); err == nil {
		t.Fatal("API accepted TRACE")
	}
}

func TestGetSnapshotAggregatesCoreResources(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/connections":
			_, _ = w.Write([]byte(`{"connections":[{"name":"internal","kind":"internal"}]}`))
		case "/api/sessions":
			_, _ = w.Write([]byte(`{"sessions":[{"id":"session-test","name":"workspace","status":"running","mode":"pty","ssh_endpoint":"internal"}]}`))
		case "/api/sessions/session-test/shells":
			_, _ = w.Write([]byte(`{"shells":[{"id":"shell-test","name":"zsh","status":"running","mode":"pty"}]}`))
		case "/api/history":
			_, _ = w.Write([]byte(`{"sessions":[{"id":"history-test","name":"previous","status":"archived"}]}`))
		case "/api/forwards":
			_, _ = w.Write([]byte(`{"forwards":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := serviceForURL(t, server.URL)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	snapshot, err := newApp(service).GetSnapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Core.Managed {
		t.Fatal("mock server should be reported as an externally managed Core")
	}
	if len(snapshot.Connections) != 1 || len(snapshot.Sessions) != 1 || len(snapshot.Sessions[0].Shells) != 1 || len(snapshot.History) != 1 {
		data, _ := json.Marshal(snapshot)
		t.Fatalf("incomplete snapshot: %s", data)
	}
}

func serviceForURL(t *testing.T, rawURL string) *corepkg.Service {
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
	return corepkg.New(host, port)
}
