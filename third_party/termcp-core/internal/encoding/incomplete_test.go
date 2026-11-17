package encoding

import "testing"

// TestIncompleteTailUTF8 pins the rune rule: a trailing rune that is not finished is
// reported, and nothing else is.
//
// The distinction that matters is between "not finished" and "not UTF-8 at all":
// utf8.DecodeRune answers RuneError for both, and a GBK stream must keep being reported
// as GBK rather than quietly shortened by a byte or two.
func TestIncompleteTailUTF8(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want int // bytes to withhold
	}{
		{"ascii ends on a boundary", []byte("abcdefg"), 0},
		{"a two-byte rune is cut in half", []byte{'a', 'b', 0xc3}, 1},
		{"a three-byte rune is cut in half", []byte{'a', 'b', 0xe6, 0x97}, 2},
		{"a four-byte rune is cut in half", []byte{'a', 'b', 0xf0, 0x9f}, 2},
		{"a whole rune at the end is kept", []byte{'a', 'b', 0xc3, 0xa9}, 0},
		{"only the first byte of a rune", []byte{0xe6}, 1},
		{"empty", nil, 0},
		// Not UTF-8 at all: this belongs to the encoding check, which can name it.
		{"gbk bytes are not trimmed", []byte{0x81, 0x40, 0x82, 0xa0}, 0},
		{"binary is not trimmed", []byte{0xff, 0xfe, 0x00, 0x41}, 0},
		// A lead byte with something that is not a continuation is not a truncated
		// character — it is not UTF-8, and the encoding check owns that answer.
		{"a lead byte and a non-continuation", []byte{0xe6, 0x41}, 0},
		{"an invalid lead byte", []byte{0xf8, 0x88, 0x80}, 0},
		{"an overlong four-byte form", []byte{0xf0, 0x80, 0x80, 0x80}, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := IncompleteTail("", c.data); got != c.want {
				t.Errorf("IncompleteTail(\"\", % x) = %d, want %d", c.data, got, c.want)
			}
			// An explicit utf-8 spelling is the same rule.
			if got := IncompleteTail("utf-8", c.data); got != c.want {
				t.Errorf("IncompleteTail(\"utf-8\", % x) = %d, want %d", c.data, got, c.want)
			}
		})
	}
}

// TestIncompleteTailUsesTheDecoderAsTheJudge pins the declared-charset half.
//
// The rule cannot be a table of byte ranges — those are each charset's private
// business, and getting one wrong would trim real text. It is the round trip Sniff
// already uses: a truncated sequence decodes to U+FFFD, which the encoder then refuses,
// so "these bytes no longer round-trip but one byte fewer does" IS "the cut split a
// character".
func TestIncompleteTailUsesTheDecoderAsTheJudge(t *testing.T) {
	// A real GBK two-byte character (0xd6 0xd0 = 中), cut after its first byte.
	if got := IncompleteTail("gbk", []byte{'a', 0xd6}); got != 1 {
		t.Errorf("a GBK character cut in half: got %d bytes to withhold, want 1", got)
	}
	// Whole: nothing to withhold.
	if got := IncompleteTail("gbk", []byte{'a', 0xd6, 0xd0}); got != 0 {
		t.Errorf("a whole GBK character: got %d, want 0", got)
	}
	// The same bytes under a charset where 0xd6 is a COMPLETE single-byte character
	// (shift_jis half-width katakana): nothing is missing, so nothing is withheld.
	if got := IncompleteTail("shift_jis", []byte{'a', 0xd6}); got != 0 {
		t.Errorf("0xd6 is a whole shift_jis character: got %d, want 0", got)
	}
	// A real shift_jis two-byte character (0x82 0xa0 = あ), cut after its first byte.
	if got := IncompleteTail("shift_jis", []byte{'a', 'b', 0x82}); got != 1 {
		t.Errorf("a shift_jis character cut in half: got %d, want 1", got)
	}
	// A charset this build does not know has no opinion: the read is refused a layer
	// up with the list of charsets that exist, which beats a guess about its characters.
	if got := IncompleteTail("klingon", []byte{'a', 0xd6}); got != 0 {
		t.Errorf("an unknown charset must not be guessed at: got %d, want 0", got)
	}
	// Bytes that are not this charset at all are never trimmed: the decode path owns
	// that answer, and shortening the window by a byte or two would hide it.
	if got := IncompleteTail("gbk", []byte{0xff, 0xff, 0xff}); got != 0 {
		t.Errorf("garbage is not a truncated character: got %d, want 0", got)
	}
}
