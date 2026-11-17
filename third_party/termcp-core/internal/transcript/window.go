package transcript

import (
	"bytes"
	"context"
	"time"
	"unicode/utf8"

	"github.com/open-mcp-ai/termcp/internal/ansi"
	"github.com/open-mcp-ai/termcp/internal/encoding"
)

// Params is a transcript read request: which bytes to return, how to render
// them, and how long to wait for a live cursor. The field names are the shared
// contract of both surfaces - the MCP tool maps its JSON arguments here, the
// HTTP API maps its query string here.
type Params struct {
	StripAnsi bool
	Timeout   float64
	MaxLines  int
	MaxBytes  int
	ReaderID  int
	// Offset names a POSITIONAL read when non-nil, and says nothing when nil.
	//
	// The same shape as TailLines, for the same reason: 0 is a real position (the
	// first byte), so an int could not distinguish "read from the start" from "no
	// positional read at all". The sentinel was -1 and it held only because every
	// caller happened to set another selector too; a struct literal that omitted
	// this field read from byte 0 without saying so.
	Offset *int64
	// TailLines names a LINE TAIL when non-nil, and says nothing when nil.
	//
	// A pointer, because ZERO IS A REAL REQUEST: `--tail 0` asks for zero lines,
	// which is a different instruction from "no tail at all" — and the two used to
	// share the value 0, so the smallest request produced the largest answer (the
	// whole buffer). No in-band sentinel can fix that for an int: -1 can say "not
	// given", but 0 would still be ambiguous in a struct literal that simply omits
	// the field. nil is unambiguous, and the compiler makes every caller state which
	// one it means.
	TailLines *int
	FromLine  int64
	Stream    string // "", "all", "o", "e"
	// TailBytes forces the byte-tail read (last MaxBytes aligned to a line
	// start) even on a live shell. The MCP tool derives it from tail_lines and
	// the closed-shell default; the HTTP tail=1 parameter sets it directly.
	TailBytes bool
	// RequireUTF8 refuses a window whose bytes are not valid UTF-8 instead of
	// returning a string that its transport will silently alter.
	//
	// It is opt-in because the two surfaces need opposite things. A reply that
	// becomes JSON cannot carry arbitrary bytes — encoding/json substitutes U+FFFD,
	// which shifts offsets and corrupts content invisibly — so the model-facing
	// read wants to fail loudly. A terminal repaint (the Web UI, StripAnsi=false)
	// is drawing the bytes it was given and is expected to be handed whatever is
	// there; refusing its window would take the terminal down over output it can
	// display perfectly well.
	RequireUTF8 bool
	// Charset is the encoding the session's bytes are in, for output that is not
	// UTF-8 at all. Empty means UTF-8.
	//
	// A TERMINAL IS NOT A UTF-8 DEVICE: on a Chinese Windows the console code page is
	// 936, so a shell writes GBK, and refusing that window left the caller with no
	// way to read its own session (issue #149). Decoding is the answer to that, not
	// an escape hatch: the bytes are the encoding the caller said they are, so
	// `--grep` matches characters rather than byte sequences and a JSON-RPC frame
	// from a GBK extension can be read at all.
	//
	// It is per READ rather than per SESSION because the two can disagree — a
	// profile's shell writes GBK while a tool it launches writes UTF-8 — and because
	// a caller who knows the encoding should not have to change a profile to say so.
	Charset string
}

// trimIncomplete drops the trailing bytes of a byte window that cannot complete a
// character, so a window never ends inside one.
//
// THE RULE BELONGS TO THE CHARSET, and it was applied to only ONE of the two ways a
// window gets cut. A cursor read goes through the buffer, which takes the rule as a
// callback; a POSITIONAL read — offset, from_line — sliced the log directly and applied
// nothing. That is the path `run` always uses (it reads at the offset the command was
// submitted), so a `bytes_limit` landing inside a three-byte glyph still produced the
// original bug: the reply was refused as "not valid UTF-8" and named a charset the
// session does not write. Same rule, same owner, applied wherever bytes are cut.
func trimIncomplete(charset string, raw []byte) ([]byte, int64) {
	cut := encoding.IncompleteTail(charset, raw)
	if cut <= 0 || cut > len(raw) {
		return raw, 0
	}
	return raw[:len(raw)-cut], int64(cut)
}

// render turns this read's bytes into its output string, enforcing the
// byte-fidelity contract only where the caller asked for it (see RequireUTF8).
func (p Params) render(raw []byte) (string, error) {
	if p.Charset != "" {
		// DECODE FIRST, THEN RENDER, so everything downstream — the UTF-8 refusal,
		// ANSI stripping, the caller's `--grep` — sees characters. The escape rule is
		// the same either way; only the meaning of a byte changes.
		decoded, err := encoding.Decode(p.Charset, raw)
		if err != nil {
			return "", errf(ErrInvalid, "%s", err.Error())
		}
		raw = decoded
	}
	if p.RequireUTF8 {
		return Render(raw, p.StripAnsi)
	}
	return clean(raw, p.StripAnsi), nil
}

// ValidateParams checks the parts of the read contract both surfaces share.
// Surface-specific ranges (the MCP tool's blocking timeout) stay with the
// surface. The messages are part of the tool contract: callers get told why
// instead of silently getting a different read.
func ValidateParams(p Params) error {
	if p.TailLines != nil && *p.TailLines < 0 {
		return errf(ErrInvalid, "tail_lines must be >= 0, got %d", *p.TailLines)
	}
	if p.Offset != nil && *p.Offset < 0 {
		return errf(ErrInvalid, "offset must be >= 0, got %d", *p.Offset)
	}
	if p.FromLine < 0 {
		return errf(ErrInvalid, "from_line must be >= 0, got %d", p.FromLine)
	}
	if p.Stream != "" && p.Stream != "all" && p.Stream != "o" && p.Stream != "e" {
		return errf(ErrInvalid, "stream must be \"all\", \"o\" or \"e\", got %q", p.Stream)
	}
	// from_line picks the window by line number, so it replaces the byte/cursor
	// selectors rather than combining with them: each names a different position
	// for the same read, and honoring both would mean silently choosing one.
	if p.FromLine > 0 {
		if p.TailLines != nil {
			return errf(ErrInvalid, "from_line and tail_lines are mutually exclusive; use one selector per read")
		}
		if p.Offset != nil {
			return errf(ErrInvalid, "from_line and offset are mutually exclusive; use one selector per read")
		}
		if p.ReaderID > 0 {
			return errf(ErrInvalid, "from_line addresses the persisted log; reader_id is a live cursor — use one selector per read")
		}
	}
	return nil
}

// Window is the slice of a shell's output that a read returned, plus where that
// slice sits in the whole stream. Those offsets are what lets a caller page:
// End is where the next read starts, and HasMore says whether anything follows
// it.
//
// Every byte field describes the RAW stream; Output is the rendering of it
// (ANSI-stripped when asked, stream-filtered when asked). PartialTail is that
// rule applied to line boundaries: how many bytes at the end of the raw window
// follow the last '\n' (0 = the window ends on a line boundary). The FromLine
// group is filled only by the from_line selector, which reads whole lines from
// the line index.
type Window struct {
	Output string
	// Raw is the bytes Output was rendered from, byte-for-byte. Callers that page
	// by offset need this rather than len(Output): rendering (ANSI stripping,
	// escapes, and the U+FFFD JSON would otherwise substitute) changes the length,
	// so an offset derived from the rendered string points at the wrong byte.
	Raw   []byte
	Start int64
	End   int64
	Total int64
	// Trimmed is how many bytes at the end of this window were withheld to keep it
	// from ending inside a character. They are NOT lost: the window stops before them,
	// so the next read delivers them whole. It is reported because "the reply is
	// shorter than the limit I set" and "the window rolled back one character" are the
	// same bytes on the wire and only this number tells them apart.
	Trimmed     int64
	HasMore     bool
	PartialTail int64
	FromLine    int64
	ToLine      int64
	TotalLines  int64
	LineBytes   []int64
	// StreamOmitted counts the bytes INSIDE [Start, End) that the stream filter passed
	// over because they belonged to the other stream.
	//
	// A filtered read answers "give me stdout" by selecting a subset of the span it
	// covers, and the span still runs to End — cursors advance by the range, not by the
	// body, or a poll loop would re-read the same bytes forever. The consequence is that
	// those bytes leave the caller's cursor without ever being shown, and nothing said
	// so: `--stream e` on a stdout-only channel answered 200 with an empty body and
	// `Content-Range: bytes 0-7/8`, then consumed all eight. The count is what lets the
	// reply say "eight bytes went past, here is the flag that shows them".
	StreamOmitted int64

	// Stream names the stream whose numbers Start/End/Total are in: "e", "o", or ""
	// for the channel itself. When it is set, the four fields above are CHANNEL
	// coordinates and StreamStart/StreamEnd/StreamTotal are the reply's.
	//
	// TWO COORDINATE SYSTEMS, ON PURPOSE, because the two consumers of a window need
	// different ones. A caller pages with the numbers in the reply, and those have to
	// address the bytes the reply actually contains — a `--stream e` body is stderr,
	// and the channel offset of a stderr byte is a number that also counts stdout,
	// so paging with it lands on the wrong side of the channel (issue #202). The
	// CURSOR is a channel position: one reader walks one write order, and the bytes
	// it steps over are the ones StreamOmitted counts.
	Stream string
	// StreamStart/StreamEnd/StreamTotal are Start/End/Total renumbered without the
	// other stream's bytes. StreamEnd-StreamStart is the delivered span in this
	// stream's bytes.
	StreamStart int64
	StreamEnd   int64
	StreamTotal int64
}

// streamCoords is one stream's numbering of a channel's bytes.
//
// The channel stays the single record — stdout and stderr are interleaved in one
// write order, which is what makes "what did the process print, in what order"
// answerable at all — and the stderr RANGES are the index that separates them. A
// stream's numbering is therefore the channel's with the other stream's bytes
// dropped, and both directions are a walk over those ranges.
type streamCoords struct {
	ranges [][2]int64 // stderr spans, ordered and disjoint, in channel offsets
	stderr bool
}

// pos maps a channel byte position into this stream's numbering.
func (c streamCoords) pos(channel int64) int64 {
	if channel <= 0 {
		return 0
	}
	n := int64(0)
	for _, r := range c.ranges {
		if r[0] >= channel {
			break
		}
		end := r[1]
		if end > channel {
			end = channel
		}
		if end > r[0] {
			n += end - r[0]
		}
	}
	if c.stderr {
		return n
	}
	return channel - n
}

// channel is the inverse: where this stream's n-th byte sits in the channel.
//
// A position past the end of the stream answers the channel position of the end,
// which is what makes `--offset <one past the end>` an empty read rather than an
// error — the same shape every other out-of-range offset has.
func (c streamCoords) channel(n int64) int64 {
	if n <= 0 {
		if c.stderr {
			if len(c.ranges) > 0 {
				return c.ranges[0][0]
			}
			return 0
		}
		if len(c.ranges) > 0 && c.ranges[0][0] == 0 {
			return c.ranges[0][1]
		}
		return 0
	}
	if c.stderr {
		seen := int64(0)
		for _, r := range c.ranges {
			length := r[1] - r[0]
			if n < seen+length {
				return r[0] + (n - seen)
			}
			seen += length
		}
		if len(c.ranges) > 0 {
			return c.ranges[len(c.ranges)-1][1]
		}
		return 0
	}
	// stdout: the bytes BETWEEN the stderr ranges.
	seen, at := int64(0), int64(0)
	for _, r := range c.ranges {
		if r[0] < at {
			continue
		}
		if n < seen+(r[0]-at) {
			return at + (n - seen)
		}
		seen += r[0] - at
		at = r[1]
	}
	return at + (n - seen)
}

// Read produces the window a read asked for. The four forms share a result
// shape but not a source, which is the point of one read entry point: a tail or
// a positional read copies bytes, so it works on a live shell and on a closed
// one alike, while a reader_id read is a live cursor that consumes the stream
// and only exists while the shell does.
func Read(ctx context.Context, src *Source, p Params) (Window, error) {
	if err := ValidateParams(p); err != nil {
		return Window{}, err
	}
	if p.ReaderID > 0 && src.Live == nil {
		return Window{}, errf(ErrInvalid, "reader_id requires a live shell; closed sessions are read with offset/tail_lines")
	}

	// THE SELECTED STREAM'S OWN NUMBERS (issue #202).
	//
	// `--stream e` used to be a VIEW: the body was stderr, the headers were the
	// channel's. So `read --stream e` on a stdout-only channel answered
	// `Content-Range: bytes */7` — "7 bytes here" — with an empty body, and a caller
	// could not tell that from "you have already read all the stderr there is". The
	// addresses a reply carries have to address the bytes the reply carries, or the
	// two questions the header exists to answer ("how much is there", "where do I
	// resume") are both unanswerable.
	//
	// The ranges are fetched once and used for both directions: the filter needs
	// them, and so does the renumbering below.
	var coords *streamCoords
	if p.Stream == "e" || p.Stream == "o" {
		if src.MsgMgr == nil {
			return Window{}, errf(ErrInvalid, "stream filtering requires a persistent transcript log")
		}
		ranges, err := src.MsgMgr.StderrRanges(src.SessID, src.ShellID)
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		coords = &streamCoords{ranges: ranges, stderr: p.Stream == "e"}
		// AN OFFSET IN A STREAM READ IS AN OFFSET IN THAT STREAM. Handing the caller
		// stream numbers in the reply and taking channel numbers in `--offset` would
		// make `Next-Offset` unusable — a reply whose resumption point cannot be typed
		// back in is the bug this fixes, not a different spelling of it.
		if p.Offset != nil {
			at := coords.channel(*p.Offset)
			p.Offset = &at
		}
	}
	var w Window
	switch {
	case p.FromLine > 0:
		// Line-addressed read, served from the persisted log: log.bin is the
		// stream of record for live and DEAD shells alike, and the line index
		// (log.lines) numbers exactly those bytes. Reading the live buffer here
		// could race its own compaction for long-past lines; the log cannot.
		if src.MsgMgr == nil {
			return Window{}, errf(ErrInvalid, "from_line is unavailable: this transcript has no persistent log")
		}
		totalLines, err := src.MsgMgr.LineCount(src.SessID, src.ShellID)
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		if p.FromLine > totalLines {
			return Window{}, errf(ErrInvalid, "from_line %d out of range (stream has %d complete line(s))", p.FromLine, totalLines)
		}
		total, err := src.Len()
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		start, err := src.MsgMgr.LineStartOffset(src.SessID, src.ShellID, p.FromLine)
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		max := p.MaxBytes
		if max <= 0 {
			max = int(total - start)
			if max < 0 {
				max = 0
			}
		}
		raw, _, err := src.MsgMgr.OutputByteRange(src.SessID, src.ShellID, start, max)
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		raw, trimmed := trimIncomplete(p.Charset, raw)
		raw = truncateAtLines(raw, p.MaxLines)
		end := start + int64(len(raw))
		// The window starts at a line start, so counting its newlines numbers
		// every complete line it holds, and their byte lengths come from the
		// gaps between them - no second pass over the index needed.
		newlines := int64(bytes.Count(raw, []byte{'\n'}))
		lineBytes := make([]int64, 0, newlines)
		prev := start - 1
		for i, c := range raw {
			if c == '\n' {
				lineBytes = append(lineBytes, start+int64(i)-prev)
				prev = start + int64(i)
			}
		}
		filtered, omitted, err := filterStream(raw, start, coords, p.Stream)
		if err != nil {
			return Window{}, err
		}
		rendered, err := p.render(filtered)
		if err != nil {
			return Window{}, err
		}
		w = Window{
			Output:        rendered,
			Raw:           filtered,
			StreamOmitted: omitted,
			Start:         start,
			End:           end,
			Total:         total,
			HasMore:       end < total,
			PartialTail:   rawPartialTail(raw),
			FromLine:      p.FromLine,
			ToLine:        p.FromLine - 1 + newlines,
			TotalLines:    totalLines,
			LineBytes:     lineBytes,
			Trimmed:       trimmed,
		}
	// AN EXPLICIT TAIL IS A TAIL. The extra disjunct that used to be here — a
	// closed source read with no selector at all — answered the same call the same
	// way forever: it is a TAIL, not a cursor, so a caller polling an extension's
	// output re-read the same bytes on every call and the offsets never advanced
	// (#159 C1). The caller layer now supplies a cursor for that case (it is the
	// layer that can hold one: this package is deliberately stateless), so what
	// arrives here is either an explicit tail or an explicit offset.
	case p.TailLines != nil || p.TailBytes:
		raw, st, tot, err := src.scanTailWindow(p.TailLines, p.MaxBytes)
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		filtered, omitted, err := filterStream(raw, st, coords, p.Stream)
		if err != nil {
			return Window{}, err
		}
		rendered, err := p.render(filtered)
		if err != nil {
			return Window{}, err
		}
		w = Window{Output: rendered, Raw: filtered, StreamOmitted: omitted, Start: st, End: tot, Total: tot, PartialTail: rawPartialTail(raw)}
	case p.Offset != nil:
		offset := *p.Offset
		total, err := src.Len()
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		max := p.MaxBytes
		if max <= 0 {
			max = int(total - offset)
			if max < 0 {
				max = 0
			}
		}
		raw, _, err := src.ByteRange(offset, max)
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		raw, trimmed := trimIncomplete(p.Charset, raw)
		raw = truncateAtLines(raw, p.MaxLines)
		end := offset + int64(len(raw))
		filtered, omitted, err := filterStream(raw, offset, coords, p.Stream)
		if err != nil {
			return Window{}, err
		}
		rendered, err := p.render(filtered)
		if err != nil {
			return Window{}, err
		}
		w = Window{Output: rendered, Raw: filtered, StreamOmitted: omitted, Start: offset, End: end, Total: total, HasMore: end < total, PartialTail: rawPartialTail(raw), Trimmed: trimmed}
	default:
		if src.Live == nil {
			// NO SELECTOR ON A CLOSED SOURCE. This cannot silently pick one: “the
			// whole history” is what the caller did not ask for (and is the O(N²)
			// replay #159 C1 was opened about), “the tail” answers a different
			// question, and a live reader cursor does not exist here. The caller
			// layer knows which one it meant — the MCP read supplies a stored cursor
			// automatically — so reaching this means a surface skipped that step.
			return Window{}, errf(ErrInvalid, "%s",
				"this channel is closed, so a read needs to say where to start: offset, from_line or tail_lines")
		}
		// Live streaming cursor path (unchanged semantics).
		pre := src.Live.ReaderCursor(p.ReaderID)
		if pre < 0 {
			return Window{}, errf(ErrReader, "reader_id %d is not registered on this shell", p.ReaderID)
		}
		// A REFUSAL MUST NOT EAT THE BYTES (issue #200).
		//
		// ReadTerminalStream commits — it advances this reader as it hands the window
		// over — so every check below answers a question about bytes that have ALREADY
		// moved: can they survive JSON, does the caller's charset decode them. Refusing
		// after the commit threw away exactly the data whose remedy the refusal named,
		// and the caller could watch it happen: `read` answered 400 "pass --encoding gbk",
		// the caller passed it, and got 204 with the byte count still on `get`.
		//
		// Putting them back is what makes this a rejection rather than a read whose
		// answer was discarded. The seek is a no-op when the caller is already at `pre`,
		// and refusing to fail the call over a failed seek keeps the reply about the
		// bytes, which is what the caller asked about.
		refuse := func(err error) (Window, error) {
			_ = src.Live.SeekReader(p.ReaderID, pre)
			return Window{}, err
		}
		// THE RULE THE BUFFER CANNOT KNOW. A byte window is cut wherever the caller's
		// budget falls, and a cut inside a character is not the caller's data being
		// wrong — but the byte-fidelity check below cannot tell the difference, so it
		// answers with a charset the session is not writing. This layer is the only one
		// that knows the charset, so it hands the buffer the rule instead of a name.
		var trimmed int64
		trim := func(window []byte) int {
			cut := encoding.IncompleteTail(p.Charset, window)
			if cut > 0 && cut <= len(window) {
				trimmed = int64(cut)
			}
			return cut
		}
		out, raw, partial, err := src.Live.ReadTerminalStream(ctx, p.ReaderID, time.Duration(p.Timeout*float64(time.Second)), p.StripAnsi, p.MaxLines, p.MaxBytes, trim)
		if err != nil {
			return Window{}, errf(ErrOperation, "%s", err.Error())
		}
		end := src.Live.ReaderCursor(p.ReaderID)
		// The source's length, not the buffer's: the log may hold bytes the buffer has
		// not been given yet, and a reply that names a smaller total than the channel
		// has is a false fact (see Source.Len).
		total, _ := src.Len()
		// The browser path strips ANSI inside ReadTerminalStream, so `out` is the
		// rendered form and `raw` is what it was rendered from. Checking the RAW
		// bytes is what makes the refusal honest: a stripped string could pass
		// utf8.Valid while the stream it came from was binary.
		if p.Charset != "" {
			decoded, derr := encoding.Decode(p.Charset, raw)
			if derr != nil {
				return refuse(errf(ErrInvalid, "%s", derr.Error()))
			}
			raw = decoded
			out = clean(raw, p.StripAnsi)
		}
		if p.RequireUTF8 {
			if err := RenderCheck(raw); err != nil {
				return refuse(err)
			}
		}
		output := out
		windowRaw := raw
		// `=` AND NOT `:=`. A `:=` here declares a NEW omitted inside this if-block —
		// the one the literal below reads is the outer one, which stays zero — so every
		// filtered read on the live path reported `stream_omitted_bytes: 0` while the
		// filter had passed over the whole window. `--offset 0` looked right only
		// because its `:=` and its literal share a scope, and the two paths are supposed
		// to be one behaviour (issue #203).
		var omitted int64
		if p.Stream != "" && p.Stream != "all" {
			var filtered []byte
			var ferr error
			filtered, omitted, ferr = filterStream(raw, pre, coords, p.Stream)
			if ferr != nil {
				return refuse(ferr)
			}
			// Raw is the bytes Output was rendered FROM, which on a filtered read are the
			// filtered ones. The other three read paths already store it that way; this one
			// kept the channel window, so `Raw` and `Output` described different byte runs.
			windowRaw = filtered
			output = clean(filtered, p.StripAnsi)
		}
		// StreamOmitted goes in the literal: the assignment replaces the whole struct,
		// so a field set before it is silently thrown away — which is exactly the shape
		// of the bug the field exists to report, once by ordering and once by shadowing.
		w = Window{Output: output, Raw: windowRaw, StreamOmitted: omitted, Start: pre, End: end, Total: total, HasMore: end < total, PartialTail: partial, Trimmed: trimmed}
	}
	// THE REPLY'S NUMBERS ARE THE SELECTED STREAM'S.
	//
	// Computed from the CHANNEL span [Start, End), not from len(Raw): Raw may have
	// been re-encoded (a charset read turns GBK bytes into UTF-8 ones), and a window's
	// address is a position in the stream, not the length of its rendering. HasMore is
	// the same question asked in the same units — "does this stream hold bytes past
	// what I just gave you" — because the channel's answer would be about the other
	// stream's bytes (issue #202).
	if coords != nil {
		w.Stream = p.Stream
		w.StreamStart = coords.pos(w.Start)
		w.StreamEnd = coords.pos(w.End)
		w.StreamTotal = coords.pos(w.Total)
		w.HasMore = w.StreamEnd < w.StreamTotal
	}
	return w, nil
}

// clean renders raw bytes for a consumer: stripping is optional (the Web UI's
// terminal repaints raw bytes; model-facing surfaces want text).
func clean(raw []byte, stripAnsi bool) string {
	if !stripAnsi {
		return string(raw)
	}
	return ansi.Compact(ansi.Strip(string(raw)))
}

// Render turns raw window bytes into the string a caller receives.
//
// It refuses to hand back a string that cannot survive the trip: a Go string can
// hold arbitrary bytes, but every consumer of this value re-encodes it as JSON,
// and encoding/json REPLACES invalid UTF-8 with U+FFFD instead of failing. That
// substitution is silent and irreversible, and it is not cosmetic — it shifts
// byte offsets, so a caller that pages by offset (mcp recv) reads from the wrong
// place, and a caller that inspects magic bytes sees \ufffd where 0x89 was.
//
// So the loss is refused here, where the raw bytes are still available to name
// the offending offset, rather than reported as mojibake three layers up. The
// caller's remedy is a byte-oriented read: text mode renders escapes, and the
// window boundaries stay byte-exact.
func renderInvalidUTF8(raw []byte) error {
	i := invalidUTF8Index(raw)
	// NAME THE CHARSET WHEN THE BYTES CONFIRM IT. On a Chinese Windows the console
	// code page is 936, so this refusal was the normal case rather than an edge one —
	// and it named two remedies that do not exist on a channel (`mode=hex`,
	// `mode=file` are file reads), leaving a caller with output it could not read at
	// all. The round trip in Sniff is what makes "these bytes are GBK" a fact rather
	// than a guess, and the flag that follows it is one this build answers.
	hint := "pass --encoding <charset> if the session writes something other than UTF-8, or read a narrower window"
	if name := encoding.Sniff(raw); name != "" {
		hint = "these bytes decode as " + name + " and round-trip byte-for-byte; if this session writes " +
			name + ", pass --encoding " + name
	}
	return errf(ErrInvalid, "the window contains bytes that are not valid UTF-8 "+
		"(first at byte %d of %d); a JSON reply would replace them with U+FFFD and silently "+
		"shift every offset after them. %s", i, len(raw), hint)
}

// Render applies clean and enforces the byte-fidelity contract above.
func Render(raw []byte, stripAnsi bool) (string, error) {
	out := clean(raw, stripAnsi)
	if !utf8.ValidString(out) {
		return "", renderInvalidUTF8(raw)
	}
	return out, nil
}

// invalidUTF8Index reports where the first invalid sequence starts, for the error.
func invalidUTF8Index(b []byte) int {
	for i := 0; i < len(b); {
		r, size := utf8.DecodeRune(b[i:])
		if r == utf8.RuneError && size == 1 {
			return i
		}
		i += size
	}
	return -1
}

// filterStream keeps only the bytes of raw (which sits at channel offset start)
// that stream asks for: "o" keeps everything outside the recorded stderr ranges,
// "e" the bytes inside them, "" and "all" return raw unchanged.
//
// It returns the bytes the selection left behind INSIDE this span, which is how
// many channel bytes the caller's cursor moves past without being shown them.
// The window's Start/End stay CHANNEL positions whatever it returns — the cursor
// walk is a channel walk — and Read renumbers them for the reply (see streamCoords).
func filterStream(raw []byte, start int64, coords *streamCoords, stream string) (filtered []byte, omitted int64, err error) {
	if stream == "" || stream == "all" {
		return raw, 0, nil
	}
	if coords == nil {
		return nil, 0, errf(ErrInvalid, "stream filtering requires a persistent transcript log")
	}
	ranges := coords.ranges
	out := selectBytes(raw, start, ranges, stream == "e")
	// What the selection left behind, INSIDE this span. Non-zero means the caller's
	// cursor is about to move past bytes it was not shown.
	return out, int64(len(raw) - len(out)), nil
}

// selectBytes is the byte-level half of the filter, over prepared ranges.
func selectBytes(raw []byte, start int64, stderrRanges [][2]int64, stderr bool) []byte {
	if stderr && len(stderrRanges) == 0 {
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

// rawPartialTail is the read window's line-boundary fact: bytes after the
// last '\n' of the RAW window. Computed on raw, never on the cleaned text —
// the stream is what a line protocol is reading, and cleaning may merge or
// drop its line endings.
func rawPartialTail(raw []byte) int64 {
	partial := int64(len(raw))
	if idx := bytes.LastIndexByte(raw, '\n'); idx >= 0 {
		partial -= int64(idx + 1)
	}
	return partial
}

// countLines counts newline-delimited lines in raw bytes; a trailing partial
// line (no closing '\n') counts as one line.
func countLines(b []byte) int64 {
	if len(b) == 0 {
		return 0
	}
	n := int64(bytes.Count(b, []byte{'\n'}))
	if b[len(b)-1] != '\n' {
		n++
	}
	return n
}

// truncateAtLines keeps only complete newline-terminated lines up to maxLines.
// Buffer parity: when the window does not reach the end of the stream, an
// incomplete trailing line stays in the stream for the next page.
func truncateAtLines(raw []byte, maxLines int) []byte {
	if maxLines <= 0 {
		return raw
	}
	seen := 0
	for i, c := range raw {
		if c != '\n' {
			continue
		}
		seen++
		if seen == maxLines {
			return raw[:i+1]
		}
	}
	return raw
}

// tailScanCeiling caps backward tail scan at 8 MiB of raw bytes.
const tailScanCeiling = 8 << 20

// defaultTailCap is the DEAD default read: last 8 KiB.
const defaultTailCap = 8192

// scanTailWindow returns the last tailLines lines of the stream (or, when
// tailLines == 0, the last maxBytes bytes aligned to a line start) via a
// bounded backward scan, plus the raw start offset of the returned window.
// The returned window always ends at the stream end.
func (o *Source) scanTailWindow(tailLines *int, maxBytes int) (raw []byte, start, total int64, err error) {
	var raw0 []byte
	total, err = o.Len()
	if err != nil || total == 0 {
		return nil, 0, total, err
	}
	if tailLines != nil && *tailLines == 0 {
		// AN EXPLICIT ZERO: an empty window at the end of the stream.
		return nil, total, total, nil
	}
	if tailLines == nil {
		// No line limit: the byte tail, aligned to a line start.
		cap := int64(maxBytes)
		if cap <= 0 {
			if o.Live != nil {
				cap = defaultTailCap
			} else {
				cap = total
			}
		}
		if cap > total {
			cap = total
		}
		start = total - cap
		raw0, _, err = o.ByteRange(start, int(cap))
		if err != nil {
			return nil, 0, total, err
		}
		if start > 0 {
			if idx := bytes.IndexByte(raw0, '\n'); idx >= 0 {
				raw0 = raw0[idx+1:]
				start += int64(idx + 1)
			}
		}
		return raw0, start, total, nil
	}

	window := int64(64 * 1024)
	for {
		if window > tailScanCeiling {
			window = tailScanCeiling
		}
		start = total - window
		if start < 0 {
			start = 0
		}
		raw0, _, err = o.ByteRange(start, int(window))
		if err != nil {
			return nil, 0, total, err
		}
		if countLines(raw0) >= int64(*tailLines) || start == 0 || window >= tailScanCeiling {
			break
		}
		window *= 2
	}

	if n := countLines(raw0); n > int64(*tailLines) {
		raw0 = takeLastLines(raw0, int64(*tailLines))
	}
	if maxBytes > 0 && int64(len(raw0)) > int64(maxBytes) {
		cut := int64(len(raw0)) - int64(maxBytes)
		if idx := bytes.IndexByte(raw0[cut:], '\n'); idx >= 0 {
			raw0 = raw0[cut+int64(idx)+1:]
		} else {
			raw0 = raw0[cut:]
		}
	}
	start = total - int64(len(raw0))
	return raw0, start, total, nil
}

// takeLastLines keeps only the final N newline-terminated lines of raw.
// A trailing '\n' terminates the last line, so it is not counted as a boundary
// of a following (empty) line: takeLastLines("A\nB\nC\n", 2) == "B\nC\n".
func takeLastLines(raw []byte, n int64) []byte {
	if n <= 0 || len(raw) == 0 {
		return nil
	}
	remaining := n
	end := len(raw)
	if raw[end-1] == '\n' {
		end--
	}
	for i := end - 1; i >= 0; i-- {
		if raw[i] != '\n' {
			continue
		}
		remaining--
		if remaining <= 0 {
			return raw[i+1:]
		}
	}
	return raw
}

// RenderCheck refuses raw bytes that no JSON consumer can reproduce.
//
// It is separate from Render because one path (the live cursor) has the rendered
// string built by the shell rather than here: there the assertion has to be made
// about the RAW bytes the renderer consumed, not about its output.
func RenderCheck(raw []byte) error {
	if !utf8.Valid(raw) {
		return renderInvalidUTF8(raw)
	}
	return nil
}
