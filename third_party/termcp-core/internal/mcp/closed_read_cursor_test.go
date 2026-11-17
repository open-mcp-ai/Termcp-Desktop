package mcp

import (
	"context"
	"testing"

	"github.com/open-mcp-ai/termcp/pkg/api"
)

// TestClosedChannelReadAdvancesItsCursor pins issue #159 C1.
//
// `read <ch>` on a live channel means "what is new since my last read". On a
// closed one it used to mean "the tail of the history", so the same call returned
// the same bytes on every poll — a caller watching an extension's output re-read
// its whole history each time and the offsets never advanced. The cursor makes the
// closed case mean what the live case means; an explicit offset is still the full
// replay, which is what a caller reading from the beginning asks for.
func TestClosedChannelReadAdvancesItsCursor(t *testing.T) {
	s, store, sessMgr, msgMgr := newTestServerWithHistory(t)

	const archID, shellID = "archived-c1", "shell-c1"
	meta := api.Session{
		ID:     archID,
		Name:   "Build Task",
		Status: api.SessionExited,
		Shells: []api.Session{{ID: shellID, Name: "main", Status: api.SessionExited}},
	}
	if err := store.SaveSession(meta); err != nil {
		t.Fatalf("SaveSession: %v", err)
	}
	for _, sh := range meta.Shells {
		if err := store.SaveShell(meta.ID, sh); err != nil {
			t.Fatalf("SaveShell: %v", err)
		}
	}
	if err := sessMgr.RestoreDead(); err != nil {
		t.Fatalf("RestoreDead: %v", err)
	}
	if _, err := msgMgr.AppendOutput(archID, shellID, []byte("first line\n")); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}

	read := func(args map[string]any) map[string]any {
		t.Helper()
		req := makeRequest(args)
		res, err := s.handleReadOutput(context.Background(), req)
		if err != nil || res.IsError {
			t.Fatalf("handleReadOutput(%v): %v", args, err)
		}
		return parseResult(t, res)
	}

	// 1. The first default read hands over the history, from byte 0.
	first := read(map[string]any{"shell_id": archID})
	if first["output"] != "first line\n" {
		t.Fatalf("first read = %q, want the history", first["output"])
	}
	if first["end_offset"] != first["total_bytes"] {
		t.Fatalf("the first read should have reached the end: %v of %v", first["end_offset"], first["total_bytes"])
	}

	// 2. The second default read returns NOTHING: there is no new byte, and the
	//    history it already gave is not new. This is the assertion that fails on
	//    the old behaviour, which replayed the whole stream here.
	second := read(map[string]any{"shell_id": archID})
	if second["output"] != "" {
		t.Fatalf("the second read replayed old bytes: %q", second["output"])
	}
	if second["start_offset"] != first["end_offset"] {
		t.Errorf("the cursor did not advance: start %v, previous end %v", second["start_offset"], first["end_offset"])
	}

	// 3. Appended bytes arrive on the next default read, and only those.
	if _, err := msgMgr.AppendOutput(archID, shellID, []byte("second line\n")); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}
	third := read(map[string]any{"shell_id": archID})
	if third["output"] != "second line\n" {
		t.Fatalf("default read = %q, want only the new line", third["output"])
	}

	// 4. An explicit offset 0 is still the full replay: the cursor is a DEFAULT,
	//    not a policy.
	replay := read(map[string]any{"shell_id": archID, "offset": 0.0})
	if replay["output"] != "first line\nsecond line\n" {
		t.Fatalf("offset 0 should replay everything, got %q", replay["output"])
	}

	// 5. And an explicit tail is still a tail, mot the cursor.
	tail := read(map[string]any{"shell_id": archID, "tail_lines": 1.0})
	if tail["output"] != "second line\n" {
		t.Fatalf("tail_lines 1 = %q, want the last line", tail["output"])
	}
}
