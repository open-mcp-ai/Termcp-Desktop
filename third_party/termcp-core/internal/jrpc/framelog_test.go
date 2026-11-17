package jrpc

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestFrameLogResumesFromACursor pins the property the event stream exists for:
// `since n` returns exactly what arrived after n, in order, and asking twice gives
// the same answer — the frame is not consumed by reading it.
func TestFrameLogResumesFromACursor(t *testing.T) {
	log := newFrameLog(100, 1<<20)
	for _, f := range []string{`{"method":"A"}`, `{"method":"B"}`, `{"method":"C"}`} {
		log.add(f, true)
	}
	all, stats := log.since(0, nil, 0)
	if len(all) != 3 || stats.Seq != 3 {
		t.Fatalf("since(0) = %d frames, stats.Seq = %d; want 3 and 3", len(all), stats.Seq)
	}
	for i, rec := range all {
		if rec.N != int64(i+1) {
			t.Fatalf("frame %d has n=%d, want %d", i, rec.N, i+1)
		}
	}
	tail, _ := log.since(2, nil, 0)
	if len(tail) != 1 || tail[0].N != 3 || !strings.Contains(tail[0].Data, `"C"`) {
		t.Fatalf("since(2) = %+v, want only the third frame", tail)
	}
	again, _ := log.since(0, nil, 0)
	if len(again) != 3 {
		t.Fatal("reading the history consumed it")
	}
}

// TestFrameLogLimitKeepsTheOldest is what makes paging lossless: a reader that gets
// a truncated page resumes from the last number it saw, so returning the newest
// instead would silently skip the middle of the window.
func TestFrameLogLimitKeepsTheOldest(t *testing.T) {
	log := newFrameLog(100, 1<<20)
	for i := 0; i < 5; i++ {
		log.add(`{"method":"x"}`, true)
	}
	page, _ := log.since(0, nil, 2)
	if len(page) != 2 || page[0].N != 1 || page[1].N != 2 {
		t.Fatalf("limit 2 returned %+v; want the first two", page)
	}
}

// TestFrameLogDropsOldestButKeepsCounting pins that truncation does not renumber:
// a cursor taken before the drop still means the same thing afterwards.
func TestFrameLogDropsOldestButKeepsCounting(t *testing.T) {
	log := newFrameLog(3, 1<<20)
	for i := 0; i < 10; i++ {
		log.add(`{"method":"x"}`, true)
	}
	_, stats := log.since(0, nil, 0)
	if stats.Seq != 10 {
		t.Fatalf("stats.Seq = %d, want 10", stats.Seq)
	}
	if stats.Size > 3 {
		t.Fatalf("retained %d frames, capacity is 3", stats.Size)
	}
	if stats.Dropped != 7 {
		t.Fatalf("Dropped = %d, want 7", stats.Dropped)
	}
	// A cursor from before the drop still reads the surviving window.
	rest, _ := log.since(5, nil, 0)
	if len(rest) != 3 || rest[0].N != 8 {
		t.Fatalf("since(5) = %+v; want frames 8..10", rest)
	}
}

// TestFrameLogKeepsTheNewestEvenAboveTheByteBudget: one oversized frame must not
// evict itself, or a single 2 MB screenshot would empty the whole history.
func TestFrameLogKeepsTheNewestEvenAboveTheByteBudget(t *testing.T) {
	log := newFrameLog(100, 10)
	log.add(strings.Repeat("a", 100), true)
	if stats := log.stats(); stats.Size != 1 {
		t.Fatalf("the oversized frame evicted itself: %+v", stats)
	}
}

// TestChannelRecordsEveryFrameAtTheWire pins that the history is not "what was
// still queued": a frame consumed by a call is still numbered and readable.
func TestChannelRecordsEveryFrameAtTheWire(t *testing.T) {
	srv, _ := wsPeer(t)
	ch, err := Open(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http"), localDial)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()

	if _, err := ch.Call(t.Context(), map[string]any{"method": "Ping"}); err != nil {
		t.Fatal(err)
	}
	if got := ch.Latest(); got < 2 {
		t.Fatalf("Latest() = %d after one event and one answer; want 2", got)
	}
	// Wait for the wire reader to have recorded both frames, then read the window.
	frames, stats := ch.History(0, nil, 0)
	if len(frames) < 2 {
		t.Fatalf("history holds %d frames, want the event and the answer", len(frames))
	}
	if frames[0].N >= frames[1].N {
		t.Fatalf("history is not ordered: %d then %d", frames[0].N, frames[1].N)
	}
	// The event is the id-less frame; the answer carries an id.
	var events int
	for _, f := range frames {
		if f.Event {
			events++
		}
	}
	if events == 0 {
		t.Fatal("the event was recorded as an answer")
	}
	if stats.Seq != ch.Latest() {
		t.Fatalf("stats.Seq = %d, Latest = %d", stats.Seq, ch.Latest())
	}
}

// TestChannelHistoryKeepsReadingAfterAWindowExpires: a caller that records the
// latest number, acts, and asks for the tail must get the tail even though the
// frames arrived while nothing was waiting.
func TestChannelHistoryKeepsReadingAfterAWindowExpires(t *testing.T) {
	srv, _ := wsPeer(t)
	ch, err := Open(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http"), localDial)
	if err != nil {
		t.Fatal(err)
	}
	defer ch.Close()

	cursor := ch.Latest()
	if err := ch.Send(t.Context(), map[string]any{"method": "Ping"}); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Second)
	for {
		frames, _ := ch.History(cursor, func(f Framed) bool { return f.Event }, 0)
		if len(frames) > 0 {
			if !strings.Contains(frames[0].Data, "Page.lifecycleEvent") {
				t.Fatalf("the event window holds %q", frames[0].Data)
			}
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("an event that arrived while nothing was waiting never reached the history")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

// TestHistoryOnAClosedChannelStillAnswers: the window is data, not a live read,
// so closing the channel must not erase what already arrived.
func TestHistoryOnAClosedChannelStillAnswers(t *testing.T) {
	srv, _ := wsPeer(t)
	ch, err := Open(t.Context(), "ws"+strings.TrimPrefix(srv.URL, "http"), localDial)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ch.Call(t.Context(), map[string]any{"method": "Ping"}); err != nil {
		t.Fatal(err)
	}
	_ = ch.Close()
	frames, _ := ch.History(0, nil, 0)
	if len(frames) == 0 {
		t.Fatal("closing the channel erased its history")
	}
	_ = context.Background()
}

// TestLastAnswerSkipsEventsOnTop: the receipt an oversized reply prints names the
// frame to page from, and the newest frame is often an event the peer pushed after
// answering. Returning that one would page the caller into the wrong frame.
func TestLastAnswerSkipsEventsOnTop(t *testing.T) {
	c := &Channel{log: newFrameLog(100, 1<<20)}
	c.record("{\"id\":1,\"result\":{}}")
	c.record("{\"method\":\"Page.load\",\"params\":{}}")

	got, ok := c.LastAnswer()
	if !ok {
		t.Fatal("no answer found")
	}
	if got.N != 1 || got.Event {
		t.Errorf("LastAnswer = #%d event=%v, want #1 event=false", got.N, got.Event)
	}
	if _, ok := (&Channel{}).LastAnswer(); ok {
		t.Error("a channel with no log claimed to have an answer")
	}
}
