package storage

import (
	"encoding/binary"
	"encoding/json"
	"io"
	"os"
	"path/filepath"

	"github.com/open-mcp-ai/termcp/pkg/api"
)

// AppendLog appends raw bytes to a shell's log.bin and returns the offset the
// data was written at.
//
// The returned offset is the file position, which is what makes an offset
// meaningful without any bookkeeping: it is a fact about the file, not a sum of
// lengths that can drift out of agreement with the bytes.
//
// stream says which low-level producer the bytes came from (StdioOut or
// StdioErr): the sink uses it to maintain the stream-switch index, recording
// only when the producing stream changes. The bytes themselves all live in the
// one log.bin regardless of stream.
//
// The append happens through a cached handle so a burst of small writes does not
// pay an open() per write. Callers must AppendMark *after* this returns so a
// crash can only ever leave a trailing span with no mark (attributed to the
// previous status), never a mark pointing past the end of the data.
func (s *Store) AppendLog(sessionID, shellID string, data []byte, stream StdioStream) (int64, error) {
	if err := validateID(sessionID); err != nil {
		return 0, err
	}
	if err := validateID(shellID); err != nil {
		return 0, err
	}
	if len(data) == 0 {
		return s.LogSize(sessionID, shellID)
	}
	return s.logSink.append(sessionID, shellID, data, stream)
}

// AppendMark appends one status transition to a shell's log.jsonl.
//
// Append this after the corresponding AppendLog: the mark then always points at
// a byte that exists. The reverse order would produce marks pointing past EOF,
// which cannot be repaired.
func (s *Store) AppendMark(sessionID, shellID string, mark api.LogMark) error {
	if err := validateID(sessionID); err != nil {
		return err
	}
	if err := validateID(shellID); err != nil {
		return err
	}
	dir := s.shellDir(sessionID, shellID)
	if err := s.initLogDir(sessionID, shellID); err != nil {
		return err
	}
	line, err := json.Marshal(mark)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	// Marks are a handful of bytes per status change, so open/append/close keeps
	// their durability story simple and never holds a second handle open.
	f, err := os.OpenFile(filepath.Join(dir, "log.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(line)
	return err
}

// LogSize returns the current size of a shell's log.bin, i.e. the offset just
// past the last byte ever written.
func (s *Store) LogSize(sessionID, shellID string) (int64, error) {
	st, err := os.Stat(s.LogPath(sessionID, shellID))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return st.Size(), nil
}

// --- stream index (log.stdio) -----------------------------------------------
//
// log.stdio records where a shell's byte stream switches between its two
// low-level producers, stdout and stderr (pipe channels only: a pty merges
// stderr into the terminal stream at the source, so a pty shell's index stays
// empty). It is the transport-layer counterpart of log.jsonl, which records
// who acted (agent, human, review) rather than which pipe the bytes came from.
//
// The file is a flat array of 8-byte little-endian int64 records, one per
// SWITCH - the offset where the producing stream changes - never per chunk:
// consecutive chunks from the same stream record nothing, so a record is the
// start of a maximal run. The sign carries the type, the magnitude the
// position:
//
//	value > 0  a stdout run starts at value-1
//	value < 0  a stderr run starts at -value-1
//
// The offset is stored +1 because a stderr switch at offset 0 would otherwise
// be -0, which is indistinguishable from a stdout switch at 0. With the shift,
// 0 is not a legal value: an empty stream has no records, and a stream's first
// chunk always records, so the records tile the stream from offset 0 with no
// implicit starting state. Reading is self-describing per record - a lost or
// torn record can only shrink the stderr set (the bytes it covered read as
// stdout, today's behaviour), never misattribute anything else.

// StdioPath returns the path of a shell's stream-switch index.
func (s *Store) StdioPath(sessionID, shellID string) string {
	return filepath.Join(s.shellDir(sessionID, shellID), "log.stdio")
}

// StdioStream identifies which low-level producer a chunk came from. The zero
// value is stdout, so a caller that does not care records stdout switches.
type StdioStream int8

const (
	// StdioOut is the process's stdout.
	StdioOut StdioStream = 0
	// StdioErr is the process's stderr.
	StdioErr StdioStream = 1
)

// stdioRecord packs one switch into an int64: sign = stream, |value|-1 = the
// absolute offset where the run starts.
func stdioRecord(offset int64, stream StdioStream) int64 {
	v := offset + 1
	if stream == StdioErr {
		v = -v
	}
	return v
}

// DecodeStdioRecord unpacks one log.stdio record: sign = stream, magnitude-1 =
// the absolute offset where that stream's run starts. ok is false for 0, which
// no legal record ever holds (the +1 shift makes sure of it).
func DecodeStdioRecord(v int64) (offset int64, stream StdioStream, ok bool) {
	if v == 0 {
		return 0, 0, false
	}
	if v < 0 {
		return -v - 1, StdioErr, true
	}
	return v - 1, StdioOut, true
}

// ReadStdioRecords returns a shell's raw switch records in file order.
func (s *Store) ReadStdioRecords(sessionID, shellID string) ([]int64, error) {
	if err := validateID(sessionID); err != nil {
		return nil, err
	}
	if err := validateID(shellID); err != nil {
		return nil, err
	}
	st, err := os.Stat(s.StdioPath(sessionID, shellID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	count := st.Size() / lineRecordSize
	if count == 0 {
		return nil, nil
	}
	return s.readInt64Records(s.StdioPath(sessionID, shellID), count)
}

// readInt64Records reads n little-endian int64 records from a fixed-record
// sidecar file. Shared by the line and stdio indexes.
func (s *Store) readInt64Records(path string, n int64) ([]int64, error) {
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	buf := make([]byte, n*lineRecordSize)
	if _, err := io.ReadFull(f, buf); err != nil {
		return nil, err
	}
	out := make([]int64, 0, n)
	for i := 0; i+lineRecordSize <= len(buf); i += lineRecordSize {
		out = append(out, int64(binary.LittleEndian.Uint64(buf[i:])))
	}
	return out, nil
}

// --- line index (log.lines) -------------------------------------------------
//
// log.lines is a sidecar index over log.bin, one shell each, maintained by the
// same sink that appends the bytes. It is a flat array of fixed-width records:
// record N-1 (8-byte little-endian) is the absolute offset of the '\n' that
// terminates line N. Line numbers are therefore absolute and 1-based, and they
// never shift: log.bin is append-only, so line N is the line that was N when it
// was written, for the life of the shell. A trailing partial line (bytes after
// the last '\n') has no record and no line number, matching how shell_output
// reports it.
//
// The index is advisory, like the mark index: a missing, stale or torn log.lines
// never changes what bytes exist, only how fast a line can be found. Every
// mutation goes through the sink, so appends and catch-up scans cannot
// interleave; readers use positional reads and never trust a torn tail record.

// lineRecordSize is the width of one log.lines record.
const lineRecordSize = 8

// LineIndexPath returns the path of a shell's line index.
func (s *Store) LineIndexPath(sessionID, shellID string) string {
	return filepath.Join(s.shellDir(sessionID, shellID), "log.lines")
}

// LineRecordCount returns how many complete lines the index holds: the file size
// in whole records. A torn tail record (a crash mid-append) is not a line and is
// not counted; the next append through the sink truncates it away.
func (s *Store) LineRecordCount(sessionID, shellID string) (int64, error) {
	if err := validateID(sessionID); err != nil {
		return 0, err
	}
	if err := validateID(shellID); err != nil {
		return 0, err
	}
	st, err := os.Stat(s.LineIndexPath(sessionID, shellID))
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	return st.Size() / lineRecordSize, nil
}

// ReadLineRecords returns count records starting at record first (0-based). A
// range past the end returns fewer records, like a short file read.
func (s *Store) ReadLineRecords(sessionID, shellID string, first int64, count int) ([]int64, error) {
	if err := validateID(sessionID); err != nil {
		return nil, err
	}
	if err := validateID(shellID); err != nil {
		return nil, err
	}
	if first < 0 || count <= 0 {
		return nil, nil
	}
	f, err := os.Open(s.LineIndexPath(sessionID, shellID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, int64(count)*lineRecordSize)
	n, err := f.ReadAt(buf, first*lineRecordSize)
	if err != nil && err != io.EOF {
		return nil, err
	}
	n -= n % lineRecordSize
	out := make([]int64, 0, n/lineRecordSize)
	for i := 0; i+lineRecordSize <= n; i += lineRecordSize {
		out = append(out, int64(binary.LittleEndian.Uint64(buf[i:])))
	}
	return out, nil
}

// SyncLineIndex brings a shell's line index up to date with its log.bin: it
// scans the bytes after the last indexed '\n' and appends a record for each
// newline found. It is the repair path for an index that is missing (a session
// that predates log.lines), lagging (a crash between the byte append and the
// record append), or torn (a truncated final record).
//
// The work is a sink request, so it serializes against every append: a scan
// cannot race the writer into appending a record twice. Idempotent by
// construction - it always resumes from the last indexed newline, re-deriving
// that position from the files rather than trusting memory.
func (s *Store) SyncLineIndex(sessionID, shellID string) error {
	if err := validateID(sessionID); err != nil {
		return err
	}
	if err := validateID(shellID); err != nil {
		return err
	}
	return s.logSink.do(sinkRequest{op: opSyncLines, session: sessionID, shell: shellID}).err
}

// ReadLog reads at most max bytes of a shell's log.bin starting at offset.
//
// This is a positional read: it never consults log.jsonl and never accumulates
// lengths, so a window is exactly the file's bytes at that position.
func (s *Store) ReadLog(sessionID, shellID string, offset int64, max int) ([]byte, error) {
	if offset < 0 {
		offset = 0
	}
	if max <= 0 {
		return nil, nil
	}
	f, err := os.Open(s.LogPath(sessionID, shellID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	buf := make([]byte, max)
	n, err := f.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, err
	}
	return buf[:n], nil
}
