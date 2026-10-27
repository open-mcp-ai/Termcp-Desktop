package main

import (
	"net"
	"net/http"
	"reflect"
	"strings"
	"testing"

	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
)

type extensionListResponse struct {
	Extensions []extensionSummary `json:"extensions"`
}

type extensionSummary struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Target    string `json:"target"`
	Status    string `json:"status"`
	Enabled   bool   `json:"enabled"`
	Session   string `json:"session_id,omitempty"`
	Shell     string `json:"shell_id,omitempty"`
	Failure   any    `json:"failure,omitempty"`
	NextRetry int64  `json:"next_retry_ms,omitempty"`
}

type extensionDetailResponse struct {
	Extension struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"extension"`
	Kind     string           `json:"kind"`
	Target   string           `json:"target"`
	Mode     string           `json:"mode"`
	Command  string           `json:"command"`
	Notes    string           `json:"notes"`
	Allow    []string         `json:"allow"`
	Failures []map[string]any `json:"failures"`
}

func TestIntegratedExtensionAuthoringLifecycle(t *testing.T) {
	app := startExtensionTestApp(t)

	created := assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":        "draft",
		"kind":        "custom",
		"target":      "internal",
		"description": "first draft",
		"enabled":     false,
		"mode":        "pipe",
		"command":     "cat",
		"notes":       "keep this note",
	}, http.StatusCreated)
	var createdSummary struct {
		Extension extensionSummary `json:"extension"`
	}
	decodeResponse(t, created, &createdSummary)
	if createdSummary.Extension.Name != "draft" || createdSummary.Extension.Status != "disabled" {
		t.Fatalf("created extension = %+v", createdSummary.Extension)
	}

	updated := assertAPIStatus(t, app, "PUT", "/api/extensions/draft", map[string]any{
		"description": "patched",
	}, http.StatusOK)
	var updatedSummary struct {
		Extension extensionSummary `json:"extension"`
	}
	decodeResponse(t, updated, &updatedSummary)
	var detail extensionDetailResponse
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions/draft", nil, http.StatusOK), &detail)
	if detail.Command != "cat" || detail.Notes != "keep this note" || detail.Extension.Status != "disabled" {
		t.Fatalf("unsubmitted fields were not preserved: %+v", detail)
	}

	assertAPIStatus(t, app, "PUT", "/api/extensions/draft", map[string]any{
		"notes": "",
		"allow": []string{},
		"args":  []string{},
	}, http.StatusOK)
	detail = extensionDetailResponse{}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions/draft", nil, http.StatusOK), &detail)
	if detail.Notes != "" || len(detail.Allow) != 0 {
		t.Fatalf("explicit clear did not persist: %+v", detail)
	}

	fileView := "kind = \"custom\"\ntarget = \"internal\"\ndescription = \"edited as TOML\"\nenabled = false\n"
	assertAPIStatus(t, app, "PUT", "/api/extensions/draft", fileView, http.StatusOK, "application/toml")
	raw := assertAPIStatus(t, app, "GET", "/api/extensions/draft?format=toml", nil, http.StatusOK)
	if !strings.Contains(raw.Body, `description = "edited as TOML"`) {
		t.Fatalf("TOML view did not read the file that was written: %q", raw.Body)
	}

	invalid := "name = \"other\"\nkind = \"custom\"\ntarget = \"internal\"\n"
	assertAPIErrorStatus(t, app, "PUT", "/api/extensions/draft", invalid, http.StatusBadRequest, "application/toml")
	unchanged := assertAPIStatus(t, app, "GET", "/api/extensions/draft?format=toml", nil, http.StatusOK)
	if unchanged.Body != raw.Body {
		t.Fatalf("invalid TOML changed the stored file:\nbefore=%q\nafter=%q", raw.Body, unchanged.Body)
	}

	retry := assertAPIStatus(t, app, "POST", "/api/extensions/draft/retry", map[string]any{}, http.StatusOK)
	var retryResult struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	decodeResponse(t, retry, &retryResult)
	if retryResult.OK || !strings.Contains(retryResult.Error, "disabled") {
		t.Fatalf("disabled retry = %+v", retryResult)
	}

	assertAPIStatus(t, app, "DELETE", "/api/extensions/draft", nil, http.StatusNoContent)
	list := extensionListResponse{}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions", nil, http.StatusOK), &list)
	if len(list.Extensions) != 0 {
		t.Fatalf("deleted extension remains listed: %+v", list.Extensions)
	}
}

func TestIntegratedExtensionBatchRoundTrip(t *testing.T) {
	app := startExtensionTestApp(t)

	empty := assertAPIStatus(t, app, "GET", "/api/extensions/batch", nil, http.StatusOK)
	if empty.Status != http.StatusOK {
		t.Fatalf("empty export status = %d", empty.Status)
	}

	document := `[[extensions]]
name = "one"
kind = "custom"
target = "internal"
description = "first"
enabled = false

[[extensions]]
name = "two"
kind = "custom"
target = "internal"
path = "/tmp/unrelated"
enabled = false
`
	imported := assertAPIStatus(t, app, "POST", "/api/extensions/batch", document, http.StatusCreated, "application/toml")
	var firstImport struct {
		Imported int `json:"imported"`
		Renamed  []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"renamed"`
	}
	decodeResponse(t, imported, &firstImport)
	if firstImport.Imported != 2 || len(firstImport.Renamed) != 0 {
		t.Fatalf("first import = %+v", firstImport)
	}

	exported := assertAPIStatus(t, app, "GET", "/api/extensions/batch", nil, http.StatusOK)
	if !strings.Contains(exported.Body, `name = "one"`) || !strings.Contains(exported.Body, `name = "two"`) {
		t.Fatalf("export omitted imported templates: %s", exported.Body)
	}

	second := assertAPIStatus(t, app, "POST", "/api/extensions/batch", exported.Body, http.StatusCreated, "application/toml")
	var secondImport struct {
		Imported int `json:"imported"`
		Renamed  []struct {
			From string `json:"from"`
			To   string `json:"to"`
		} `json:"renamed"`
	}
	decodeResponse(t, second, &secondImport)
	if secondImport.Imported != 2 || len(secondImport.Renamed) != 2 {
		t.Fatalf("conflicting import = %+v", secondImport)
	}
	renamed := map[string]string{}
	for _, item := range secondImport.Renamed {
		renamed[item.From] = item.To
	}
	if renamed["one"] != "one-2" || renamed["two"] != "two-2" {
		t.Fatalf("renames = %+v", renamed)
	}

	before := extensionListResponse{}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions", nil, http.StatusOK), &before)
	invalid := `[[extensions]]
name = "valid-part"
kind = "custom"
target = "internal"
enabled = false

[[extensions]]
name = "invalid-part"
kind = "skill"
target = "internal"
`
	assertAPIErrorStatus(t, app, "POST", "/api/extensions/batch", invalid, http.StatusBadRequest, "application/toml")
	after := extensionListResponse{}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions", nil, http.StatusOK), &after)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("invalid batch partially changed the store:\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestIntegratedExtensionRetryRecordsFailureHistory(t *testing.T) {
	app := startExtensionTestApp(t)
	assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":    "broken",
		"kind":    "custom",
		"target":  "missing-profile",
		"mode":    "pipe",
		"command": "cat",
		"enabled": true,
	}, http.StatusCreated)

	retry := assertAPIStatus(t, app, "POST", "/api/extensions/broken/retry", map[string]any{}, http.StatusOK)
	var retryResult struct {
		OK    bool   `json:"ok"`
		Error string `json:"error"`
	}
	decodeResponse(t, retry, &retryResult)
	if !retryResult.OK {
		t.Fatalf("retry was not accepted: %+v", retryResult)
	}

	var detail extensionDetailResponse
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions/broken", nil, http.StatusOK), &detail)
	if len(detail.Failures) == 0 {
		t.Fatal("failed retry did not record failure history")
	}
	if detail.Extension.Status == "available" {
		t.Fatal("failed retry reported the extension as available")
	}
}

func startExtensionTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("TERMCP_DATA_DIR", t.TempDir())
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
	t.Cleanup(func() {
		if app.historyStore != nil {
			_ = app.historyStore.Close()
		}
	})
	return app
}

func assertAPIErrorStatus(t *testing.T, app *App, method, path, body string, want int, contentType string) model.APIResponse {
	t.Helper()
	response, err := app.API(model.APIRequest{Method: method, Path: path, Body: body, ContentType: contentType})
	if err == nil {
		t.Fatalf("%s %s unexpectedly succeeded", method, path)
	}
	if response.Status != want {
		t.Fatalf("%s %s status = %d, want %d (error %v)", method, path, response.Status, want, err)
	}
	return response
}
