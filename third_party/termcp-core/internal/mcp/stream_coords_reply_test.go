package mcp

import (
	"runtime"
	"strings"
	"testing"
)

// streamOnlyScript prints stdout only, then stderr only, with a pause between so the
// two land in separate write spans — a pipe command that exits immediately can lose
// its last frame to the transport's drain race.
func streamOnlyScript(out, err string, tailMs int) string {
	if runtime.GOOS == "windows" {
		return `[Console]::Out.WriteLine('` + out + `'); Start-Sleep -m 300; ` +
			`[Console]::Error.WriteLine('` + err + `'); Start-Sleep -m ` + itoa(tailMs)
	}
	return `echo ` + out + `; sleep 0.3; echo ` + err + ` >&2; sleep 0.` + itoa(tailMs/100)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}

// TestAFilteredReadAddressesItsOwnStream is issue #202, at the reply a caller reads.
//
// `read <ch> --stream e` returns stderr, and the addresses in the reply used to be the
// CHANNEL's: a stdout byte's offset counted, so `Content-Range` described a span the
// body did not fill, `total` described the channel while the body was one stream, and
// `Next-Offset` was a number the caller could not resume from — it pointed into the
// other stream. The reply's numbers have to address the bytes the reply contains, or
// the two questions those headers exist to answer are both unanswerable.
func TestAFilteredReadAddressesItsOwnStream(t *testing.T) {
	s, _, shellID := startPipeStdioSession(t, streamOnlyScript("OUT-ONLY", "ERR-ONLY", 300))
	waitForOutput(t, s, shellID, "ERR-ONLY")

	e := readStream(t, s, shellID, "e", nil)
	if e["stream"] != "e" {
		t.Fatalf("the reply does not say which stream its addresses are in: %v", e["stream"])
	}
	// The stderr body is the whole stderr and nothing else.
	body := e["output"].(string)
	if !strings.Contains(body, "ERR-ONLY") || strings.Contains(body, "OUT-ONLY") {
		t.Fatalf("--stream e returned %q", body)
	}
	// Its span is measured in stderr bytes, so the body fits inside it.
	span := int64(e["stream_raw_bytes"].(float64))
	if span < int64(len("ERR-ONLY")) {
		t.Fatalf("the stderr span is %d bytes but the body is %d", span, len("ERR-ONLY"))
	}
	nl := 1
	if runtime.GOOS == "windows" {
		nl = 2 // WriteLine emits CRLF
	}
	if span != int64(len("ERR-ONLY")+nl) {
		t.Errorf("stderr holds %q plus its line ending, so %d bytes, but the reply says %d",
			"ERR-ONLY", len("ERR-ONLY")+nl, span)
	}
	// THE NUMBER THAT WAS A LIE: the channel total, in a reply whose body is one stream.
	chanTotal := int64(e["total_bytes"].(float64))
	streamTotal := int64(e["stream_total_bytes"].(float64))
	if streamTotal >= chanTotal {
		t.Fatalf("stderr (%d) cannot be as large as the whole channel (%d) when stdout is in it", streamTotal, chanTotal)
	}
	if streamTotal != span {
		t.Errorf("the read returned the whole stderr, so its total (%d) is its span (%d)", streamTotal, span)
	}
	// THE RESUMPTION POINT IS TYPEABLE. Resuming at stream_end in the reply's own
	// numbering must read nothing new, not jump into stdout.
	again := readStream(t, s, shellID, "e", map[string]any{"offset": e["stream_end"]})
	if out := again["output"].(string); out != "" {
		t.Fatalf("resuming at the reply's own stream_end returned %q: the number does not address the stream", out)
	}

	// The term-level reply is what a caller actually pages with.
	got := termText(t, s, map[string]any{
		"cmd": "read " + shellID + " --stream e --offset 0", "timeout": 1,
	})
	if !strings.Contains(got, "Content-Range: bytes 0-") {
		t.Fatalf("a stderr read starting at stderr byte 0 must say so:\n%s", got)
	}
	if strings.Contains(got, "/"+itoa(int(chanTotal))+"\n") {
		t.Errorf("Content-Range still totals the whole channel (%d) instead of the stream (%d):\n%s",
			chanTotal, streamTotal, got)
	}
}

// TestAnEmptyStreamReportsZeroBytes is the first half of issue #202, and the shape that
// made it a bug rather than a preference.
//
// A stdout-only channel read with `--stream e` selects nothing. It used to answer
// `Content-Range: bytes */7` — the channel's length — so a caller saw "seven bytes
// here" beside an empty body and could not tell that from "you have already read all
// the stderr there is". `*/0` says the one thing that is true.
func TestAnEmptyStreamReportsZeroBytes(t *testing.T) {
	s, _, shellID := startPipeStdioSession(t, streamOnlyScript("ONLY-OUT", "unused", 300))
	waitForOutput(t, s, shellID, "ONLY-OUT")

	got := termText(t, s, map[string]any{
		"cmd": "read " + shellID + " --stream e --offset 0", "timeout": 1,
	})
	if !strings.Contains(got, "Content-Range: bytes */0") {
		t.Fatalf("a stream with no bytes must total zero, not the channel's length:\n%s", got)
	}

	m := readStream(t, s, shellID, "e", nil)
	if total := int64(m["stream_total_bytes"].(float64)); total != 0 {
		t.Errorf("stderr is empty, so its total is 0, got %d", total)
	}
	if hasMore := m["has_more"].(bool); hasMore {
		t.Error("an empty stream has nothing more to read, whatever the channel holds")
	}
}

// TestAFilteredCursorReadAlsoSaysWhatItPassedOver is issue #203.
//
// The live-cursor branch declared its own `omitted` with `:=` inside the filter block,
// shadowing the one the struct literal reads, and then set the field on a `w` that the
// next line replaced wholesale. Two ways of losing the same number: every filtered read
// that named no offset reported `stream_omitted_bytes: 0` while the filter had skipped
// every stdout byte in the window.
//
// The channel has to INTERLEAVE for this to be visible at all: a read whose window holds
// only one stream has nothing to step over, and 0 is then the right answer. So the
// script writes stdout, then stderr, then stdout again.
func TestAFilteredCursorReadAlsoSaysWhatItPassedOver(t *testing.T) {
	var script string
	if runtime.GOOS == "windows" {
		script = `[Console]::Out.WriteLine('C1-OUT'); Start-Sleep -m 300; ` +
			`[Console]::Error.WriteLine('C1-ERR'); Start-Sleep -m 300; ` +
			`[Console]::Out.WriteLine('C2-OUT'); Start-Sleep -m 300`
	} else {
		script = `echo C1-OUT; sleep 0.3; echo C1-ERR >&2; sleep 0.3; echo C2-OUT; sleep 0.3`
	}
	s, _, shellID := startPipeStdioSession(t, script)
	waitForOutput(t, s, shellID, "C2-OUT")

	// No offset: this is the live cursor path, and its window is the whole channel —
	// which holds stdout on both sides of the stderr run.
	cursor := readStream(t, s, shellID, "e", nil)
	span := int64(cursor["stream_raw_bytes"].(float64))
	omitted := int64(cursor["stream_omitted_bytes"].(float64))
	raw := int64(cursor["raw_bytes"].(float64))
	if span == 0 {
		t.Fatalf("the cursor read returned no stderr: %v", cursor["output"])
	}
	if omitted == 0 {
		t.Fatalf("a %d byte channel window delivered %d stderr bytes and claims it stepped over "+
			"nothing: the cursor branch lost its count (issue #203)", raw, span)
	}
	if span+omitted != raw {
		t.Fatalf("the %d stderr bytes plus the %d it stepped over is not the %d byte channel window",
			span, omitted, raw)
	}
}
