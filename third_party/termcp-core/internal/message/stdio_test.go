package message

import (
	"testing"

	"github.com/open-mcp-ai/termcp/internal/storage"
)

// log.stdio records stream SWITCHES, not chunks: the tests below pin the
// encoding (sign = stream, |v|-1 = offset), the switch-only write rule, and
// the range reconstruction the shell_output stream filter consumes.

func TestStderrRanges_MergesRunsAndClosesAtLogEnd(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"

	// OUT(0..9) ERR(10..19) OUT(20..29) ERR(30..39, run to EOF).
	// Switch records: +1 (stdout@0), -11 (stderr@10), +21 (stdout@20), -31 (stderr@30).
	appendOut := func(data string) {
		if _, err := mgr.AppendOutput(sess, shell, []byte(data)); err != nil {
			t.Fatalf("AppendOutput: %v", err)
		}
	}
	appendErr := func(data string) {
		if _, err := mgr.AppendStderr(sess, shell, []byte(data)); err != nil {
			t.Fatalf("AppendStderr: %v", err)
		}
	}
	appendOut("0123456789")
	appendErr("abcdefghij")
	appendOut("KLMNOPQRST")
	appendErr("uvwxyz0123")

	recs, err := mgr.store.ReadStdioRecords(sess, shell)
	if err != nil {
		t.Fatalf("ReadStdioRecords: %v", err)
	}
	if len(recs) != 4 {
		t.Fatalf("switch records = %v, want 4 (one per change, not per chunk)", recs)
	}
	if recs[0] != 1 || recs[1] != -11 || recs[2] != 21 || recs[3] != -31 {
		t.Fatalf("records = %v, want [+1 -11 +21 -31] (sign=stream, |v|-1=offset)", recs)
	}

	ranges, err := mgr.StderrRanges(sess, shell)
	if err != nil {
		t.Fatalf("StderrRanges: %v", err)
	}
	want := [][2]int64{{10, 20}, {30, 40}}
	if len(ranges) != len(want) {
		t.Fatalf("stderr ranges = %v, want %v", ranges, want)
	}
	for i, r := range want {
		if ranges[i] != r {
			t.Fatalf("range %d = %v, want %v", i, ranges[i], r)
		}
	}
}

// A stream whose first write is stderr must still be described from offset 0:
// the +1 shift is what keeps a stderr switch at 0 (-1) distinct from a stdout
// one (+1).
func TestStderrRanges_StderrFromOffsetZero(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"

	if _, err := mgr.AppendStderr(sess, shell, []byte("ERR\n")); err != nil {
		t.Fatalf("AppendStderr: %v", err)
	}
	if _, err := mgr.AppendOutput(sess, shell, []byte("OUT\n")); err != nil {
		t.Fatalf("AppendOutput: %v", err)
	}

	recs, err := mgr.store.ReadStdioRecords(sess, shell)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0] != -1 || recs[1] != 5 {
		t.Fatalf("records = %v, want [-1 5] (stderr@0, stdout@4)", recs)
	}
	ranges, err := mgr.StderrRanges(sess, shell)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 1 || ranges[0] != [2]int64{0, 4} {
		t.Fatalf("stderr ranges = %v, want [[0 4]]", ranges)
	}
}

// Storage-level: the sink records a switch only when the producing stream
// changes, and a torn index tail is truncated on the next open.
//
// The torn tail is created the way only a crash can create it - after the
// process is gone: in-process writes are fixed-size and always aligned, so the
// test closes the store, tears the file, and reopens a fresh store over the
// same directory (the restart this recovery path exists for).
func TestStdioSwitches_OnlyOnChangeAndTornTailRecovers(t *testing.T) {
	dir := t.TempDir()
	store := storage.New(dir)
	const sess, shell = "s1", "sh1"

	append := func(data string, stream storage.StdioStream) {
		if _, err := store.AppendLog(sess, shell, []byte(data), stream); err != nil {
			t.Fatalf("AppendLog: %v", err)
		}
	}
	append("a", storage.StdioOut)
	append("b", storage.StdioOut) // same stream: no record
	append("c", storage.StdioErr)
	append("d", storage.StdioErr) // same stream: no record

	recs, err := store.ReadStdioRecords(sess, shell)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 2 || recs[0] != 1 || recs[1] != -3 {
		t.Fatalf("records = %v, want [+1 -3] (switches only)", recs)
	}

	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	if err := rawAppend(store.StdioPath(sess, shell), []byte{1, 2, 3}); err != nil {
		t.Fatalf("torn append: %v", err)
	}

	// The restart: a fresh store opens the torn file, truncates the partial
	// record, and the next switch realigns everything.
	restarted := storage.New(dir)
	t.Cleanup(func() { _ = restarted.Close() })
	if _, err := restarted.AppendLog(sess, shell, []byte("e"), storage.StdioOut); err != nil {
		t.Fatalf("AppendLog after restart: %v", err)
	}
	recs, err = restarted.ReadStdioRecords(sess, shell)
	if err != nil {
		t.Fatal(err)
	}
	if len(recs) != 3 || recs[0] != 1 || recs[1] != -3 || recs[2] != 5 {
		t.Fatalf("records after torn recovery = %v, want [+1 -3 +5] (realigned)", recs)
	}
}

// The shell_input / stream plumbing shares the offsets: an offset returned by
// an append must address the same byte the stream index saw.
func TestStderrRanges_OffsetsMatchAppends(t *testing.T) {
	mgr := newManager(t)
	const sess, shell = "s1", "sh1"

	off, err := mgr.AppendOutput(sess, shell, []byte("OUT\n"))
	if err != nil {
		t.Fatal(err)
	}
	if off != 0 {
		t.Fatalf("first offset = %d, want 0", off)
	}
	off, err = mgr.AppendStderr(sess, shell, []byte("ERR\n"))
	if err != nil {
		t.Fatal(err)
	}
	if off != 4 {
		t.Fatalf("stderr offset = %d, want 4", off)
	}
	ranges, err := mgr.StderrRanges(sess, shell)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranges) != 1 || ranges[0] != [2]int64{4, 8} {
		t.Fatalf("stderr ranges = %v, want [[4 8]]", ranges)
	}
}
