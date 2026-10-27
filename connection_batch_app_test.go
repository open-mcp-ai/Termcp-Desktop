package main

import (
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/configcrypt"
	"github.com/open-mcp-ai/termcp/internal/storage"
	"github.com/open-mcp-ai/termcp/pkg/api"
)

func TestConnectionBatchExportAndPartialImport(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	plain := `[[connections]]
name = "good"
kind = "remote"
host = "good.example"
user = "tester"
password = "secret"

[[connections]]
name = "broken"
kind = "internal"
`
	sealed, err := configcrypt.Encrypt([]byte(plain), "archive-password")
	if err != nil {
		t.Fatal(err)
	}
	perHostHeaders := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/sessions" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"sessions":[]}`))
			return
		}
		if r.URL.Path == "/api/connections/batch" && r.Method == http.MethodGet {
			if got := r.URL.Query().Get("names"); got != "good" {
				t.Errorf("export names = %q", got)
			}
			password := r.Header.Get("X-Termcp-Password")
			if password == "" {
				w.Header().Set("Content-Type", "application/toml; charset=utf-8")
				_, _ = w.Write([]byte(plain))
				return
			}
			if password != "archive-password" {
				t.Errorf("export password header = %q", password)
			}
			data, _ := configcrypt.Encrypt([]byte(plain), "archive-password")
			w.Header().Set("Content-Type", "application/x-termcp-encrypted")
			_, _ = w.Write(data)
			return
		}
		if r.URL.Path != "/api/connections/batch" || r.Method != http.MethodPost {
			http.NotFound(w, r)
			return
		}
		if got := r.Header.Get("X-Termcp-Password"); got != "archive-password" && got != "wrong" {
			t.Errorf("import password header = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if configcrypt.IsEncrypted(body) {
			http.Error(w, "connection 2 must be remote", http.StatusBadRequest)
			return
		}
		perHostHeaders++
		if strings.Contains(string(body), `name = "broken"`) {
			http.Error(w, "connection broken must be remote", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"imported":1}`))
	}))
	defer server.Close()

	service := serviceForURL(t, server.URL)
	if err := service.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Stop() })
	app := newApp(service)

	exported, names, err := app.connectionExport([]string{"good", "internal"}, "archive-password")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 || names[0] != "good" || exported.Base64 == "" {
		t.Fatalf("encrypted export = %+v names=%v", exported, names)
	}
	exportBytes, err := base64.StdEncoding.DecodeString(exported.Base64)
	if err != nil || !configcrypt.IsEncrypted(exportBytes) {
		t.Fatalf("export is not encrypted: %v", err)
	}
	plainExport, _, err := app.connectionExport([]string{"good", "internal"}, "")
	if err != nil {
		t.Fatal(err)
	}
	if plainExport.Base64 != "" || !strings.Contains(plainExport.Body, `name = "good"`) {
		t.Fatalf("plain export = %+v", plainExport)
	}
	if _, _, err := app.connectionExport([]string{"internal"}, ""); err == nil {
		t.Fatal("internal profile-only export must be refused")
	}

	result, err := app.importConnectionsDocument("connections.enc", sealed, "archive-password")
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 1 || result.Failed != 1 || len(result.Hosts) != 2 {
		t.Fatalf("partial import = %+v", result)
	}
	if result.Hosts[0].Name != "good" || result.Hosts[0].Status != "imported" || result.Hosts[1].Name != "broken" || result.Hosts[1].Status != "failed" {
		t.Fatalf("per-host import results = %+v", result.Hosts)
	}
	if perHostHeaders != 2 {
		t.Fatalf("per-host import calls = %d, want 2", perHostHeaders)
	}

	if _, err := app.importConnectionsDocument("connections.enc", sealed, "wrong"); err == nil || !strings.Contains(err.Error(), "wrong password") {
		t.Fatalf("wrong password error = %v", err)
	}
}

func TestSnapshotIncludesArchivedShellFromPersistedManifest(t *testing.T) {
	dataDir := t.TempDir()
	t.Setenv("TERMCP_DATA_DIR", dataDir)
	store := storage.New(dataDir)
	if err := store.SaveSession(api.Session{ID: "live-session", Name: "Live", Status: api.SessionRunning, SSHEndpoint: "remote"}); err != nil {
		t.Fatal(err)
	}
	code := 0
	if err := store.SaveShell("live-session", api.Session{ID: "archived-shell", Name: "finished", Status: api.SessionExited, Mode: api.ModePTY, ExitCode: &code}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/connections":
			_, _ = w.Write([]byte(`{"connections":[]}`))
		case "/api/extensions":
			_, _ = w.Write([]byte(`{"extensions":[]}`))
		case "/api/sessions":
			_, _ = w.Write([]byte(`{"sessions":[{"id":"live-session","name":"Live","status":"running","ssh_endpoint":"remote"}]}`))
		case "/api/sessions/live-session/shells":
			_, _ = w.Write([]byte(`{"shells":[{"id":"live-shell","name":"active","status":"running","mode":"pty"}]}`))
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
	if len(snapshot.Sessions) != 1 || len(snapshot.Sessions[0].Shells) != 1 {
		t.Fatalf("live session = %+v", snapshot.Sessions)
	}
	if len(snapshot.History) != 1 || !snapshot.History[0].ArchivedShellsOnly || len(snapshot.History[0].Shells) != 1 {
		t.Fatalf("archived shell history = %+v", snapshot.History)
	}
	archived := snapshot.History[0].Shells[0]
	if archived.ID != "archived-shell" || !archived.Archived || archived.Status != "exited" {
		t.Fatalf("archived shell = %+v", archived)
	}
}

func TestArchiveShellUsesArchiveEndpointAndVerifiesRetainedRecord(t *testing.T) {
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
	archiveCalls := 0
	deleteCalls := 0
	retained := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/shells/shell-one/archive":
			archiveCalls++
			w.WriteHeader(http.StatusNoContent)
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/api/shells/"):
			deleteCalls++
			w.WriteHeader(http.StatusNoContent)
		case r.URL.Path == "/api/connections":
			_, _ = w.Write([]byte(`{"connections":[]}`))
		case r.URL.Path == "/api/extensions":
			_, _ = w.Write([]byte(`{"extensions":[]}`))
		case r.URL.Path == "/api/sessions":
			if retained {
				_, _ = w.Write([]byte(`{"sessions":[{"id":"history-session","name":"Archived","status":"exited","exit_reason":"closed on request"}]}`))
			} else {
				_, _ = w.Write([]byte(`{"sessions":[{"id":"live-session","name":"Live","status":"running"}]}`))
			}
		case r.URL.Path == "/api/sessions/live-session/shells":
			_, _ = w.Write([]byte(`{"shells":[{"id":"other-shell","name":"active","status":"running","mode":"pty"}]}`))
		case r.URL.Path == "/api/sessions/history-session/shells":
			_, _ = w.Write([]byte(`{"shells":[{"id":"shell-one","name":"finished","status":"exited","mode":"pty"}]}`))
		case r.URL.Path == "/api/forwards":
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
	app := newApp(service)

	if err := app.ArchiveShell("shell-one"); err == nil {
		t.Fatal("a 204 without a readable retained shell must not be reported as success")
	}
	retained = true
	if err := app.ArchiveShell("shell-one"); err != nil {
		t.Fatal(err)
	}
	if archiveCalls != 2 || deleteCalls != 0 {
		t.Fatalf("archive calls=%d delete calls=%d", archiveCalls, deleteCalls)
	}
}
