package message

import (
	"github.com/open-mcp-ai/termcp/internal/storage"
)

// Stream ownership over the transcript's low-level index (log.stdio, see
// storage/log.go). The transcript is one byte stream; these ranges say which
// parts of it came from the process's stderr, so a reader can filter either
// stream while offsets keep a single meaning. The business-level spans
// (log.jsonl) deliberately stay out of this: they say who acted, not which
// pipe the bytes arrived on.

// StderrRanges returns the half-open byte ranges of the shell's stream that
// came from stderr, in order, non-overlapping. Everything outside them is
// stdout - including every byte of a shell whose index is missing or empty,
// which is exactly the pre-index behaviour. The last range runs to the end of
// the log while the stream is still in stderr.
func (m *Manager) StderrRanges(sessionID, shellID string) ([][2]int64, error) {
	if m.store == nil {
		return nil, nil
	}
	recs, err := m.store.ReadStdioRecords(sessionID, shellID)
	if err != nil {
		return nil, err
	}
	var ranges [][2]int64
	var open [2]int64
	opening := false
	for _, v := range recs {
		off, stream, ok := storage.DecodeStdioRecord(v)
		if !ok {
			continue
		}
		if stream == storage.StdioErr {
			// Records exist only for switches, so two stderr starts in a row
			// cannot happen; should a damaged file produce one, keeping the
			// earlier start stays within the recorded evidence.
			if !opening {
				open = [2]int64{off, 0}
				opening = true
			}
			continue
		}
		if opening {
			open[1] = off
			ranges = append(ranges, open)
			opening = false
		}
	}
	if opening {
		size, err := m.store.LogSize(sessionID, shellID)
		if err != nil {
			return nil, err
		}
		open[1] = size
		ranges = append(ranges, open)
	}
	return ranges, nil
}

// SelectBytes keeps the bytes of raw (which sits at stream offset start) that
// belong to one low-level stream: the stderr ranges when stderr is true,
// everything outside them when it is false. It is the single byte-level
// implementation behind shell_output(stream=) on MCP and the stream parameter
// on REST output-range, so the two surfaces cannot drift apart. The ranges
// must be ordered and non-overlapping, as StderrRanges returns them.
func SelectBytes(raw []byte, start int64, stderrRanges [][2]int64, stderr bool) []byte {
	if len(stderrRanges) == 0 && stderr {
		return nil
	}
	end := start + int64(len(raw))
	var out []byte
	pos := start // for !stderr: the start of the stdout gap not yet copied
	for _, r := range stderrRanges {
		rs, re := r[0], r[1]
		if rs < start {
			rs = start
		}
		if re > end {
			re = end
		}
		if rs >= re {
			continue
		}
		if stderr {
			out = append(out, raw[rs-start:re-start]...)
			continue
		}
		if rs > pos {
			out = append(out, raw[pos-start:rs-start]...)
		}
		pos = re
	}
	if !stderr && pos < end {
		out = append(out, raw[pos-start:]...)
	}
	return out
}
