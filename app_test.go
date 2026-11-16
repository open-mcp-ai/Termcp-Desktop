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
	"strings"
	"testing"
	"time"

	guiConfig "github.com/open-mcp-ai/termcp/gui/internal/config"
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

func TestBuiltInHTTPMCPSkillPointsAtLiveCoreSurfaces(t *testing.T) {
	for _, text := range []string{"GET /api.md", "POST /api/term", "help <directive>", "/sse", "/stream"} {
		if !strings.Contains(termcpSkillMarkdown, text) {
			t.Fatalf("embedded HTTP skill is missing %q", text)
		}
	}
	if strings.Contains(termcpSkillMarkdown, "skills.md") {
		t.Fatal("embedded HTTP skill links to the removed /skills.md asset")
	}
}

func TestAPIProxiesTextJSONAndBinary(t *testing.T) {
	server := readyCoreTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	if err := service.Attach(2 * time.Second); err != nil {
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

func TestDefaultDirectoryUsesCoreGetwdForRemoteAndHomeForInternal(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	legacyRouteCalls := 0
	server := readyCoreTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/sessions/remote":
			_, _ = w.Write([]byte(`{"id":"remote","status":"running","ssh_endpoint":"remote"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/sessions/internal":
			_, _ = w.Write([]byte(`{"id":"internal","status":"running","ssh_endpoint":"internal"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/term":
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "get remote file . --op getwd") {
				t.Errorf("getwd command = %s", body)
			}
			_, _ = w.Write([]byte(`{"protocol":"TERM/1","code":200,"body":"directory: /srv/workspace"}`))
		case strings.Contains(r.URL.Path, "/files/default-directory"):
			legacyRouteCalls++
			http.NotFound(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	service := serviceForURL(t, server.URL)
	if err := service.Attach(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	app := newApp(service)

	remote, err := app.DefaultDirectory("remote")
	if err != nil || remote != "/srv/workspace" {
		t.Fatalf("remote default directory = %q, %v", remote, err)
	}
	internal, err := app.DefaultDirectory("internal")
	if err != nil {
		t.Fatal(err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	if internal != home {
		t.Fatalf("internal default directory = %q, want Home %q", internal, home)
	}
	if legacyRouteCalls != 0 {
		t.Fatalf("Desktop called the removed default-directory route %d times", legacyRouteCalls)
	}
}

func TestGetSnapshotAggregatesCoreResources(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	server := readyCoreTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/connections":
			_, _ = w.Write([]byte(`{"connections":[{"name":"internal","kind":"internal"}]}`))
		case "/api/extensions":
			_, _ = w.Write([]byte(`{"extensions":[{"name":"docs","kind":"skill","target":"internal","enabled":true,"status":"idle"}]}`))
		case "/api/sessions":
			_, _ = w.Write([]byte(`{"sessions":[{"id":"session-test","name":"workspace","status":"running","ssh_endpoint":"internal","created_at":1789990000000},{"id":"history-test","name":"previous","status":"exited","exit_reason":"SSH connection lost","updated_at":1789990001000},{"id":"extension-test","name":"ext:internal","status":"running","namespace":"ext:internal"},{"id":"extension-archive","name":"ext:old","status":"exited","namespace":"ext:old"}]}`))
		case "/api/sessions/session-test/shells":
			_, _ = w.Write([]byte(`{"shells":[{"id":"shell-test","name":"zsh","status":"running","mode":"pty"}]}`))
		case "/api/sessions/history-test/shells":
			_, _ = w.Write([]byte(`{"shells":[{"id":"old-shell","name":"shell","status":"exited","mode":"pipe"}]}`))
		case "/api/forwards":
			_, _ = w.Write([]byte(`{"forwards":[]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := serviceForURL(t, server.URL)
	if err := service.Attach(2 * time.Second); err != nil {
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
	if len(snapshot.Connections) != 1 || len(snapshot.Sessions) != 1 || len(snapshot.Sessions[0].Shells) != 1 || len(snapshot.History) != 1 || len(snapshot.History[0].Shells) != 1 || snapshot.History[0].UpdatedAt != 1789990001000 || snapshot.History[0].ExitReason != "SSH connection lost" || len(snapshot.Extensions) != 1 || snapshot.Extensions[0].Name != "docs" {
		data, _ := json.Marshal(snapshot)
		t.Fatalf("incomplete snapshot: %s", data)
	}
}

func TestGetActivityAndClientsAggregateCoreObservability(t *testing.T) {
	server := readyCoreTestServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/sessions":
			_, _ = w.Write([]byte(`{"sessions":[]}`))
		case "/api/activity":
			if got := r.URL.Query().Get("limit"); got != "2000" {
				t.Errorf("limit = %q, want clamped 2000", got)
			}
			if got := r.URL.Query().Get("session_id"); got != "session-one" {
				t.Errorf("session_id = %q", got)
			}
			_, _ = w.Write([]byte(`{"activity":[{"time":"2026-10-27T08:00:00Z","unix_ms":1,"session_id":"session-one","shell_id":"shell-one","session_name":"one","source":"ai","kind":"a","verdict":"input","bytes":42}]}`))
		case "/api/clients":
			_, _ = w.Write([]byte(`{"webui":[{"id":"1","addr":"127.0.0.1","watching":1,"since":"2026-10-27T08:00:00Z"}],"mcp":[{"session_id":"mcp-one","name":"agent","version":"1","namespace":"ns:agent","connected_at":"2026-10-27T08:00:00Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	service := serviceForURL(t, server.URL)
	if err := service.Attach(2 * time.Second); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	app := newApp(service)

	activity, err := app.GetActivity("session-one", 9000)
	if err != nil {
		t.Fatal(err)
	}
	if len(activity) != 1 || activity[0].ShellID != "shell-one" || activity[0].Bytes != 42 {
		t.Fatalf("activity = %+v", activity)
	}
	clients, err := app.GetClients()
	if err != nil {
		t.Fatal(err)
	}
	if len(clients.WebUI) != 1 || len(clients.MCP) != 1 || clients.MCP[0].Namespace != "ns:agent" {
		t.Fatalf("clients = %+v", clients)
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

// readyCoreTestServer fills only the readiness probes a mock handler leaves at
// 404. Tests keep ownership of every route they define while Attach still runs
// the same version and interface checks as a real system service.
func readyCoreTestServer(t *testing.T, next http.Handler) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Termcp-Desktop-Ready") == "1" {
			writeReadyProbe(w, r)
			return
		}
		recorder := httptest.NewRecorder()
		next.ServeHTTP(recorder, r)
		if recorder.Code != http.StatusNotFound {
			for key, values := range recorder.Header() {
				w.Header()[key] = append([]string(nil), values...)
			}
			w.WriteHeader(recorder.Code)
			_, _ = w.Write(recorder.Body.Bytes())
			return
		}
		writeReadyProbe(w, r)
	}))
	t.Cleanup(server.Close)
	return server
}

func writeReadyProbe(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/api/version":
		_, _ = w.Write([]byte(`{"version":"v` + guiConfig.CoreVersion + `"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/ui/ws":
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"Upgrade required"}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api.md":
		w.Header().Set("Content-Type", "text/markdown")
		_, _ = w.Write([]byte("# TERM/1 API\n"))
	case r.Method == http.MethodPost && r.URL.Path == "/api/term":
		_, _ = w.Write([]byte(`{"protocol":"TERM/1","code":200}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/connections":
		_, _ = w.Write([]byte(`{"connections":[]}`))
	case r.Method == http.MethodGet && r.URL.Path == "/api/sessions":
		_, _ = w.Write([]byte(`{"sessions":[]}`))
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
}
