package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/message"
	"github.com/open-mcp-ai/termcp/internal/session"
	"github.com/open-mcp-ai/termcp/internal/storage"
	"github.com/open-mcp-ai/termcp/pkg/api"
)

// The lines endpoint and the output-range stream parameter are the REST side of
// the same two indexes the MCP tools read (log.lines and log.stdio). The
// fixtures below persist deterministic logs, so offsets and contents are
// asserted literally.

// linesFixture persists a DEAD session whose log holds two complete lines and
// a trailing partial line, then registers a handler over it.
func linesFixture(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	dir := t.TempDir()
	store := storage.New(dir)
	const sessID, shellID = "s-lines", "sh-lines"

	if _, err := store.AppendLog(sessID, shellID, []byte("alpha\n"), storage.StdioOut); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendLog(sessID, shellID, []byte("beta\n"), storage.StdioOut); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendLog(sessID, shellID, []byte("gamma"), storage.StdioOut); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(api.Session{ID: sessID, Name: "lines", Status: api.SessionExited}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveShell(sessID, api.Session{ID: shellID, Name: "shell-1", Status: api.SessionExited}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store2 := storage.New(dir)
	msgMgr := message.NewManager(store2)
	sessMgr := session.NewManager(msgMgr, store2, nil)
	if err := sessMgr.RestoreDead(); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Sessions: sessMgr, MsgMgr: msgMgr}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, sessID, shellID
}

type linesBody struct {
	Lines []struct {
		N       int64  `json:"n"`
		Start   int64  `json:"start"`
		End     int64  `json:"end"`
		Content string `json:"content"`
	} `json:"lines"`
	TotalLines int64  `json:"total_lines"`
	TotalBytes int64  `json:"total_bytes"`
	SessionID  string `json:"session_id"`
}

func getLines(t *testing.T, mux *http.ServeMux, id, query string) (*httptest.ResponseRecorder, linesBody) {
	t.Helper()
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/shells/"+id+"/lines"+query, nil))
	var body linesBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil && rr.Code == http.StatusOK {
		t.Fatalf("decode lines body: %v (%s)", err, rr.Body.String())
	}
	return rr, body
}

func TestShellLines_PagesAndSkipsThePartialTail(t *testing.T) {
	mux, sessID, shellID := linesFixture(t)

	rr, body := getLines(t, mux, shellID, "")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET lines = %d (%s), want 200", rr.Code, rr.Body.String())
	}
	if body.TotalLines != 2 {
		t.Fatalf("total_lines = %d, want 2: the trailing partial line takes no number", body.TotalLines)
	}
	if body.TotalBytes != int64(len("alpha\nbeta\ngamma")) {
		t.Fatalf("total_bytes = %d, want %d", body.TotalBytes, len("alpha\nbeta\ngamma"))
	}
	if len(body.Lines) != 2 {
		t.Fatalf("lines = %d entries, want 2", len(body.Lines))
	}
	if body.Lines[0].N != 1 || body.Lines[0].Start != 0 || body.Lines[0].End != 6 {
		t.Fatalf("line 1 = %+v, want n=1 [0,6)", body.Lines[0])
	}
	if body.Lines[1].N != 2 || body.Lines[1].Start != 6 || body.Lines[1].End != 11 {
		t.Fatalf("line 2 = %+v, want n=2 [6,11)", body.Lines[1])
	}
	if body.Lines[0].Content != "" {
		t.Fatalf("content = %q, want absent without content=1", body.Lines[0].Content)
	}
	if body.SessionID != sessID {
		t.Errorf("session_id = %q, want %q", body.SessionID, sessID)
	}

	// content=1 includes the raw line, newline included.
	rr, body = getLines(t, mux, shellID, "?content=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET lines?content=1 = %d (%s)", rr.Code, rr.Body.String())
	}
	if body.Lines[0].Content != "alpha\n" || body.Lines[1].Content != "beta\n" {
		t.Fatalf("contents = %q / %q, want alpha / beta lines", body.Lines[0].Content, body.Lines[1].Content)
	}

	// A page from line 2 returns exactly line 2.
	rr, body = getLines(t, mux, shellID, "?from=2&count=1&content=1")
	if rr.Code != http.StatusOK {
		t.Fatalf("GET lines?from=2 = %d (%s)", rr.Code, rr.Body.String())
	}
	if len(body.Lines) != 1 || body.Lines[0].N != 2 || body.Lines[0].Content != "beta\n" {
		t.Fatalf("page from=2 = %+v, want only line 2", body.Lines)
	}

	// from past the end is an error that says so, not a page from elsewhere.
	rr, _ = getLines(t, mux, shellID, "?from=99")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("GET lines?from=99 = %d, want 400", rr.Code)
	}
}

// stdioFixture persists a log with interleaved stdout and stderr writes, so
// the stream parameter on output-range can be asserted byte-exactly.
func stdioFixture(t *testing.T) (*http.ServeMux, string, string) {
	t.Helper()
	dir := t.TempDir()
	store := storage.New(dir)
	const sessID, shellID = "s-stdio", "sh-stdio"

	if _, err := store.AppendLog(sessID, shellID, []byte("OUT-1\n"), storage.StdioOut); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendLog(sessID, shellID, []byte("ERR\n"), storage.StdioErr); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AppendLog(sessID, shellID, []byte("OUT-2\n"), storage.StdioOut); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveSession(api.Session{ID: sessID, Name: "stdio", Status: api.SessionExited}); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveShell(sessID, api.Session{ID: shellID, Name: "shell-1", Status: api.SessionExited}); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store2 := storage.New(dir)
	msgMgr := message.NewManager(store2)
	sessMgr := session.NewManager(msgMgr, store2, nil)
	if err := sessMgr.RestoreDead(); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Sessions: sessMgr, MsgMgr: msgMgr}
	mux := http.NewServeMux()
	h.Register(mux)
	return mux, sessID, shellID
}

type outputRangeBody struct {
	Start int64  `json:"start"`
	End   int64  `json:"end"`
	Total int64  `json:"total"`
	D     string `json:"d"`
}

func getOutputRange(t *testing.T, mux *http.ServeMux, id, query string) (*httptest.ResponseRecorder, outputRangeBody) {
	t.Helper()
	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/shells/"+id+"/output-range"+query, nil))
	var body outputRangeBody
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil && rr.Code == http.StatusOK {
		t.Fatalf("decode output-range body: %v (%s)", err, rr.Body.String())
	}
	return rr, body
}

func TestOutputRangeStreamFilter(t *testing.T) {
	mux, _, shellID := stdioFixture(t)

	rr, body := getOutputRange(t, mux, shellID, "?start=0&max=65536&stream=o")
	if rr.Code != http.StatusOK {
		t.Fatalf("stream=o = %d (%s)", rr.Code, rr.Body.String())
	}
	if body.D != "OUT-1\nOUT-2\n" {
		t.Fatalf("stream=o d = %q, want exactly the stdout lines", body.D)
	}
	// Offsets stay raw: end covers the whole window even though the returned
	// content shrank by the stderr bytes.
	if body.End != body.Total {
		t.Fatalf("end = %d, want total %d (the filter must not move offsets)", body.End, body.Total)
	}

	rr, body = getOutputRange(t, mux, shellID, "?start=0&max=65536&stream=e")
	if rr.Code != http.StatusOK {
		t.Fatalf("stream=e = %d (%s)", rr.Code, rr.Body.String())
	}
	if body.D != "ERR\n" {
		t.Fatalf("stream=e d = %q, want exactly the stderr line", body.D)
	}

	rr, body = getOutputRange(t, mux, shellID, "?start=0&max=65536")
	if rr.Code != http.StatusOK {
		t.Fatalf("default = %d (%s)", rr.Code, rr.Body.String())
	}
	if body.D != "OUT-1\nERR\nOUT-2\n" {
		t.Fatalf("default d = %q, want the merged stream", body.D)
	}
	if body.End != body.Total {
		t.Fatalf("default end = %d, want total %d", body.End, body.Total)
	}

	rr, _ = getOutputRange(t, mux, shellID, "?start=0&stream=bogus")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("stream=bogus = %d, want 400", rr.Code)
	}
}
