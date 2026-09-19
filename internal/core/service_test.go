package core

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestServiceStartsIntegratedCore(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	service := New("127.0.0.1", port)
	if err := service.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() { _ = service.Stop() })

	status := service.Status()
	if !status.Running || !status.Managed {
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
		t.Fatalf("integrated Core did not expose the internal connection: %+v", body.Connections)
	}
}

func TestServiceAttachesToExistingCore(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/sessions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[]}`))
	}))
	defer server.Close()

	u, err := url.Parse(server.URL)
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
	service := New(host, port)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	if status := service.Status(); !status.Running || status.Managed {
		t.Fatalf("expected external Core attachment, got %+v", status)
	}
	if err := service.Stop(); err != nil {
		t.Fatal(err)
	}
	if _, err := http.Get(server.URL + "/api/sessions"); err != nil {
		t.Fatalf("stopping the GUI service stopped the external Core: %v", err)
	}
}
