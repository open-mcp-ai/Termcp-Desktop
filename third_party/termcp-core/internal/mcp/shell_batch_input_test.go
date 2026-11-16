package mcp

import (
	"context"
	"testing"

	"github.com/open-mcp-ai/termcp/pkg/api"
)

// Issue #80: one command to a fleet. The lifecycle commands already accepted a
// comma-separated id list; input did not, so "input this into all ten shells" was
// ten round trips — and an agent's turns are the scarce resource, not its calls.
//
// Two properties are worth pinning and neither is visible from the tool
// description: the single-id reply must stay exactly what it was (an agent that
// types into one shell must not start reading a results array), and one bad entry
// must not stop the others (a dead shell among nine live ones is the normal case,
// not the exception).
func TestShellInputBatchReachesEveryShell(t *testing.T) {
	s, _, _, _ := newTestServerWithHistory(t)
	ctx := context.Background()

	startSession := func() (sessID, shellID string) {
		t.Helper()
		res, err := s.handleStartSession(ctx, makeRequest(map[string]any{
			"command":    testShell(),
			"args":       testShellIdleArgs(),
			"mode":       "pipe",
			"ssh_config": "internal",
		}))
		if err != nil || res.IsError {
			t.Fatalf("start session: %v %+v", err, res)
		}
		m := parseResult(t, res)
		id := m["session_id"].(string)
		t.Cleanup(func() {
			_, _ = s.handleTerminateSession(ctx, makeRequest(map[string]any{"session_id": id, "force": true}))
		})
		return id, m["shell_id"].(string)
	}

	_, shellA := startSession()
	_, shellB := startSession()

	// A batch writes to every named shell.
	res, err := s.handleSendInput(ctx, makeRequest(map[string]any{
		"shell_id": shellA + "," + shellB,
		"text":     "echo FLEET\n",
	}))
	if err != nil || res.IsError {
		t.Fatalf("batch shell_input: %v %+v", err, res)
	}
	m := parseResult(t, res)
	results, ok := m["results"].([]any)
	if !ok {
		t.Fatalf("a batch must answer with results, got %v", m)
	}
	if len(results) != 2 {
		t.Fatalf("got %d entries, want 2", len(results))
	}
	for _, r := range results {
		entry, _ := r.(map[string]any)
		if entry["ok"] != true {
			t.Errorf("entry not ok: %v", entry)
		}
		// Each entry carries the same receipt a single call would have, so a
		// caller can still pair a later response with the line it typed.
		if _, has := entry["offset"]; !has {
			t.Errorf("a batch entry must carry its own offset: %v", entry)
		}
		if entry["shell_id"] == "" || entry["shell_id"] == nil {
			t.Errorf("a batch entry must name its shell: %v", entry)
		}
	}

	/* Every shell must actually have received the bytes. Checking the reply
	   alone is not enough: a fan-out that wrote to the first shell and reported
	   success for the rest would pass a shape-only assertion, and the shape is
	   the part that is easy to get right.

	   The witness is the log's own input mark. These are pipe shells, so there is
	   no echo to read back — an input mark (status "a", zero-length) is exactly
	   what "bytes were written to this shell" leaves behind. */
	for _, sh := range []string{shellA, shellB} {
		sessID := sessionIDOf(t, s, sh)
		marks, err := s.sessMgr.Marks(sessID, sh)
		if err != nil {
			t.Fatalf("marks for %s: %v", sh, err)
		}
		found := false
		for _, m := range marks {
			if m.Status == api.LogAIInput {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("shell %s has no AI input mark: the batch never wrote to it", sh)
		}
	}

	// A single id keeps the flat reply it always had: no results wrapper.
	res, err = s.handleSendInput(ctx, makeRequest(map[string]any{
		"shell_id": shellA, "text": "echo ONE\n",
	}))
	if err != nil || res.IsError {
		t.Fatalf("single shell_input: %v %+v", err, res)
	}
	m = parseResult(t, res)
	if _, has := m["results"]; has {
		t.Errorf("a single id must not answer with a results array: %v", m)
	}
	if m["success"] != true {
		t.Errorf("single reply = %v, want the flat success shape", m)
	}
}

// One unresolvable entry must not abort the batch: the other shells still take
// the bytes, and the reply says exactly which entry failed.
func TestShellInputBatchIsNotAllOrNothing(t *testing.T) {
	s, _, _, _ := newTestServerWithHistory(t)
	ctx := context.Background()

	res, err := s.handleStartSession(ctx, makeRequest(map[string]any{
		"command":    testShell(),
		"args":       testShellIdleArgs(),
		"mode":       "pipe",
		"ssh_config": "internal",
	}))
	if err != nil || res.IsError {
		t.Fatalf("start session: %v %+v", err, res)
	}
	m := parseResult(t, res)
	sessID := m["session_id"].(string)
	shellID := m["shell_id"].(string)
	t.Cleanup(func() {
		_, _ = s.handleTerminateSession(ctx, makeRequest(map[string]any{"session_id": sessID, "force": true}))
	})

	res, err = s.handleSendInput(ctx, makeRequest(map[string]any{
		"shell_id": "no-such-shell," + shellID,
		"text":     "echo SURVIVOR\n",
	}))
	if err != nil || res.IsError {
		t.Fatalf("batch shell_input with a bad entry: %v %+v", err, res)
	}
	results, _ := parseResult(t, res)["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("got %d entries, want 2", len(results))
	}
	byID := map[string]map[string]any{}
	for _, r := range results {
		entry, _ := r.(map[string]any)
		byID[entry["shell_id"].(string)] = entry
	}
	if bad := byID["no-such-shell"]; bad == nil || bad["ok"] != false {
		t.Errorf("the unknown entry must be reported as failed: %v", byID["no-such-shell"])
	} else if bad["error"] == nil {
		t.Error("a failed entry must say why")
	}
	if good := byID[shellID]; good == nil || good["ok"] != true {
		t.Errorf("the good entry must still have been written: %v", byID[shellID])
	}
}

// The same batch form works for keys, so ending many lines is one call.
func TestShellKeyBatchReachesEveryShell(t *testing.T) {
	s, _, _, _ := newTestServerWithHistory(t)
	ctx := context.Background()

	start := func() string {
		t.Helper()
		res, err := s.handleStartSession(ctx, makeRequest(map[string]any{
			"command":    testShell(),
			"args":       testShellIdleArgs(),
			"mode":       "pipe",
			"ssh_config": "internal",
		}))
		if err != nil || res.IsError {
			t.Fatalf("start session: %v %+v", err, res)
		}
		m := parseResult(t, res)
		t.Cleanup(func() {
			_, _ = s.handleTerminateSession(ctx, makeRequest(map[string]any{"session_id": m["session_id"].(string), "force": true}))
		})
		return m["shell_id"].(string)
	}
	a, b := start(), start()

	res, err := s.handlePressKey(ctx, makeRequest(map[string]any{
		"shell_id": a + "," + b,
		"key":      "enter",
	}))
	if err != nil || res.IsError {
		t.Fatalf("batch shell_key: %v %+v", err, res)
	}
	results, _ := parseResult(t, res)["results"].([]any)
	if len(results) != 2 {
		t.Fatalf("got %d entries, want 2", len(results))
	}
	for _, r := range results {
		entry, _ := r.(map[string]any)
		if entry["ok"] != true {
			t.Errorf("entry not ok: %v", entry)
		}
	}

	// A single id still answers flat.
	res, err = s.handlePressKey(ctx, makeRequest(map[string]any{"shell_id": a, "key": "enter"}))
	if err != nil || res.IsError {
		t.Fatalf("single shell_key: %v %+v", err, res)
	}
	if _, has := parseResult(t, res)["results"]; has {
		t.Error("a single id must not answer with a results array")
	}
}

// A batch into review-gated shells must report review per entry rather than
// claiming a write: the batch reply is the one place a model reads to decide
// whether anything happened.
func TestShellInputBatchReportsReviewPending(t *testing.T) {
	s, _, _, _ := newTestServerWithHistory(t)
	ctx := context.Background()

	start := func() (string, string) {
		t.Helper()
		res, err := s.handleStartSession(ctx, makeRequest(map[string]any{
			"command":    testShell(),
			"args":       testShellIdleArgs(),
			"mode":       "pipe",
			"ssh_config": "internal",
		}))
		if err != nil || res.IsError {
			t.Fatalf("start session: %v %+v", err, res)
		}
		m := parseResult(t, res)
		id := m["session_id"].(string)
		t.Cleanup(func() {
			_, _ = s.handleTerminateSession(ctx, makeRequest(map[string]any{"session_id": id, "force": true}))
		})
		return id, m["shell_id"].(string)
	}
	sessA, shellA := start()
	_, shellB := start()

	live := s.sessMgr.Get(sessA)
	if live == nil {
		t.Fatal("session missing")
	}
	if err := live.EnableApproval(1, 0); err != nil {
		t.Fatal(err)
	}

	res, err := s.handleSendInput(ctx, makeRequest(map[string]any{
		"shell_id": shellA + "," + shellB,
		"text":     "echo HELD\n",
	}))
	if err != nil || res.IsError {
		t.Fatalf("batch into a gated shell: %v %+v", err, res)
	}
	results, _ := parseResult(t, res)["results"].([]any)
	for _, r := range results {
		entry, _ := r.(map[string]any)
		if entry["shell_id"] == shellA {
			if entry["review_pending"] != true {
				t.Errorf("the gated shell must report review_pending: %v", entry)
			}
			// A held write did not happen, so the canonical review shape has to
			// be there — a model branches on approved/review_pending.
			if entry["approved"] != false {
				t.Errorf("approved = %v, want false for a held write", entry["approved"])
			}
		} else if entry["review_pending"] == true {
			t.Errorf("the ungated shell must not report review_pending: %v", entry)
		}
	}
}
