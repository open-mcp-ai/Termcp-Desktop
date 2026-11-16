package main

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
)

type extensionListResponse struct {
	Extensions []extensionSummary `json:"extensions"`
}

type extensionSummary struct {
	Name      string `json:"name"`
	Target    string `json:"target"`
	URL       string `json:"url,omitempty"`
	Status    string `json:"status"`
	Enabled   bool   `json:"enabled"`
	Session   string `json:"session_id,omitempty"`
	Shell     string `json:"shell_id,omitempty"`
	Failure   any    `json:"failure,omitempty"`
	NextRetry int64  `json:"next_retry_ms,omitempty"`
	Targets   []struct {
		Target string `json:"target"`
		Status string `json:"status"`
	} `json:"targets,omitempty"`
	HostCount    int `json:"host_count"`
	ForwardCount int `json:"forward_count"`
	Holes        []struct {
		Name string `json:"name"`
		Set  bool   `json:"set"`
	} `json:"holes,omitempty"`
	Needs []string `json:"needs,omitempty"`
}

type extensionDetailResponse struct {
	Extension struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"extension"`
	Target   string           `json:"target"`
	Mode     string           `json:"mode"`
	Command  string           `json:"command"`
	Notes    string           `json:"notes"`
	Allow    []string         `json:"allow"`
	Paths    []string         `json:"paths"`
	Failures []map[string]any `json:"failures"`
	Holes    []struct {
		Name string `json:"name"`
		Set  bool   `json:"set"`
	} `json:"holes"`
}

func TestIntegratedExtensionAuthoringLifecycle(t *testing.T) {
	app := startExtensionTestApp(t)

	created := assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":        "draft",
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

	fileView := "target = \"internal\"\ndescription = \"edited as TOML\"\nenabled = false\nmode = \"pipe\"\ncommand = \"cat\"\n"
	assertAPIStatus(t, app, "PUT", "/api/extensions/draft", fileView, http.StatusOK, "application/toml")
	raw := assertAPIStatus(t, app, "GET", "/api/extensions/draft?format=toml", nil, http.StatusOK)
	if !strings.Contains(raw.Body, `description = "edited as TOML"`) {
		t.Fatalf("TOML view did not read the file that was written: %q", raw.Body)
	}

	invalid := "name = \"other\"\ntarget = \"internal\"\nmode = \"pipe\"\ncommand = \"cat\"\n"
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
target = "internal"
description = "first"
enabled = false
mode = "pipe"
command = "cat"

[[extensions]]
name = "two"
target = "internal"
enabled = false
paths = ["/tmp/unrelated"]
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
target = "internal"
enabled = false
mode = "pipe"
command = "cat"

[[extensions]]
name = "invalid-part"
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

func TestLegacyExtensionKindIsOnlyAnIgnoredAuthoringLabel(t *testing.T) {
	app := startExtensionTestApp(t)
	assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":    "legacy-label",
		"kind":    "skill",
		"target":  "internal",
		"enabled": false,
		"mode":    "pipe",
		"command": "cat",
	}, http.StatusCreated)
	var detail extensionDetailResponse
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions/legacy-label", nil, http.StatusOK), &detail)
	if detail.Command != "cat" || detail.Mode != "pipe" {
		t.Fatalf("legacy kind changed the real capability fields: %+v", detail)
	}
	list := extensionListResponse{}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions", nil, http.StatusOK), &list)
	if len(list.Extensions) != 1 || list.Extensions[0].Status != "disabled" {
		t.Fatalf("legacy label did not resolve as an ordinary disabled template: %+v", list.Extensions)
	}
}

func TestIntegratedExtensionTemplateValuesPreviewAndCompatibilityFields(t *testing.T) {
	app := startExtensionTestApp(t)
	root := t.TempDir()
	template := `target = "{{target}}"
mode = "pipe"
command = "{{command}}"
paths = ["{{root}}"]

[params.target]
type = "string"
help = "Connection profile that runs the extension"

[params.command]
type = "string"
help = "Program on the target"

[params.root]
type = "string"
help = "Path this installation may read"
`
	assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":     "values",
		"template": template,
		"values": map[string]any{
			"target":  "internal",
			"command": "cat",
			"root":    root,
		},
	}, http.StatusCreated)

	detail := assertAPIStatus(t, app, "GET", "/api/extensions/values", nil, http.StatusOK)
	var before extensionDetailResponse
	decodeResponse(t, detail, &before)
	if joinedHoles(before.Holes) != "command:true,root:true,target:true" {
		t.Fatalf("holes after install = %s", joinedHoles(before.Holes))
	}
	if before.Command != "cat" || len(before.Paths) != 1 || before.Paths[0] != root {
		t.Fatalf("rendered extension = %+v", before)
	}

	exported := assertAPIStatus(t, app, "GET", "/api/extensions/batch?names=values", nil, http.StatusOK)
	if !strings.Contains(exported.Body, "{{root}}") || !strings.Contains(exported.Body, "params.root") {
		t.Fatalf("export lost the template holes: %s", exported.Body)
	}
	if strings.Contains(exported.Body, root) {
		t.Fatalf("export leaked config.local.toml value %q: %s", root, exported.Body)
	}

	updatedTemplate := strings.Replace(template, `paths = ["{{root}}"]`, "description = \"updated template\"\npaths = [\"{{root}}\"]", 1)
	assertAPIStatus(t, app, "PUT", "/api/extensions/values", map[string]any{"template": updatedTemplate}, http.StatusOK)
	var afterUpdate extensionDetailResponse
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions/values", nil, http.StatusOK), &afterUpdate)
	if joinedHoles(afterUpdate.Holes) != "command:true,root:true,target:true" {
		t.Fatalf("template update lost local values: %s", joinedHoles(afterUpdate.Holes))
	}

	preview := assertAPIStatus(t, app, "POST", "/api/extensions/values/preview", updatedTemplate, http.StatusOK, "application/toml; charset=utf-8")
	if strings.Contains(preview.Body, "{{root}}") || strings.Contains(preview.Body, "[params.root]") || !strings.Contains(preview.Body, root) {
		t.Fatalf("preview did not render with saved values: %s", preview.Body)
	}

	assertAPIErrorStatus(t, app, "PUT", "/api/extensions/values", `{"values":{"unknown":"x"}}`, http.StatusBadRequest, "application/json")
	assertAPIStatus(t, app, "PUT", "/api/extensions/values", map[string]any{"values": map[string]any{"root": nil}}, http.StatusOK)
	afterClear := assertAPIStatus(t, app, "GET", "/api/extensions/values", nil, http.StatusOK)
	var waiting extensionDetailResponse
	decodeResponse(t, afterClear, &waiting)
	if joinedHoles(waiting.Holes) != "command:true,root:false,target:true" {
		t.Fatalf("cleared value state = %s", joinedHoles(waiting.Holes))
	}

	assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":    "multi-host",
		"enabled": false,
		"mode":    "pipe",
		"command": "cat",
		"targets": []any{
			map[string]any{"host": "internal"},
			map[string]any{"host": "not-connected"},
		},
	}, http.StatusCreated)
	assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":        "url-peer",
		"enabled":     false,
		"target":      "internal",
		"url":         "http://127.0.0.1:1/mcp",
		"headers":     map[string]string{"X-Test": "one"},
		"termcp_tool": "termcp_term",
	}, http.StatusCreated)

	list := extensionListResponse{}
	decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions", nil, http.StatusOK), &list)
	byName := map[string]extensionSummary{}
	for _, item := range list.Extensions {
		byName[item.Name] = item
	}
	if got := byName["multi-host"]; got.Target != "" || got.HostCount != 2 {
		t.Fatalf("multi-host summary = %+v", got)
	}
	if got := byName["url-peer"]; got.URL == "" || got.HostCount != 1 {
		t.Fatalf("URL peer summary = %+v", got)
	}
	assertAPIStatus(t, app, "PUT", "/api/extensions/url-peer", map[string]any{
		"url":     "http://127.0.0.1:2/mcp",
		"headers": map[string]string{"X-Test": "two"},
	}, http.StatusOK)
}

func TestURLNativePeerUpdateInvalidatesOldHandle(t *testing.T) {
	app := startExtensionTestApp(t)
	var firstCalls atomic.Int32
	first := mockStreamableMCP(t, "Bearer one", "ONE", &firstCalls)
	secondCalls := atomic.Int32{}
	second := mockStreamableMCP(t, "Bearer two", "TWO", &secondCalls)

	assertAPIStatus(t, app, "POST", "/api/extensions", map[string]any{
		"name":    "url-peer-live",
		"target":  "internal",
		"enabled": true,
		"url":     first.URL,
		"headers": map[string]string{"Authorization": "Bearer one"},
	}, http.StatusCreated)
	waitExtensionStatus(t, app, "url-peer-live", "available", 10*time.Second)

	open := termREST(t, app, "open ext url-peer-live", nil, 10)
	if open.Code != http.StatusOK || !strings.Contains(open.Body, "echo_tool") {
		t.Fatalf("open URL peer = %d %s", open.Code, open.Body)
	}
	handle := open.Headers["Handle"]
	if handle == "" {
		handle = open.Headers["handle"]
	}
	if handle == "" {
		t.Fatalf("open URL peer returned no Handle: %+v", open.Headers)
	}
	call := termREST(t, app, `call `+handle+` echo_tool {"text":"first"}`, nil, 10)
	if call.Code != http.StatusOK || !strings.Contains(call.Body, "ONE:first") {
		t.Fatalf("first URL peer call = %d %s", call.Code, call.Body)
	}

	initialize := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","clientInfo":{"name":"ext-test","version":"1"}}}`)
	extResp, err := http.Post(app.core.BaseURL()+"/ext/url-peer-live/stream", "application/json", strings.NewReader(string(initialize)))
	if err != nil {
		t.Fatal(err)
	}
	_ = extResp.Body.Close()
	if extResp.StatusCode != http.StatusOK {
		t.Fatalf("extension MCP endpoint initialize = %d", extResp.StatusCode)
	}

	assertAPIStatus(t, app, "PUT", "/api/extensions/url-peer-live", map[string]any{
		"url":     second.URL,
		"headers": map[string]string{"Authorization": "Bearer two"},
	}, http.StatusOK)
	waitExtensionStatus(t, app, "url-peer-live", "available", 10*time.Second)

	deadline := time.Now().Add(5 * time.Second)
	for {
		old, _ := rawTermREST(t, app, `call `+handle+` echo_tool {"text":"old"}`, nil, 2)
		if old.Code >= http.StatusBadRequest {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("old URL peer handle still works after URL/headers update: %d %s", old.Code, old.Body)
		}
		time.Sleep(50 * time.Millisecond)
	}

	reopened := termREST(t, app, "open ext url-peer-live", nil, 10)
	if reopened.Code != http.StatusOK {
		t.Fatalf("reopen updated URL peer = %d %s", reopened.Code, reopened.Body)
	}
	newHandle := reopened.Headers["Handle"]
	if newHandle == "" {
		newHandle = reopened.Headers["handle"]
	}
	if newHandle == "" || newHandle == handle {
		t.Fatalf("updated URL peer handle = %q, old = %q", newHandle, handle)
	}
	updated := termREST(t, app, `call `+newHandle+` echo_tool {"text":"second"}`, nil, 10)
	if updated.Code != http.StatusOK || !strings.Contains(updated.Body, "TWO:second") {
		t.Fatalf("updated URL peer call = %d %s", updated.Code, updated.Body)
	}
	if firstCalls.Load() == 0 || secondCalls.Load() == 0 {
		t.Fatalf("URL peers were not both exercised: first=%d second=%d", firstCalls.Load(), secondCalls.Load())
	}

	assertAPIStatus(t, app, "DELETE", "/api/extensions/url-peer-live", nil, http.StatusNoContent)
	afterDelete, _ := rawTermREST(t, app, `call `+newHandle+` echo_tool {"text":"gone"}`, nil, 2)
	if afterDelete.Code < http.StatusBadRequest {
		t.Fatalf("deleted URL peer still answered: %d %s", afterDelete.Code, afterDelete.Body)
	}
}

type termEnvelope struct {
	Code    int               `json:"code"`
	Body    string            `json:"body"`
	Headers map[string]string `json:"-"`
	Raw     map[string]any    `json:"-"`
}

func termREST(t *testing.T, app *App, command string, args []any, timeout int) termEnvelope {
	t.Helper()
	result, err := rawTermREST(t, app, command, args, timeout)
	if err != nil {
		t.Fatalf("TERM %q: %v (%+v)", command, err, result)
	}
	return result
}

func rawTermREST(t *testing.T, app *App, command string, args []any, timeout int) (termEnvelope, error) {
	t.Helper()
	payload := map[string]any{"cmd": command}
	if args != nil {
		payload["args"] = args
	}
	if timeout > 0 {
		payload["timeout"] = timeout
	}
	data, _ := json.Marshal(payload)
	response, requestErr := app.API(model.APIRequest{Method: http.MethodPost, Path: "/api/term", Body: string(data), ContentType: "application/json"})
	if response.Body == "" {
		return termEnvelope{}, requestErr
	}
	var raw map[string]any
	if err := json.Unmarshal([]byte(response.Body), &raw); err != nil {
		t.Fatalf("decode TERM reply: %v (%s)", err, response.Body)
	}
	code, _ := raw["code"].(float64)
	body, _ := raw["body"].(string)
	headers := map[string]string{}
	for key, value := range raw {
		if text, ok := value.(string); ok {
			headers[key] = text
		}
	}
	return termEnvelope{Code: int(code), Body: body, Headers: headers, Raw: raw}, requestErr
}

func waitExtensionStatus(t *testing.T, app *App, name, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		var list extensionListResponse
		decodeResponse(t, assertAPIStatus(t, app, "GET", "/api/extensions", nil, http.StatusOK), &list)
		for _, extension := range list.Extensions {
			if extension.Name == name && extension.Status == want {
				return
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("extension %s did not reach %s: %+v", name, want, list.Extensions)
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func mockStreamableMCP(t *testing.T, authorization, prefix string, calls *atomic.Int32) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != authorization {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		calls.Add(1)
		var env map[string]any
		if err := json.NewDecoder(r.Body).Decode(&env); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		id := env["id"]
		method, _ := env["method"].(string)
		w.Header().Set("Content-Type", "application/json")
		switch method {
		case "initialize":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"protocolVersion": "2024-11-05",
				"serverInfo":      map[string]any{"name": "desktop-url-peer", "version": "1"},
				"capabilities":    map[string]any{"tools": map[string]any{}},
			}})
		case "notifications/initialized":
			w.WriteHeader(http.StatusOK)
		case "tools/list":
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"tools": []any{map[string]any{"name": "echo_tool", "description": prefix + " echo", "inputSchema": map[string]any{"type": "object"}}},
			}})
		case "tools/call":
			params, _ := env["params"].(map[string]any)
			args, _ := params["arguments"].(map[string]any)
			text, _ := args["text"].(string)
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{
				"content": []any{map[string]any{"type": "text", "text": prefix + ":" + text}},
			}})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"jsonrpc": "2.0", "id": id, "result": map[string]any{}})
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func joinedHoles(holes []struct {
	Name string `json:"name"`
	Set  bool   `json:"set"`
}) string {
	parts := make([]string, 0, len(holes))
	for _, hole := range holes {
		parts = append(parts, hole.Name+":"+strconv.FormatBool(hole.Set))
	}
	sort.Strings(parts)
	return strings.Join(parts, ",")
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
