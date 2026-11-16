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

// Issue #110 asks to see, from outside any one terminal, what an agent did and
// when. The data was always there: every shell's log.jsonl marks each span with
// its producer ("a" for an agent, "i" for the human, "q"/"A" for a review
// decision) and the millisecond it started. What was missing was a view across
// sessions. These tests pin that view's shape: which marks it carries, which it
// drops on purpose, and that it is ordered and bounded.

func TestActivityClassifiesEachMarkKind(t *testing.T) {
	cases := []struct {
		status  api.LogStatus
		wantOK  bool
		source  string
		verdict string
	}{
		{api.LogAIInput, true, "ai", "input"},
		{api.LogAPIInput, true, "human", "input"},
		{api.LogApprovalRequest, true, "system", "review-requested"},
		{api.LogApprovalGranted, true, "system", "review-granted"},
		// Output is not an action. Every command produces it — a `find` prints
		// thousands of lines — so carrying it would bury the few rows a reader is
		// looking for. The input's byte offset is where that output starts.
		{api.LogOutput, false, "", ""},
		{api.LogStatus("?"), false, "", ""},
	}
	for _, tc := range cases {
		got, ok := activityFromMark("sess-1", "prod", "shell-1", api.LogMark{
			Status: tc.status, Time: 1700000000000, Offset: 4096,
		})
		if ok != tc.wantOK {
			t.Errorf("%s: carried = %v, want %v", tc.status, ok, tc.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if got.Source != tc.source || got.Verdict != tc.verdict {
			t.Errorf("%s: got %s/%s, want %s/%s", tc.status, got.Source, got.Verdict, tc.source, tc.verdict)
		}
		// Identity, time and offset all have to survive: an event without them
		// cannot be acted on (which session? when? where are the bytes?).
		if got.SessionID != "sess-1" || got.SessionName != "prod" || got.ShellID != "shell-1" {
			t.Errorf("%s: identity lost: %+v", tc.status, got)
		}
		if got.UnixMS != 1700000000000 || got.Time == "" {
			t.Errorf("%s: timestamp lost: %+v", tc.status, got)
		}
		if got.Bytes != 4096 {
			t.Errorf("%s: bytes = %d, want 4096", tc.status, got.Bytes)
		}
	}
}

// The endpoint's contract, exercised over a real store: newest first, bounded by
// limit, filterable by session and since_ms, and never null.
func TestActivityFeedOverRealSessions(t *testing.T) {
	dir := t.TempDir()
	store := storage.New(dir)

	// Two sessions, three actions between them at known instants. Output spans
	// are interleaved to prove they are dropped and do not disturb the order.
	seed := func(sessID, name, shellID string, marks []api.LogMark, size int) {
		t.Helper()
		if _, err := store.AppendLog(sessID, shellID, make([]byte, size), storage.StdioOut); err != nil {
			t.Fatal(err)
		}
		for _, m := range marks {
			if err := store.AppendMark(sessID, shellID, m); err != nil {
				t.Fatal(err)
			}
		}
		if err := store.SaveSession(api.Session{ID: sessID, Name: name, Status: api.SessionExited}); err != nil {
			t.Fatal(err)
		}
		if err := store.SaveShell(sessID, api.Session{ID: shellID, Name: "shell-1", Status: api.SessionExited}); err != nil {
			t.Fatal(err)
		}
	}
	seed("sess-a", "prod", "sh-a", []api.LogMark{
		{Status: api.LogAIInput, Time: 1000, Offset: 0},
		{Status: api.LogOutput, Time: 1001, Offset: 4},
		{Status: api.LogAIInput, Time: 3000, Offset: 8},
	}, 16)
	seed("sess-b", "staging", "sh-b", []api.LogMark{
		{Status: api.LogAPIInput, Time: 2000, Offset: 0},
	}, 8)
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	store2 := storage.New(dir)
	msgMgr := message.NewManager(store2)
	sessMgr := session.NewManager(msgMgr, store2, nil)
	if err := sessMgr.RestoreDead(); err != nil {
		t.Fatal(err)
	}
	h := &Handler{Sessions: sessMgr}
	mux := http.NewServeMux()
	h.Register(mux)

	type feed struct {
		Activity []struct {
			UnixMS      int64  `json:"unix_ms"`
			SessionID   string `json:"session_id"`
			SessionName string `json:"session_name"`
			ShellID     string `json:"shell_id"`
			Source      string `json:"source"`
			Verdict     string `json:"verdict"`
			Bytes       int64  `json:"bytes"`
		} `json:"activity"`
	}
	get := func(query string) feed {
		t.Helper()
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/activity"+query, nil))
		if rr.Code != http.StatusOK {
			t.Fatalf("%s = %d: %s", query, rr.Code, rr.Body.String())
		}
		var f feed
		if err := json.Unmarshal(rr.Body.Bytes(), &f); err != nil {
			t.Fatalf("decode: %v (%s)", err, rr.Body.String())
		}
		return f
	}

	all := get("")
	if len(all.Activity) != 3 {
		t.Fatalf("got %d events, want 3 (output spans are not events): %+v", len(all.Activity), all.Activity)
	}
	// Newest first.
	for i := 1; i < len(all.Activity); i++ {
		if all.Activity[i].UnixMS > all.Activity[i-1].UnixMS {
			t.Fatalf("not newest-first: %+v", all.Activity)
		}
	}
	// The newest event is the agent's second command, and the name resolves so a
	// reader does not have to look the session up.
	if all.Activity[0].SessionName != "prod" || all.Activity[0].Source != "ai" {
		t.Errorf("first event = %+v, want the agent's line in prod", all.Activity[0])
	}

	// limit caps the result rather than being ignored.
	if got := get("?limit=1"); len(got.Activity) != 1 {
		t.Errorf("limit=1 gave %d rows", len(got.Activity))
	}
	// since_ms lets a client page without re-reading everything.
	if got := get("?since_ms=2000"); len(got.Activity) != 2 {
		t.Errorf("since_ms=2000 gave %d rows, want 2", len(got.Activity))
	}
	// session_id narrows to one session.
	if got := get("?session_id=sess-b"); len(got.Activity) != 1 || got.Activity[0].SessionName != "staging" {
		t.Errorf("session filter gave %+v", got.Activity)
	}

	// A bad parameter is refused rather than silently defaulted: a client that
	// asked for 0 rows and got 200 would draw a wrong picture.
	for _, bad := range []string{"?limit=0", "?limit=-1", "?limit=abc", "?since_ms=-5", "?since_ms=x"} {
		rr := httptest.NewRecorder()
		mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/activity"+bad, nil))
		if rr.Code != http.StatusBadRequest {
			t.Errorf("%s = %d, want 400", bad, rr.Code)
		}
	}
}

// An instance with nothing to report answers with an empty array, not null: a
// client renders the feed either way and should not special-case it.
func TestActivityFeedIsEmptyNotNil(t *testing.T) {
	dir := t.TempDir()
	store := storage.New(dir)
	sessMgr := session.NewManager(message.NewManager(store), store, nil)
	h := &Handler{Sessions: sessMgr}
	mux := http.NewServeMux()
	h.Register(mux)

	rr := httptest.NewRecorder()
	mux.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/activity", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if _, ok := body["activity"].([]any); !ok {
		t.Fatalf("activity must be a JSON array, got %T", body["activity"])
	}
}
