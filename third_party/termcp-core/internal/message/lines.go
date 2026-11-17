package message

import "fmt"

// Line addressing over the transcript's line index (log.lines, see
// storage/log.go). Lines are 1-based, counted on the raw byte stream, and
// numbered only where they are complete: a trailing partial line (bytes after
// the last '\n') has no number, so a line number never changes meaning later.
// All offsets are absolute log.bin positions, the same numbering every other
// read tool uses.

// ErrLinesUnavailable reports that line addressing was requested on a transcript
// with no persistence behind it: there is no log, so there is nothing to index.
var ErrLinesUnavailable = errLines{}

type errLines struct{}

func (errLines) Error() string {
	return "line addressing unavailable: transcript has no persistent log"
}

// syncLines brings a shell's line index up to date with its log before any
// query answers from it. The store serializes the sync against every append,
// so a query can never see the index mid-update, and a legacy or crashed index
// is repaired by the first query that needs it.
func (m *Manager) syncLines(sessionID, shellID string) error {
	if m.store == nil {
		return ErrLinesUnavailable
	}
	return m.store.SyncLineIndex(sessionID, shellID)
}

// LineCount returns the number of complete lines in a shell's byte stream. The
// trailing partial line, if any, is not counted.
func (m *Manager) LineCount(sessionID, shellID string) (int64, error) {
	if m.store == nil {
		return 0, ErrLinesUnavailable
	}
	if err := m.syncLines(sessionID, shellID); err != nil {
		return 0, err
	}
	return m.store.LineRecordCount(sessionID, shellID)
}

// LineBounds returns the half-open byte range [start, end) of line number line
// (1-based). The range includes the terminating '\n', so reading it reproduces
// the line exactly as it sits in the stream.
func (m *Manager) LineBounds(sessionID, shellID string, line int64) (int64, int64, error) {
	if m.store == nil {
		return 0, 0, ErrLinesUnavailable
	}
	if line < 1 {
		return 0, 0, errLineOutOfRange(line, 0)
	}
	if err := m.syncLines(sessionID, shellID); err != nil {
		return 0, 0, err
	}
	total, err := m.store.LineRecordCount(sessionID, shellID)
	if err != nil {
		return 0, 0, err
	}
	if line > total {
		return 0, 0, errLineOutOfRange(line, total)
	}
	recs, err := m.store.ReadLineRecords(sessionID, shellID, line-1, 1)
	if err != nil {
		return 0, 0, err
	}
	if len(recs) == 0 {
		return 0, 0, errLineOutOfRange(line, total)
	}
	end := recs[0] + 1
	var start int64
	if line > 1 {
		prev, err := m.store.ReadLineRecords(sessionID, shellID, line-2, 1)
		if err != nil {
			return 0, 0, err
		}
		if len(prev) == 0 {
			return 0, 0, errLineOutOfRange(line, total)
		}
		start = prev[0] + 1
	}
	return start, end, nil
}

// LineRecords returns the raw line contents of lines [from, to] (1-based,
// inclusive, to >= from) as byte slices, each including its '\n'.
func (m *Manager) LineRecords(sessionID, shellID string, from, to int64) ([][]byte, error) {
	if m.store == nil {
		return nil, ErrLinesUnavailable
	}
	if from < 1 || to < from {
		return nil, nil
	}
	if err := m.syncLines(sessionID, shellID); err != nil {
		return nil, err
	}
	total, err := m.store.LineRecordCount(sessionID, shellID)
	if err != nil {
		return nil, err
	}
	if from > total {
		return nil, errLineOutOfRange(from, total)
	}
	if to > total {
		to = total
	}
	// Records needed: from-2 (0-based, giving line `from` its start) through
	// to-1 (ending line `to`). Line 1 has no predecessor record; its start is 0.
	first := from - 2
	if first < 0 {
		first = 0
	}
	recs, err := m.store.ReadLineRecords(sessionID, shellID, first, int(to-first))
	if err != nil {
		return nil, err
	}
	// Walking the records once gives every line's start and end without
	// per-line reads. prevEnd tracks the '\n' before the line being emitted.
	var out [][]byte
	prevEnd := int64(-1) // end offset of the line before `from`; -1 = stream start
	idx := 0
	if from > 1 {
		if len(recs) == 0 {
			return nil, errLineOutOfRange(from, total)
		}
		prevEnd = recs[0]
		idx = 1
	}
	for line := from; line <= to; line++ {
		if idx >= len(recs) {
			break
		}
		end := recs[idx]
		idx++
		start := prevEnd + 1
		raw, err := m.store.ReadLog(sessionID, shellID, start, int(end-start+1))
		if err != nil {
			return nil, err
		}
		out = append(out, raw)
		prevEnd = end
	}
	return out, nil
}

// LineStartOffset returns the absolute byte offset where line `line` (1-based)
// begins. The caller is responsible for the line existing; LineBounds is the
// checked variant.
func (m *Manager) LineStartOffset(sessionID, shellID string, line int64) (int64, error) {
	if m.store == nil {
		return 0, ErrLinesUnavailable
	}
	if line <= 1 {
		return 0, nil
	}
	recs, err := m.store.ReadLineRecords(sessionID, shellID, line-2, 1)
	if err != nil {
		return 0, err
	}
	if len(recs) == 0 {
		return 0, errLineOutOfRange(line, 0)
	}
	return recs[0] + 1, nil
}

// LineSpanOfOffsets maps a byte range [start, end) onto line numbers. It is how
// transcript spans (message) answer "which lines is this?".
//
//   - end > start: from_line is the line containing byte start (0 when start
//     falls in the trailing partial line or past the end). to_line is the
//     absolute number of the last complete line whose '\n' lies inside the
//     range - 0 when the range completes no line (it ends mid-line, or lies
//     entirely in the partial tail, or past the end of the stream).
//   - end == start (a position, e.g. a zero-length input mark): both numbers
//     name the line that position sits on - or is about to become, for a mark
//     at a line start - and 0 when that is the trailing partial line, the
//     stream start, or past the end.
func (m *Manager) LineSpanOfOffsets(sessionID, shellID string, start, end int64) (int64, int64, error) {
	if m.store == nil {
		return 0, 0, ErrLinesUnavailable
	}
	if err := m.syncLines(sessionID, shellID); err != nil {
		return 0, 0, err
	}
	total, err := m.store.LineRecordCount(sessionID, shellID)
	if err != nil {
		return 0, 0, err
	}
	if total == 0 {
		return 0, 0, nil
	}
	last, err := m.lineRecord(sessionID, shellID, total-1)
	if err != nil {
		return 0, 0, err
	}
	// Byte `start` belongs to a complete line only while it is at or before the
	// last indexed '\n'; past that lies the unnumbered partial tail. A non-empty
	// range always has a real byte at `start` (offset 0 included); only the
	// zero-length form below has no byte to sit on at 0.
	beforeStart, err := m.recordsBefore(sessionID, shellID, total, start)
	if err != nil {
		return 0, 0, err
	}
	if end == start {
		if start <= 0 || start > last {
			return 0, 0, nil
		}
		return beforeStart + 1, beforeStart + 1, nil
	}
	var fromLine int64
	if start <= last {
		fromLine = beforeStart + 1
	}
	beforeEnd, err := m.recordsBefore(sessionID, shellID, total, end)
	if err != nil {
		return 0, 0, err
	}
	// beforeEnd counts every complete line ending before `end`, including ones
	// that ended before the span even began; only when the span swallowed at
	// least one more newline does a line lie inside it.
	var toLine int64
	if beforeEnd > beforeStart {
		toLine = beforeEnd
	}
	return fromLine, toLine, nil
}

// lineRecord returns record i (0-based): the offset of the '\n' ending line i+1.
func (m *Manager) lineRecord(sessionID, shellID string, i int64) (int64, error) {
	recs, err := m.store.ReadLineRecords(sessionID, shellID, i, 1)
	if err != nil {
		return 0, err
	}
	if len(recs) == 0 {
		return 0, errLineOutOfRange(i+1, 0)
	}
	return recs[0], nil
}

// recordsBefore counts records strictly below offset x - the number of complete
// lines ending before x - by binary search over the index file. Records are
// strictly increasing, so the count is unambiguous.
func (m *Manager) recordsBefore(sessionID, shellID string, total, x int64) (int64, error) {
	lo, hi := int64(0), total
	for lo < hi {
		mid := (lo + hi) / 2
		rec, err := m.lineRecord(sessionID, shellID, mid)
		if err != nil {
			return 0, err
		}
		if rec < x {
			lo = mid + 1
		} else {
			hi = mid
		}
	}
	return lo, nil
}

func errLineOutOfRange(line, total int64) error {
	return &LineOutOfRangeError{Line: line, Total: total}
}

// LineOutOfRangeError is the one wording for "that line number names no line".
// The stream is never truncated, so this can only mean the caller asked past
// the lines that exist (or before the first one).
type LineOutOfRangeError struct {
	Line  int64
	Total int64
}

func (e *LineOutOfRangeError) Error() string {
	return fmt.Sprintf("line %d out of range (stream has %d complete line(s))", e.Line, e.Total)
}

// LineSpan is one line's number and its half-open byte range in the raw
// stream (end exclusive, i.e. the range includes the terminating newline).
type LineSpan struct {
	N     int64
	Start int64
	End   int64
}

// LineSpans returns the spans of lines [from, to] (1-based, inclusive). It is
// the batch form of LineBounds: one index read prices the whole range, which
// is what a REST lines endpoint needs to answer a page without per-line reads.
func (m *Manager) LineSpans(sessionID, shellID string, from, to int64) ([]LineSpan, error) {
	if m.store == nil {
		return nil, ErrLinesUnavailable
	}
	if from < 1 || to < from {
		return nil, nil
	}
	if err := m.syncLines(sessionID, shellID); err != nil {
		return nil, err
	}
	total, err := m.store.LineRecordCount(sessionID, shellID)
	if err != nil {
		return nil, err
	}
	if from > total {
		return nil, errLineOutOfRange(from, total)
	}
	if to > total {
		to = total
	}
	// Records from-2 (0-based, giving line `from` its start) through to-1
	// (ending line `to`); line 1 has no predecessor record and starts at 0.
	first := from - 2
	if first < 0 {
		first = 0
	}
	recs, err := m.store.ReadLineRecords(sessionID, shellID, first, int(to-first))
	if err != nil {
		return nil, err
	}
	out := make([]LineSpan, 0, to-from+1)
	prevEnd := int64(-1)
	idx := 0
	if from > 1 {
		if len(recs) == 0 {
			return nil, errLineOutOfRange(from, total)
		}
		prevEnd = recs[0]
		idx = 1
	}
	for line := from; line <= to; line++ {
		if idx >= len(recs) {
			break
		}
		end := recs[idx]
		idx++
		out = append(out, LineSpan{N: line, Start: prevEnd + 1, End: end + 1})
		prevEnd = end
	}
	return out, nil
}
