package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
)

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
