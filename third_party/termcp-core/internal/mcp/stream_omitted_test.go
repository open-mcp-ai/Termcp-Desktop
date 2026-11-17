package mcp

import (
	"context"
	"runtime"
	"strings"
	"testing"
)

// A FILTERED READ HAS TO SAY WHAT IT PASSED OVER.
//
// `--stream o` answers "stdout only" by selecting a subset of the span the window covers,
// and the span still runs to End — the cursor must move by the range or a poll loop would
// re-read the same bytes forever. So the bytes of the other stream leave the caller's
// cursor without ever being shown, and nothing said so. The worst case was not a partial
// body: `--stream e` on a stdout-only channel answered 200 with an EMPTY body and
// `Content-Range: bytes 0-7/8` — "eight bytes here", zero delivered — and the plain read
// after it returned 204. Eight bytes of output were consumed by a reply that displayed
// nothing at all, and no line in either reply mentioned them.
func TestAStreamFilteredReadSaysWhatItPassedOver(t *testing.T) {
	var script string
	if runtime.GOOS == "windows" {
		script = `[Console]::Out.WriteLine('OUT-1'); Start-Sleep -m 300; [Console]::Error.WriteLine('ERR-1'); Start-Sleep -m 300; [Console]::Out.WriteLine('OUT-2'); Start-Sleep -m 300`
	} else {
		script = `echo OUT-1; sleep 0.3; echo ERR-1 >&2; sleep 0.3; echo OUT-2; sleep 0.3`
	}
	s, _, shellID := startPipeStdioSession(t, script)
	waitForOutput(t, s, shellID, "OUT-2")

	// Each reply has TWO spans, and they are both facts about the same window: the
	// bytes of the stream the caller asked for (stream_raw_bytes) and the bytes of the
	// other stream it stepped over (stream_omitted_bytes). Together they are the
	// channel window the read covered, in the one unit where the two are comparable —
	// raw bytes, not bytes_returned, which is the RENDERED length and would compare a
	// filtered raw count with a post-strip character count.
	o := readStream(t, s, shellID, "o", nil)
	e := readStream(t, s, shellID, "e", nil)
	for _, m := range []map[string]any{o, e} {
		span := int64(m["stream_raw_bytes"].(float64))
		omitted := int64(m["stream_omitted_bytes"].(float64))
		raw := int64(m["raw_bytes"].(float64))
		if span+omitted != raw {
			t.Fatalf("%s: the %d bytes of the stream plus the %d it passed over is not the %d byte channel window",
				m["stream"], span, omitted, raw)
		}
	}
	omittedOut := int64(o["stream_omitted_bytes"].(float64))
	omittedErr := int64(e["stream_omitted_bytes"].(float64))
	if omittedOut <= 0 || omittedErr <= 0 {
		t.Fatalf("a filtered read passed over nothing: stdout omitted %d, stderr omitted %d", omittedOut, omittedErr)
	}
	// And the two streams together are the channel: no byte is in both and none in
	// neither, which is what makes paging in either numbering safe.
	if sum := int64(o["stream_raw_bytes"].(float64)) + int64(e["stream_raw_bytes"].(float64)); sum != int64(o["total_bytes"].(float64)) {
		t.Fatalf("stdout and stderr hold %d bytes together but the channel holds %d", sum, int64(o["total_bytes"].(float64)))
	}

	// And the term-level reply, which is what a caller reads, names them and the flag that
	// shows them. This is the reply that used to be silent.
	got := termText(t, s, map[string]any{
		"cmd": "read " + shellID + " --stream o --offset 0", "timeout": 1,
	})
	if !strings.Contains(got, "Omitted:") {
		t.Fatalf("a filtered read that passed over %d bytes must say so:\n%s", omittedOut, got)
	}
	if !strings.Contains(got, "--stream e") {
		t.Errorf("the Omitted header must name the flag that shows them:\n%s", got)
	}
}

// TestAStreamFilterThatSelectsNothingIsNotAnEmptyChannel is the case that read as a bug.
//
// A stdout-only channel read with `--stream e` selects nothing, and the reply said exactly
// that and nothing else — 200, empty body, a Content-Range covering all eight bytes. The
// bytes existed, they were about to be consumed by this reply, and the one fact that would
// have explained the empty body was the one fact missing.
func TestAStreamFilterThatSelectsNothingIsNotAnEmptyChannel(t *testing.T) {
	var script string
	if runtime.GOOS == "windows" {
		script = `[Console]::Out.WriteLine('ONLY-OUT'); Start-Sleep -m 600`
	} else {
		script = `echo ONLY-OUT; sleep 0.6`
	}
	s, _, shellID := startPipeStdioSession(t, script)
	waitForOutput(t, s, shellID, "ONLY-OUT")

	m := readStream(t, s, shellID, "e", nil)
	if out := m["output"].(string); out != "" {
		t.Fatalf("stream=e on a stdout-only channel returned %q", out)
	}
	if omitted := int64(m["stream_omitted_bytes"].(float64)); omitted == 0 {
		t.Fatal("a filtered read that returned nothing must still say how much it passed over")
	}

	// BOTH SPELLINGS. `--offset 0` and the default cursor are two paths into the same
	// question, and only one of them used to answer: the live cursor path shadowed the
	// count inside its own if-block, so `stream_omitted_bytes` came back 0 for every
	// filtered read that did not name an offset (issue #202). A test that pins one
	// spelling cannot see that.
	for _, suffix := range []string{" --offset 0", ""} {
		got := termText(t, s, map[string]any{
			"cmd": "read " + shellID + " --stream e" + suffix, "timeout": 1,
		})
		if !strings.Contains(got, "Omitted:") {
			t.Fatalf("an empty body under a full Content-Range must explain itself (suffix %q):\n%s", suffix, got)
		}
	}
}

// TestAnUnfilteredReadSaysNothingAboutStreams is the reverse: the header is a fact about
// filtering, and a plain read did not filter, so it must not carry it. A header that
// appears everywhere is noise, and noise is how a real one gets skimmed past.
func TestAnUnfilteredReadSaysNothingAboutStreams(t *testing.T) {
	var script string
	if runtime.GOOS == "windows" {
		script = `[Console]::Out.WriteLine('PLAIN-OUT'); Start-Sleep -m 300`
	} else {
		script = `echo PLAIN-OUT; sleep 0.3`
	}
	s, _, shellID := startPipeStdioSession(t, script)
	waitForOutput(t, s, shellID, "PLAIN-OUT")

	got := termText(t, s, map[string]any{"cmd": "read " + shellID + " --offset 0", "timeout": 1})
	if strings.Contains(got, "Omitted:") {
		t.Errorf("an unfiltered read passed nothing over:\n%s", got)
	}
	// --stream all is the same statement in other words.
	got = termText(t, s, map[string]any{"cmd": "read " + shellID + " --stream all --offset 0", "timeout": 1})
	if strings.Contains(got, "Omitted:") {
		t.Errorf("--stream all filters nothing:\n%s", got)
	}
	_ = context.Background()
}
