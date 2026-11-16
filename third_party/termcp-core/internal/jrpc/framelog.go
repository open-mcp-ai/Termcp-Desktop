package jrpc

import (
	"sync"
	"time"
)

// Framed is one frame the channel received, with the sequence number the
// channel gave it.
//
// THE NUMBER IS THE POINT. CDP has no event replay: once a peer pushes an event
// it is gone, so a reader that was not looking at that moment can never ask for
// it again. A consumer that wants "everything that happened while I clicked" —
// which is the only way to wait for a page load without guessing a sleep — has to
// keep its own window of history and be able to resume exactly. A byte offset
// cannot do that (the ring truncates from the front, so every offset shifts); a
// monotonic number can, because `since n` means the same thing before and after
// truncation.
type Framed struct {
	N  int64
	At time.Time
	// Data is the frame as it arrived, one line, verbatim.
	Data string
	// Event reports that the frame carries no id: the peer pushed it rather
	// than answering anything. An answer is history too — it is what a caller
	// that lost the reply needs to find again — but the distinction is what
	// lets a reader ask for "just the events".
	Event bool
}

// FrameLogStats describes the window the log currently holds.
type FrameLogStats struct {
	Seq      int64 // frames received so far (= the latest sequence number)
	Size     int   // frames retained
	Capacity int   // frames retained before the oldest is dropped
	Dropped  int64 // frames dropped for exceeding the capacity
}

// frameLog is a bounded ring of received frames with monotonic sequence numbers.
//
// Bounded in BOTH directions (count and bytes) because one CDP frame can be a
// 2 MB screenshot: a count-only cap of 2048 would hold gigabytes after a page
// that streams responses, while a byte-only cap would hold two frames and call it
// history. Whichever limit trips first drops the oldest.
type frameLog struct {
	mu       sync.Mutex
	items    []Framed
	seq      int64
	dropped  int64
	bytes    int
	maxItems int
	maxBytes int
}

// newFrameLog returns a ring that keeps at most maxItems frames or maxBytes of
// them, whichever fills first.
func newFrameLog(maxItems, maxBytes int) *frameLog {
	if maxItems <= 0 {
		maxItems = 2048
	}
	if maxBytes <= 0 {
		maxBytes = 8 << 20
	}
	return &frameLog{maxItems: maxItems, maxBytes: maxBytes}
}

// add stamps a frame with the next sequence number and retains it.
func (l *frameLog) add(data string, event bool) Framed {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.seq++
	rec := Framed{N: l.seq, At: time.Now(), Data: data, Event: event}
	l.items = append(l.items, rec)
	l.bytes += len(data)
	for len(l.items) > 1 && (len(l.items) > l.maxItems || l.bytes > l.maxBytes) {
		l.dropped++
		l.bytes -= len(l.items[0].Data)
		l.items = l.items[1:]
	}
	return rec
}

// since returns the frames with N > since, oldest first, at most limit of them.
//
// LIMIT KEEPS THE FIRST ONES, NOT THE LAST. A reader that asks for fewer than
// are available resumes from the highest number it received, so a forward page is
// lossless; returning the newest instead would silently skip the middle of the
// window, which is the failure mode the cursor exists to prevent.
func (l *frameLog) since(since int64, keep func(Framed) bool, limit int) ([]Framed, FrameLogStats) {
	l.mu.Lock()
	defer l.mu.Unlock()
	out := make([]Framed, 0, len(l.items))
	for _, rec := range l.items {
		if rec.N <= since {
			continue
		}
		if keep != nil && !keep(rec) {
			continue
		}
		out = append(out, rec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, l.statsLocked()
}

// stats reports the window without reading frames from it.
func (l *frameLog) stats() FrameLogStats {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.statsLocked()
}

func (l *frameLog) statsLocked() FrameLogStats {
	return FrameLogStats{Seq: l.seq, Size: len(l.items), Capacity: l.maxItems, Dropped: l.dropped}
}

// History returns the frames with a sequence number greater than since, oldest
// first, at most limit of them (0 = no limit).
//
// It does NOT consume them: the destructive read (`read <peer>`) advances its own
// cursor, and this is the other question — "what happened after n" — which must
// stay answerable twice. A caller that gets 0 frames back is told the latest
// number anyway, so "nothing new yet" and "I am behind by 30" are different
// answers rather than the same emptiness.
func (c *Channel) History(since int64, keep func(Framed) bool, limit int) ([]Framed, FrameLogStats) {
	if c.log == nil {
		return nil, FrameLogStats{}
	}
	return c.log.since(since, keep, limit)
}

// Latest reports the newest sequence number this channel has received. It is what
// a caller records before an action and passes as `since` after it.
func (c *Channel) Latest() int64 {
	if c.log == nil {
		return 0
	}
	return c.log.stats().Seq
}

// record files one received frame in the history. Called at the wire, once per
// frame: the buffered/destructive paths hand the same record around, and recording
// there too would number each of them twice.
func (c *Channel) record(payload string) Framed {
	if c.log == nil || payload == "" {
		return Framed{Data: payload}
	}
	return c.log.add(payload, !hasID(payload))
}

// LastAnswer returns the newest frame that is an answer rather than an event.
//
// It exists for the receipt an oversized reply hands back: "this frame is 300 KB,
// here is its number" is only useful if the number is the one the caller can page
// from, and the answer is not necessarily the newest frame on the wire — a peer that
// interleaves events with answers leaves an event on top.
func (c *Channel) LastAnswer() (Framed, bool) {
	if c.log == nil {
		return Framed{}, false
	}
	c.log.mu.Lock()
	defer c.log.mu.Unlock()
	for i := len(c.log.items) - 1; i >= 0; i-- {
		if !c.log.items[i].Event {
			return c.log.items[i], true
		}
	}
	return Framed{}, false
}

// Frame returns one recorded frame by its number.
//
// It is how a paged read reaches the payload a receipt named: `--at n --offset b`
// is the same log `--since n` pages, indexed by position instead of by time.
func (c *Channel) Frame(n int64) (Framed, bool) {
	if c.log == nil {
		return Framed{}, false
	}
	c.log.mu.Lock()
	defer c.log.mu.Unlock()
	for _, rec := range c.log.items {
		if rec.N == n {
			return rec, true
		}
	}
	return Framed{}, false
}
