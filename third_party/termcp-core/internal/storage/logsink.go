package storage

import (
	"encoding/binary"
	"io"
	"log/slog"
	"os"
	"strings"
)

// sinkOp selects what the owner goroutine should do with one request.
type sinkOp uint8

const (
	opAppend sinkOp = iota
	opSyncLines
	opCloseShell
	opCloseSession
	opCloseAll
)

// sinkRequest is one unit of work for the owner. It is a value so a caller can
// set reply after the fields it knows; the reply channel is how the outcome gets
// back, since the owner is the one that can compute it.
type sinkRequest struct {
	op      sinkOp
	session string
	shell   string
	data    []byte
	// stream says which low-level producer the data came from (stdout or
	// stderr); the owner compares it against the shell's last chunk to decide
	// whether this append switches streams and needs a log.stdio record.
	stream StdioStream
	reply  chan sinkResult
}

type sinkResult struct {
	offset int64
	err    error
}

// logSink owns every cached append handle and performs every append.
//
// An append is two steps that must not be separated: the offset the bytes landed
// at has to be read from the file, and that offset is what the caller is told the
// stream position of its bytes is. If another write happens in between, two
// callers are handed the same offset and one of them is wrong about where its
// bytes live.
//
// A mutex can serialize that pair, and one did. It was the wrong tool here for the
// same reason the per-session lock in the message package was: the handles it
// guarded are opened on first append and closed on shell delete or shutdown, so
// the lock would have to be looked up in a map and would have a lifetime shorter
// than the files it protects. Deletion is where such a design stops working - a
// writer holding the old lock and one holding its successor can both write the
// same file - and no amount of locking fixes a lock that can be replaced.
//
// So the handles have no lock and no shared home: they are local variables of the
// goroutine below, which is the only code in the process that opens, writes,
// seeks or closes them, and every operation on them is a request to that
// goroutine. The offset is decided by the same goroutine that performed the write,
// which makes it true by construction rather than by holding something across
// both steps.
type logSink struct {
	store *Store
	reqs  chan sinkRequest
}

func newLogSink(store *Store) *logSink {
	sink := &logSink{store: store, reqs: make(chan sinkRequest)}
	go sink.run()
	return sink
}

// run owns the open handles and is the only goroutine that touches them.
//
// Each shell has three handles: log.bin (the byte log), log.lines (the line
// index) and log.stdio (the stream-switch index), plus a lastStream entry per
// shell so a switch record is only written when the producing stream actually
// changes. All of them live on this one goroutine, so a byte append, its line
// records and its stream switch cannot interleave with each other or with a
// concurrent shell's appends.
func (sink *logSink) run() {
	handles := make(map[string]*os.File)
	lineHandles := make(map[string]*os.File)
	stdioHandles := make(map[string]*os.File)
	lastStream := make(map[string]StdioStream)

	for req := range sink.reqs {
		switch req.op {
		case opAppend:
			f, err := sink.handle(handles, req.session, req.shell)
			if err != nil {
				req.reply <- sinkResult{err: err}
				continue
			}
			// The handle is O_APPEND, so the bytes land at the end of the file
			// whatever the position; this seek is only how that position is learned.
			off, err := f.Seek(0, io.SeekEnd)
			if err != nil {
				req.reply <- sinkResult{err: err}
				continue
			}
			if _, err := f.Write(req.data); err != nil {
				// The offset is reported anyway: the caller compares it against its
				// own view of the stream and needs to know where the gap starts.
				req.reply <- sinkResult{offset: off, err: err}
				continue
			}
			// The bytes are in log.bin, so this is the moment their line records
			// are due. A failed index write must not fail the byte append - the
			// index is advisory and a later SyncLineIndex repairs it - but it is
			// loud, because a silently missing record would look like a bug in
			// everything built on top.
			if err := sink.appendLineRecords(lineHandles, req.session, req.shell, lineStartsIn(req.data, off)); err != nil {
				slog.Warn("log.lines append failed; index will be repaired on next sync",
					"session", req.session, "shell", req.shell, "err", err)
			}
			// Same advisory contract for the stream-switch record: a chunk whose
			// switch was not recorded reads as stdout (the pre-#104 behaviour),
			// and the next chunk from the stderr stream retries the record.
			k := logKey(req.session, req.shell)
			if prev, seen := lastStream[k]; !seen || prev != req.stream {
				if err := sink.appendStdioRecord(stdioHandles, req.session, req.shell, off, req.stream); err != nil {
					slog.Warn("log.stdio switch record failed; stream reads as stdout until it succeeds",
						"session", req.session, "shell", req.shell, "err", err)
				} else {
					lastStream[k] = req.stream
				}
			}
			req.reply <- sinkResult{offset: off}

		case opSyncLines:
			err := sink.syncLines(lineHandles, req.session, req.shell)
			req.reply <- sinkResult{err: err}

		case opCloseShell:
			k := logKey(req.session, req.shell)
			if f, ok := handles[k]; ok {
				_ = f.Close()
				delete(handles, k)
			}
			if f, ok := lineHandles[k]; ok {
				_ = f.Close()
				delete(lineHandles, k)
			}
			if f, ok := stdioHandles[k]; ok {
				_ = f.Close()
				delete(stdioHandles, k)
			}
			delete(lastStream, k)
			req.reply <- sinkResult{}

		case opCloseSession:
			prefix := req.session + "\x00"
			for k, f := range handles {
				if strings.HasPrefix(k, prefix) {
					_ = f.Close()
					delete(handles, k)
				}
			}
			for k, f := range lineHandles {
				if strings.HasPrefix(k, prefix) {
					_ = f.Close()
					delete(lineHandles, k)
				}
			}
			for k, f := range stdioHandles {
				if strings.HasPrefix(k, prefix) {
					_ = f.Close()
					delete(stdioHandles, k)
				}
			}
			for k := range lastStream {
				if strings.HasPrefix(k, prefix) {
					delete(lastStream, k)
				}
			}
			req.reply <- sinkResult{}

		case opCloseAll:
			var firstErr error
			for k, f := range handles {
				if err := f.Close(); err != nil && firstErr == nil {
					firstErr = err
				}
				delete(handles, k)
			}
			for k, f := range lineHandles {
				if err := f.Close(); err != nil && firstErr == nil {
					firstErr = err
				}
				delete(lineHandles, k)
			}
			for k, f := range stdioHandles {
				if err := f.Close(); err != nil && firstErr == nil {
					firstErr = err
				}
				delete(stdioHandles, k)
			}
			lastStream = make(map[string]StdioStream)
			req.reply <- sinkResult{err: firstErr}
		}
	}
}

// lineStartsIn returns the absolute offsets of every '\n' in data, where data
// begins at absolute stream offset off. Each becomes one log.lines record.
func lineStartsIn(data []byte, off int64) []int64 {
	var starts []int64
	for i, c := range data {
		if c == '\n' {
			starts = append(starts, off+int64(i))
		}
	}
	return starts
}

// appendLineRecords writes one 8-byte little-endian record per newline offset.
// Runs on run's goroutine, which is why the handle map needs no lock.
func (sink *logSink) appendLineRecords(lineHandles map[string]*os.File, sessionID, shellID string, starts []int64) error {
	if len(starts) == 0 {
		return nil
	}
	f, err := sink.lineHandle(lineHandles, sessionID, shellID)
	if err != nil {
		return err
	}
	buf := make([]byte, len(starts)*lineRecordSize)
	for i, off := range starts {
		binary.LittleEndian.PutUint64(buf[i*lineRecordSize:], uint64(off))
	}
	_, err = f.Write(buf)
	return err
}

// lineHandle returns the cached log.lines append handle for a shell, opening it
// if needed. Opening repairs a torn tail first: a crash between record writes
// can leave a partial record, and appending after it would misalign every later
// record. The partial bytes are dropped, not interpreted; SyncLineIndex re-derives
// anything they might have covered from log.bin itself.
//
// The truncation happens through its own handle before the append handle opens:
// an O_APPEND handle carries FILE_APPEND_DATA on Windows, which SetEndOfFile
// needs FILE_WRITE_DATA to act on, so f.Truncate on the cached handle is
// refused. This runs on run's goroutine, so no append can slip into the
// truncate-open window.
func (sink *logSink) lineHandle(lineHandles map[string]*os.File, sessionID, shellID string) (*os.File, error) {
	k := logKey(sessionID, shellID)
	if f, ok := lineHandles[k]; ok {
		return f, nil
	}

	if err := sink.store.initLogDir(sessionID, shellID); err != nil {
		return nil, err
	}
	path := sink.store.LineIndexPath(sessionID, shellID)
	if st, err := os.Stat(path); err == nil {
		if torn := st.Size() % lineRecordSize; torn != 0 {
			if err := os.Truncate(path, st.Size()-torn); err != nil {
				return nil, err
			}
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	lineHandles[k] = f
	return f, nil
}

// syncLines brings one shell's line index up to its log.bin: scan the bytes
// after the last indexed newline, append a record per newline found. Runs on
// run's goroutine, so it cannot interleave with an append's byte write and its
// record write - that ordering is what makes a double-indexed line impossible.
func (sink *logSink) syncLines(lineHandles map[string]*os.File, sessionID, shellID string) error {
	size, err := sink.store.LogSize(sessionID, shellID)
	if err != nil || size == 0 {
		return err
	}
	last, err := sink.lastIndexedNewline(sessionID, shellID)
	if err != nil {
		return err
	}
	// Nothing new to index. The common case once the index has caught up: one
	// stat and one small read, no scan.
	if last == size-1 {
		return nil
	}
	from := last + 1
	if from < 0 {
		from = 0
	}
	const chunk = 4 << 20
	for pos := from; pos < size; {
		n := int64(chunk)
		if size-pos < n {
			n = size - pos
		}
		data, err := sink.store.ReadLog(sessionID, shellID, pos, int(n))
		if err != nil {
			return err
		}
		if len(data) == 0 {
			break
		}
		if err := sink.appendLineRecords(lineHandles, sessionID, shellID, lineStartsIn(data, pos)); err != nil {
			return err
		}
		pos += int64(len(data))
	}
	return nil
}

// lastIndexedNewline returns the stream offset of the last newline the index
// holds, or -1 when the index holds no record (empty stream, missing file, or a
// log whose first line is not yet terminated).
func (sink *logSink) lastIndexedNewline(sessionID, shellID string) (int64, error) {
	st, err := os.Stat(sink.store.LineIndexPath(sessionID, shellID))
	if err != nil {
		if os.IsNotExist(err) {
			return -1, nil
		}
		return 0, err
	}
	records := st.Size() / lineRecordSize
	if records == 0 {
		return -1, nil
	}
	recs, err := sink.store.ReadLineRecords(sessionID, shellID, records-1, 1)
	if err != nil || len(recs) == 0 {
		return -1, err
	}
	return recs[0], nil
}

// appendStdioRecord writes one stream-switch record: the offset where a run of
// `stream` bytes begins, packed as ±(offset+1) so the sign carries the type and
// a stderr switch at offset 0 is still distinguishable from a stdout one.
// Runs on run's goroutine.
func (sink *logSink) appendStdioRecord(stdioHandles map[string]*os.File, sessionID, shellID string, offset int64, stream StdioStream) error {
	f, err := sink.stdioHandle(stdioHandles, sessionID, shellID)
	if err != nil {
		return err
	}
	var buf [lineRecordSize]byte
	binary.LittleEndian.PutUint64(buf[:], uint64(stdioRecord(offset, stream)))
	_, err = f.Write(buf[:])
	return err
}

// stdioHandle returns the cached log.stdio append handle for a shell, opening
// it if needed. Opening repairs a torn tail first: a crash between record
// writes can leave a partial record, and appending after it would misalign
// every later record. The partial bytes are dropped, not interpreted.
func (sink *logSink) stdioHandle(stdioHandles map[string]*os.File, sessionID, shellID string) (*os.File, error) {
	k := logKey(sessionID, shellID)
	if f, ok := stdioHandles[k]; ok {
		return f, nil
	}

	if err := sink.store.initLogDir(sessionID, shellID); err != nil {
		return nil, err
	}
	path := sink.store.StdioPath(sessionID, shellID)
	if st, err := os.Stat(path); err == nil {
		if torn := st.Size() % lineRecordSize; torn != 0 {
			if err := os.Truncate(path, st.Size()-torn); err != nil {
				return nil, err
			}
		}
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	stdioHandles[k] = f
	return f, nil
}

// handle returns the cached append handle for a shell, opening it if needed.
// Runs on run's goroutine, which is why handles needs no lock.
func (sink *logSink) handle(handles map[string]*os.File, sessionID, shellID string) (*os.File, error) {
	k := logKey(sessionID, shellID)
	if f, ok := handles[k]; ok {
		return f, nil
	}

	if err := sink.store.initLogDir(sessionID, shellID); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(sink.store.LogPath(sessionID, shellID), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}
	handles[k] = f
	return f, nil
}

// do sends one request and waits for the owner to perform it. Waiting is what
// makes the request's data safe to reuse afterwards: the bytes have been written
// by the time this returns.
func (sink *logSink) do(req sinkRequest) sinkResult {
	// Buffered so the owner can always answer, even if this caller gives up.
	req.reply = make(chan sinkResult, 1)
	sink.reqs <- req
	return <-req.reply
}

func (sink *logSink) append(sessionID, shellID string, data []byte, stream StdioStream) (int64, error) {
	res := sink.do(sinkRequest{op: opAppend, session: sessionID, shell: shellID, data: data, stream: stream})
	return res.offset, res.err
}

func (sink *logSink) closeShell(sessionID, shellID string) {
	sink.do(sinkRequest{op: opCloseShell, session: sessionID, shell: shellID})
}

func (sink *logSink) closeSession(sessionID string) {
	sink.do(sinkRequest{op: opCloseSession, session: sessionID})
}

func (sink *logSink) closeAll() error {
	return sink.do(sinkRequest{op: opCloseAll}).err
}

// logKey identifies a shell's log for the handle cache. The NUL separator cannot
// appear in an ID (see validateID), so no two shells can share a key.
func logKey(sessionID, shellID string) string {
	return sessionID + "\x00" + shellID
}
