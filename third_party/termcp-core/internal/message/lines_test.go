package message

import (
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/storage"
)

// The line index answers "which bytes are line N?" over the same log.bin every
// other read tool uses. These tests pin the numbering rules the tool contract
// states: lines are 1-based and absolute, the trailing partial line has no
// number, and a line's byte range reproduces it exactly, '\n' included.

func appendAll(t *testing.T, mgr *Manager, sess, shell string, chunks ...string) {
	t.Helper()
	for _, c := range chunks {
		if _, err := mgr.AppendOutput(sess, shell, []byte(c)); err != nil {
			t.Fatalf("AppendOutput(%q): %v", c, err)
		}
	}
}

func TestLineCount_CountsCompleteLinesOnly(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"

	appendAll(t, mgr, sess, shell, "alpha\n", "beta\ngamma")
	count, err := mgr.LineCount(sess, shell)
	if err != nil {
		t.Fatalf("LineCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("LineCount = %d, want 2: the trailing partial line must not be counted", count)
	}

	// Completing the partial line moves the count, and the completed line keeps
	// its number: nothing about line 1 or 2 changed.
	appendAll(t, mgr, sess, shell, "\n")
	count, err = mgr.LineCount(sess, shell)
	if err != nil {
		t.Fatalf("LineCount: %v", err)
	}
	if count != 3 {
		t.Fatalf("LineCount = %d, want 3", count)
	}
}

func TestLineBounds_ReproducesEachLine(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"

	lines := []string{"first\n", "second line\n", "\n", "fourth\n"}
	for _, l := range lines {
		appendAll(t, mgr, sess, shell, l)
	}

	for i, want := range lines {
		start, end, err := mgr.LineBounds(sess, shell, int64(i+1))
		if err != nil {
			t.Fatalf("LineBounds(line %d): %v", i+1, err)
		}
		if end-start != int64(len(want)) {
			t.Fatalf("line %d bounds [%d,%d) hold %d bytes, want %d", i+1, start, end, end-start, len(want))
		}
		data, err := mgr.store.ReadLog(sess, shell, start, int(end-start))
		if err != nil {
			t.Fatalf("ReadLog(line %d): %v", i+1, err)
		}
		if string(data) != want {
			t.Fatalf("line %d = %q, want %q", i+1, data, want)
		}
	}
}

func TestLineBounds_OutOfRangeIsAnErrorNotSilence(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"
	appendAll(t, mgr, sess, shell, "only\n")

	if _, _, err := mgr.LineBounds(sess, shell, 0); err == nil {
		t.Error("line 0 must be an error: line numbers are 1-based")
	}
	if _, _, err := mgr.LineBounds(sess, shell, 2); err == nil {
		t.Error("a line past the end must be an error, not a wrong window")
	}
	var rangeErr *LineOutOfRangeError
	if _, _, err := mgr.LineBounds(sess, shell, 9); !errors.As(err, &rangeErr) {
		t.Errorf("out-of-range error type = %v, want *LineOutOfRangeError", err)
	}
}

func TestLineRecords_SplitsExactlyOnNewlines(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"
	appendAll(t, mgr, sess, shell, "a\nbb\nccc\n")

	recs, err := mgr.LineRecords(sess, shell, 2, 3)
	if err != nil {
		t.Fatalf("LineRecords: %v", err)
	}
	if len(recs) != 2 || string(recs[0]) != "bb\n" || string(recs[1]) != "ccc\n" {
		t.Fatalf("LineRecords(2..3) = %q, want [bb\\n ccc\\n]", recs)
	}

	recs, err = mgr.LineRecords(sess, shell, 1, 99)
	if err != nil {
		t.Fatalf("LineRecords past the end: %v", err)
	}
	if len(recs) != 3 {
		t.Fatalf("LineRecords clamped to %d lines, want 3", len(recs))
	}
}

func TestLineSpanOfOffsets_MarksAndRanges(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"
	// Offsets: "one\n" = 0..3, "two\n" = 4..7, "three" = 8..12 (partial tail).
	appendAll(t, mgr, sess, shell, "one\ntwo\nthree")

	cases := []struct {
		name       string
		start, end int64
		from, to   int64
	}{
		{"line 1 whole", 0, 4, 1, 1},
		{"mid line 1", 2, 4, 1, 1},
		{"spans the boundary", 2, 7, 1, 1},
		{"swallows line 2's newline", 2, 8, 1, 2},
		{"mid line 2", 6, 13, 2, 2},
		{"into the partial tail", 4, 13, 2, 2},
		{"whole stream", 0, 13, 1, 2},
		{"zero-length mid line 1", 2, 2, 1, 1},
		{"zero-length right after a newline", 4, 4, 2, 2},
		{"zero-length inside the partial tail", 10, 10, 0, 0},
		{"zero-length at stream start", 0, 0, 0, 0},
		{"past the end", 20, 30, 0, 0},
		{"no complete line in range", 8, 13, 0, 0},
		{"range ends exactly at a newline", 4, 8, 2, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			from, to, err := mgr.LineSpanOfOffsets(sess, shell, c.start, c.end)
			if err != nil {
				t.Fatalf("LineSpanOfOffsets(%d,%d): %v", c.start, c.end, err)
			}
			if from != c.from || to != c.to {
				t.Fatalf("LineSpanOfOffsets(%d,%d) = (%d,%d), want (%d,%d)", c.start, c.end, from, to, c.from, c.to)
			}
		})
	}
}

// A line index that lags or went missing must repair itself on the next query,
// never answer from stale numbers: the client's cursor is the line number.
//
// Both simulations write the files the way a previous process would have left
// them - raw appends from outside the running store - because a torn or legacy
// index can only exist before this process ever touches the shell.
func TestLineIndex_RepairsLegacyAndLaggingIndex(t *testing.T) {
	store := storage.New(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	mgr := NewManager(store)
	const sess, shell = "s1", "sh1"

	// A session that predates log.lines: bytes exist, index does not. The last
	// line is partial ("gamma" has no newline), so only two lines are complete.
	if err := rawAppend(store.LogPath(sess, shell), []byte("alpha\nbeta\ngamma")); err != nil {
		t.Fatalf("raw log append: %v", err)
	}
	count, err := mgr.LineCount(sess, shell)
	if err != nil {
		t.Fatalf("LineCount: %v", err)
	}
	if count != 2 {
		t.Fatalf("LineCount = %d, want 2 after building a missing index", count)
	}

	// Simulate a crash between the byte append and the record append: bytes the
	// index has never seen. The sync must index them without duplicating the
	// records it already holds.
	if err := rawAppend(store.LogPath(sess, shell), []byte("delta\n")); err != nil {
		t.Fatalf("raw log append: %v", err)
	}
	count, err = mgr.LineCount(sess, shell)
	if err != nil {
		t.Fatalf("LineCount after lag: %v", err)
	}
	if count != 3 {
		t.Fatalf("LineCount = %d, want 3 after syncing a lagging index", count)
	}
	records, err := store.ReadLineRecords(sess, shell, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	// '\n' at 5, 10, and 21 ("gamma" is a partial tail with no record): exactly
	// three records, so the repair appended each newline exactly once.
	want := []int64{5, 10, 21}
	if len(records) != len(want) {
		t.Fatalf("index holds %d records (%v), want %v", len(records), records, want)
	}
	for i := range want {
		if records[i] != want[i] {
			t.Fatalf("record %d = %d, want %d (index duplicated or skipped a line)", i, records[i], want[i])
		}
	}
}

// TestLineIndex_TornTailIsTruncatedOnNextAppend pins the crash-mid-record
// recovery: a partial 8-byte record is never counted as a line, and the first
// append after the crash realigns the file instead of writing records into the
// garbage.
func TestLineIndex_TornTailIsTruncatedOnNextAppend(t *testing.T) {
	store := storage.New(t.TempDir())
	t.Cleanup(func() { _ = store.Close() })
	mgr := NewManager(store)
	const sess, shell = "s1", "sh1"

	if err := rawAppend(store.LogPath(sess, shell), []byte("alpha\n")); err != nil {
		t.Fatalf("raw log append: %v", err)
	}
	torn := make([]byte, 8+5)
	binary.LittleEndian.PutUint64(torn, 5)
	copy(torn[8:], []byte{1, 2, 3, 4, 5})
	if err := rawAppend(store.LineIndexPath(sess, shell), torn); err != nil {
		t.Fatalf("torn index append: %v", err)
	}

	count, err := mgr.LineCount(sess, shell)
	if err != nil {
		t.Fatalf("LineCount: %v", err)
	}
	if count != 1 {
		t.Fatalf("LineCount = %d, want 1: a torn record is not a line", count)
	}

	if _, err := mgr.AppendOutput(sess, shell, []byte("beta\n")); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}
	records, err := store.ReadLineRecords(sess, shell, 0, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 || records[0] != 5 || records[1] != 10 {
		t.Fatalf("records after torn-tail recovery = %v, want [5 10] (realigned)", records)
	}
	count, err = mgr.LineCount(sess, shell)
	if err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("LineCount = %d, want 2 after realignment", count)
	}
}

func TestLines_UnavailableWithoutPersistence(t *testing.T) {
	mgr := NewManager(nil)
	if _, err := mgr.LineCount("s", "sh"); !errors.Is(err, ErrLinesUnavailable) {
		t.Errorf("LineCount without a store = %v, want ErrLinesUnavailable", err)
	}
	if _, _, err := mgr.LineBounds("s", "sh", 1); !errors.Is(err, ErrLinesUnavailable) {
		t.Errorf("LineBounds without a store = %v, want ErrLinesUnavailable", err)
	}
}

// rawAppend appends bytes to a file the way a previous process would have:
// open-append-close, outside every handle the running store owns. The store
// creates its directory tree on first append, so a legacy file has to bring its
// own directory.
func rawAppend(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}
