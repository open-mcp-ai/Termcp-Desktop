package transcript

import (
	"testing"

	"github.com/open-mcp-ai/termcp/internal/message"
	"github.com/open-mcp-ai/termcp/internal/session"
	"github.com/open-mcp-ai/termcp/internal/storage"
)

// TestALiveSourceIsNeverShorterThanItsLog pins the two records of one stream to one
// answer.
//
// A live shell's bytes are written TWICE, and in this order on purpose: the durable log
// first (crash safety), the in-memory buffer second (positioned by the offset the log
// returned). Readers are split across the two — `--from-line` and `--grep` read the log,
// `--tail` and `--offset` read the buffer — so between those two writes the same channel
// answered two different lengths. With the gap widened to 1.5s on Linux:
//
//	read <ch> --from-line 1    200, the bytes
//	read <ch> --tail 20        204, Content-Range: bytes */0
//
// `bytes */0` is a false fact about a channel that has them, and it is what made a GBK
// test fail on one CI runner and pass on another.
//
// The source here is a live shell whose buffer is BEHIND its log, which is exactly the
// state the write order produces for a few microseconds per chunk. The buffer is empty
// rather than short because a ChildShell with no buffer reports length 0 — the same
// answer the runner saw.
func TestALiveSourceIsNeverShorterThanItsLog(t *testing.T) {
	store := storage.New(t.TempDir())
	msgMgr := message.NewManager(store)
	t.Cleanup(func() {
		// The log file stays open under the manager, and Windows refuses to remove a
		// directory holding an open file — so the cleanup is part of the fixture.
		msgMgr.ForgetSession("s1")
		_ = store.Close()
	})

	const sessID, shellID = "s1", "sh1"
	payload := []byte{0xD2, 0xD4, 0xCF, 0xC2, 0x0A} // 以下 in GBK, plus a newline
	if _, err := msgMgr.AppendOutput(sessID, shellID, payload); err != nil {
		t.Fatalf("append: %v", err)
	}

	// The live record: a shell whose buffer has not been written yet.
	src := &Source{Live: &session.ChildShell{}, SessID: sessID, ShellID: shellID, MsgMgr: msgMgr}

	if got := src.Live.BufferLen(); got != 0 {
		t.Fatalf("this test is about a buffer that is BEHIND the log; it holds %d bytes", got)
	}
	n, err := src.Len()
	if err != nil {
		t.Fatalf("Len: %v", err)
	}
	if n != int64(len(payload)) {
		t.Errorf("Len = %d, want %d: the log is the record and the buffer is a cache, so the length cannot be the cache's", n, len(payload))
	}
	// And the bytes themselves, not only the count: a length without its bytes is a
	// range the caller cannot read.
	got, total, err := src.ByteRange(0, len(payload))
	if err != nil {
		t.Fatalf("ByteRange: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("ByteRange = %x, want %x", got, payload)
	}
	if total != int64(len(payload)) {
		t.Errorf("ByteRange total = %d, want %d", total, len(payload))
	}
}

// TestABufferThatDroppedItsHeadIsAnsweredByTheLog is the other end of the same
// disagreement.
//
// The buffer drops a prefix once it is over its threshold and every reader has passed
// it — and with no reader at all it drops everything, because "every reader has passed
// it" is vacuously true. The log is not compacted, so those bytes are still the record's.
// A source that reported the buffer's base as the stream's start would tell a caller
// that bytes it can read are gone, and would put a start in Content-Range that nobody
// asked for.
//
// The state is built without a buffer at all, which is the extreme of the same thing:
// a ChildShell whose buffer holds nothing reports base 0 and serves nothing, and the
// log has to answer both.
func TestABufferThatDroppedItsHeadIsAnsweredByTheLog(t *testing.T) {
	store := storage.New(t.TempDir())
	msgMgr := message.NewManager(store)
	t.Cleanup(func() {
		msgMgr.ForgetSession("s2")
		_ = store.Close()
	})

	const sessID, shellID = "s2", "sh2"
	payload := []byte("first line\nsecond line\n")
	if _, err := msgMgr.AppendOutput(sessID, shellID, payload); err != nil {
		t.Fatalf("append: %v", err)
	}
	src := &Source{Live: &session.ChildShell{}, SessID: sessID, ShellID: shellID, MsgMgr: msgMgr}

	// The record starts at zero whatever the cache did, so a caller is never told that
	// bytes the log holds are unreachable.
	if got := src.BaseOffset(); got != 0 {
		t.Errorf("BaseOffset = %d, want 0: the log is the record and it starts at zero", got)
	}
	// And a window at zero is served, whole, from the record.
	got, total, err := src.ByteRange(0, len(payload))
	if err != nil {
		t.Fatalf("ByteRange: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("ByteRange(0) = %q, want %q", got, payload)
	}
	if total != int64(len(payload)) {
		t.Errorf("total = %d, want %d", total, len(payload))
	}
	// A window that starts in the middle is not shifted either.
	mid, _, err := src.ByteRange(6, 4)
	if err != nil {
		t.Fatalf("ByteRange(6): %v", err)
	}
	if string(mid) != "line" {
		t.Errorf("ByteRange(6,4) = %q, want %q", mid, "line")
	}
}

// TestALiveSourceWithNoLogStillAnswersFromTheBuffer is the boundary of the rule above.
//
// With no log there is no second record, so the buffer IS the record: a source must not
// invent bytes from a log that is not there, and a buffer that cannot serve keeps
// saying so. This pins that the log-filling above did not turn every live source into
// one that answers with bytes it does not have.
func TestALiveSourceWithNoLogStillAnswersFromTheBuffer(t *testing.T) {
	src := &Source{Live: &session.ChildShell{}, SessID: "s3", ShellID: "sh3"}
	if _, _, err := src.ByteRange(0, 16); err == nil {
		t.Error("a live source with no log and no buffer answered with bytes it cannot have")
	}
	// And its base is the buffer's own, because nothing else can serve the bytes before
	// it — reporting zero here would promise a prefix no reader could get.
	if got, want := src.BaseOffset(), src.Live.OutputBaseOffset(); got != want {
		t.Errorf("BaseOffset = %d, want the buffer's %d when there is no log", got, want)
	}
}
