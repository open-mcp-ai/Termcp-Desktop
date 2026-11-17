package webui

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/configcrypt"
	"github.com/open-mcp-ai/termcp/internal/sshconfig"
)

// exportImportEnv is a Handler with a store holding one profile, plus a second
// empty store to import into — the actual round trip an operator performs
// between two machines.
func exportImportEnv(t *testing.T) (exportMux, importMux *http.ServeMux) {
	t.Helper()
	src := sshconfig.NewStore(t.TempDir())
	h := &Handler{SSH: src}
	exportMux = http.NewServeMux()
	h.Register(exportMux)

	profile := "kind = \"remote\"\nhost = \"sample.example\"\nuser = \"tester\"\npassword = \"placeholder\"\n"
	put := httptest.NewRecorder()
	exportMux.ServeHTTP(put, httptest.NewRequest(http.MethodPut, "/api/connections/prod", strings.NewReader(profile)))
	if put.Code != http.StatusNoContent {
		t.Fatalf("seed PUT: %d %s", put.Code, put.Body.String())
	}

	dst := sshconfig.NewStore(t.TempDir())
	h2 := &Handler{SSH: dst}
	importMux = http.NewServeMux()
	h2.Register(importMux)
	return exportMux, importMux
}

// The whole point of issue #108's third item: carry the configuration to another
// machine protected by a password the operator types. This drives the real
// routes, because the parts that can go wrong are at the boundary — where the
// password is read from and what a half-configured import does.
func TestEncryptedExportImportRoundTrip(t *testing.T) {
	exportMux, importMux := exportImportEnv(t)
	const password = "a password with spaces"

	req := httptest.NewRequest(http.MethodGet, "/api/connections/batch", nil)
	req.Header.Set(EncryptHeader, password)
	export := httptest.NewRecorder()
	exportMux.ServeHTTP(export, req)
	if export.Code != http.StatusOK {
		t.Fatalf("encrypted export: %d %s", export.Code, export.Body.String())
	}
	// The export is recognisable as encrypted both by its bytes and by the
	// headers a browser saves it with.
	if !configcrypt.IsEncrypted(export.Body.Bytes()) {
		t.Fatal("the export does not carry the envelope header")
	}
	if ct := export.Header().Get("Content-Type"); !strings.Contains(ct, "encrypted") {
		t.Errorf("content-type = %q, want the encrypted type", ct)
	}
	if cd := export.Header().Get("Content-Disposition"); !strings.Contains(cd, ".enc") {
		t.Errorf("content-disposition = %q, want a .enc filename", cd)
	}
	// The profile's password must not be readable in the body.
	if strings.Contains(export.Body.String(), "placeholder") {
		t.Fatal("the encrypted export leaks the profile password")
	}

	// Import it into a clean store with the same password.
	imp := httptest.NewRequest(http.MethodPost, "/api/connections/batch", strings.NewReader(export.Body.String()))
	imp.Header.Set(EncryptHeader, password)
	imported := httptest.NewRecorder()
	importMux.ServeHTTP(imported, imp)
	if imported.Code != http.StatusCreated {
		t.Fatalf("encrypted import: %d %s", imported.Code, imported.Body.String())
	}
	// The import reply is a count, not a list; one profile must have arrived.
	if !strings.Contains(imported.Body.String(), `"imported":1`) {
		t.Errorf("the profile did not arrive: %s", imported.Body.String())
	}
}

// A wrong password must be refused, and the refusal must not be mistaken for a
// malformed document — the operator needs to know which mistake they made.
func TestEncryptedImportRefusesAWrongPassword(t *testing.T) {
	exportMux, importMux := exportImportEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/connections/batch", nil)
	req.Header.Set(EncryptHeader, "right")
	export := httptest.NewRecorder()
	exportMux.ServeHTTP(export, req)

	imp := httptest.NewRequest(http.MethodPost, "/api/connections/batch", strings.NewReader(export.Body.String()))
	imp.Header.Set(EncryptHeader, "wrong")
	out := httptest.NewRecorder()
	importMux.ServeHTTP(out, imp)
	if out.Code != http.StatusBadRequest {
		t.Fatalf("wrong password = %d, want 400: %s", out.Code, out.Body.String())
	}
	if !strings.Contains(strings.ToLower(out.Body.String()), "password") {
		t.Errorf("the error must name the password as the problem: %s", out.Body.String())
	}
}

// Importing an encrypted file without the password must say so, rather than
// feeding ciphertext to the TOML parser and reporting a syntax error somewhere
// in a wall of base64.
func TestEncryptedImportWithoutPasswordSaysSo(t *testing.T) {
	exportMux, importMux := exportImportEnv(t)

	req := httptest.NewRequest(http.MethodGet, "/api/connections/batch", nil)
	req.Header.Set(EncryptHeader, "pw")
	export := httptest.NewRecorder()
	exportMux.ServeHTTP(export, req)

	imp := httptest.NewRequest(http.MethodPost, "/api/connections/batch", strings.NewReader(export.Body.String()))
	out := httptest.NewRecorder()
	importMux.ServeHTTP(out, imp)
	if out.Code != http.StatusUnauthorized {
		t.Fatalf("no password = %d, want 401: %s", out.Code, out.Body.String())
	}
	if !strings.Contains(out.Body.String(), EncryptHeader) {
		t.Errorf("the error must name the header to use: %s", out.Body.String())
	}
}

// The old path keeps working byte for byte: no password means the plain TOML
// export scripts have always parsed, and a plain import needs no password.
func TestPlainExportIsUnchanged(t *testing.T) {
	exportMux, importMux := exportImportEnv(t)

	export := httptest.NewRecorder()
	exportMux.ServeHTTP(export, httptest.NewRequest(http.MethodGet, "/api/connections/batch", nil))
	if export.Code != http.StatusOK {
		t.Fatalf("plain export: %d", export.Code)
	}
	if configcrypt.IsEncrypted(export.Body.Bytes()) {
		t.Fatal("an export with no password must stay plain TOML")
	}
	if !strings.Contains(export.Body.String(), `name = "prod"`) {
		t.Fatalf("plain export lost its content: %s", export.Body.String())
	}
	if cd := export.Header().Get("Content-Disposition"); !strings.Contains(cd, ".toml") {
		t.Errorf("content-disposition = %q, want the .toml filename", cd)
	}

	imp := httptest.NewRequest(http.MethodPost, "/api/connections/batch", strings.NewReader(export.Body.String()))
	out := httptest.NewRecorder()
	importMux.ServeHTTP(out, imp)
	if out.Code != http.StatusCreated {
		t.Fatalf("plain import = %d: %s", out.Code, out.Body.String())
	}
}

// The password must never be accepted from the query string. A URL lands in
// shell history, proxy logs and browser history; this asserts the route does not
// read one, so nobody can reintroduce the leak as a convenience.
func TestExportDoesNotTakeAPasswordFromTheQuery(t *testing.T) {
	exportMux, _ := exportImportEnv(t)
	out := httptest.NewRecorder()
	exportMux.ServeHTTP(out, httptest.NewRequest(http.MethodGet, "/api/connections/batch?password=leaked", nil))
	if configcrypt.IsEncrypted(out.Body.Bytes()) {
		t.Fatal("a password in the query string must be ignored; a URL is logged everywhere")
	}
}
