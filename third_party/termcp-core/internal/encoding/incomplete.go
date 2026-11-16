package encoding

import (
	"bytes"
	"unicode/utf8"

	"golang.org/x/text/encoding"
)

// IncompleteTail reports how many bytes at the END of a window cannot complete a
// character in the named charset, so a reader can hand back whole ones and leave the
// rest for the next read.
//
// WHY THE RULE LIVES HERE. A byte window is cut wherever the caller's `bytes_limit`
// falls, which can be inside a character — and then the window is not valid text, so
// the byte-fidelity check refuses the whole reply and names a charset the session is
// not writing. The decision "where does a character end" is a property of the CHARSET,
// and the charset is a property of the read request; this package is the only one that
// knows both, so it is the only one that can answer.
//
// UTF-8 IS THE RUNE RULE: an incomplete rune is a lead byte followed by the right
// continuations and not enough of them. Nothing else is trimmed — a stream that is not
// UTF-8 must keep reaching the refusal that names it.
//
// ANY OTHER CHARSET USES THE DECODER AS THE JUDGE, by ROUND TRIP — the same test Sniff
// uses. A truncated sequence decodes to U+FFFD, which the encoder then refuses, so
// "these bytes no longer round-trip but one byte fewer does" is exactly "the cut split
// a character". It is deliberately not a table of byte ranges: those are each charset's
// private business, and getting one wrong would trim real text.
//
// A charset this build does not know returns 0. The read is refused a layer up with
// the list of charsets that exist, which is a better answer than a guess about where
// its characters end.
func IncompleteTail(name string, data []byte) int {
	if len(data) == 0 {
		return 0
	}
	norm, known := KnownCharset(name)
	if !known {
		return 0
	}
	if norm == "utf-8" {
		return incompleteUTF8Tail(data)
	}
	// norm is not utf-8 here, so the map holds a real decoder: every UTF-8 spelling
	// normalises to "utf-8" and was answered above.
	enc := charsets[norm]
	if roundTrips(enc, data) {
		return 0
	}
	// A sequence is at most four bytes in every charset this build carries (GB18030's
	// four-byte form is the longest), so the search is bounded and short.
	for back := 1; back <= 4 && back < len(data); back++ {
		if roundTrips(enc, data[:len(data)-back]) {
			return back
		}
	}
	return 0
}

// roundTrips reports whether data decodes and re-encodes to itself: the bytes ARE this
// charset, rather than merely happening to decode as it.
func roundTrips(enc encoding.Encoding, data []byte) bool {
	out, err := enc.NewDecoder().Bytes(data)
	if err != nil || !utf8.Valid(out) {
		return false
	}
	back, err := enc.NewEncoder().Bytes(out)
	return err == nil && bytes.Equal(back, data)
}

// incompleteUTF8Tail reports how many trailing bytes are a truncated rune.
//
// The distinction that matters is between "not finished" and "not UTF-8 at all":
// utf8.DecodeRune answers RuneError for both, and a GBK stream must keep being reported
// as GBK rather than quietly shortened by a byte or two.
func incompleteUTF8Tail(data []byte) int {
	if len(data) == 0 || utf8.Valid(data) {
		return 0
	}
	start := len(data) - 1
	for i := 0; i < utf8.UTFMax-1 && start > 0 && data[start]&0xC0 == 0x80; i++ {
		start--
	}
	if incompleteRune(data[start:]) == 0 || !utf8.Valid(data[:start]) {
		return 0
	}
	return len(data) - start
}

// incompleteRune reports how long a TRUNCATED rune at the start of tail is, or 0 when
// tail is complete or is not UTF-8 at all.
func incompleteRune(tail []byte) int {
	if len(tail) == 0 || len(tail) >= utf8.UTFMax {
		return 0
	}
	var need int
	switch b := tail[0]; {
	case b < 0x80:
		return 0
	case b&0xE0 == 0xC0:
		need = 2
	case b&0xF0 == 0xE0:
		need = 3
	case b&0xF8 == 0xF0:
		need = 4
	default:
		return 0
	}
	if len(tail) >= need {
		return 0
	}
	for _, b := range tail[1:] {
		if b&0xC0 != 0x80 {
			return 0 // a lead byte without continuations is not UTF-8
		}
	}
	return len(tail)
}
