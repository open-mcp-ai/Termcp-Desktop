package logging

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
)

func TestRotatesBySizeAndRemovesExpiredLogs(t *testing.T) {
	dataDir := t.TempDir()
	logDir := filepath.Join(dataDir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatal(err)
	}
	oldPath := filepath.Join(logDir, "termcp-desktop-old.log")
	if err := os.WriteFile(oldPath, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.September, 21, 10, 0, 0, 0, time.Local)
	old := now.Add(-15 * 24 * time.Hour)
	if err := os.Chtimes(oldPath, old, old); err != nil {
		t.Fatal(err)
	}

	current := now
	runtime, err := Configure(Config{
		DataDir: dataDir, Component: "Termcp Desktop", Level: slog.LevelDebug,
		Retention: 14 * 24 * time.Hour, MaxBytes: 180, Now: func() time.Time { return current }, PID: 42,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Fatalf("expired log still exists: %v", err)
	}

	for index := 0; index < 8; index++ {
		runtime.Logger.Debug("interface completed", "operation", "API", "index", index)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) < 2 {
		t.Fatalf("expected size rotation, got %d file(s)", len(entries))
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), "termcp-desktop-2026-09-21-42") {
			t.Fatalf("unexpected log name %q", entry.Name())
		}
		info, err := entry.Info()
		if err != nil {
			t.Fatal(err)
		}
		// Windows does not model Unix permission bits, so only assert them elsewhere.
		if goruntime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
			t.Fatalf("log permissions = %o", info.Mode().Perm())
		}
	}
}

func TestRotatesWhenDateChanges(t *testing.T) {
	current := time.Date(2026, time.September, 21, 23, 59, 0, 0, time.UTC)
	runtime, err := Configure(Config{
		DataDir: t.TempDir(), Component: "core-service", Level: slog.LevelDebug,
		Now: func() time.Time { return current }, PID: 7,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close() })
	runtime.Logger.Info("before midnight")
	first := runtime.writer.path
	current = current.Add(2 * time.Minute)
	runtime.Logger.Info("after midnight")
	second := runtime.writer.path
	if first == second || !strings.Contains(second, "2026-09-22") {
		t.Fatalf("date rotation paths = %q, %q", first, second)
	}
}

func TestHTTPMiddlewareLogsMetadataWithoutQueryValues(t *testing.T) {
	runtime, err := Configure(Config{DataDir: t.TempDir(), Component: "core-service", Level: slog.LevelDebug})
	if err != nil {
		t.Fatal(err)
	}
	handler := HTTPMiddleware(http.HandlerFunc(func(response http.ResponseWriter, _ *http.Request) {
		response.WriteHeader(http.StatusCreated)
		_, _ = response.Write([]byte("ok"))
	}))
	request := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/sessions?token=secret-value&path=%2Fprivate", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	path := runtime.writer.path
	if err := runtime.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	contents := string(data)
	if !strings.Contains(contents, `"status":201`) || !strings.Contains(contents, `"query_keys":"path,token"`) {
		t.Fatalf("missing HTTP metadata: %s", contents)
	}
	if strings.Contains(contents, "secret-value") || strings.Contains(contents, "/private") {
		t.Fatalf("query value leaked into logs: %s", contents)
	}
}

func TestRedactRemovesCredentialsAndPrivateKeys(t *testing.T) {
	raw := "password = hunter2 token:abc123\n-----BEGIN OPENSSH PRIVATE KEY-----\nsensitive\n-----END OPENSSH PRIVATE KEY-----"
	redacted := Redact(raw)
	for _, secret := range []string{"hunter2", "abc123", "sensitive"} {
		if strings.Contains(redacted, secret) {
			t.Fatalf("secret %q leaked: %s", secret, redacted)
		}
	}
}
