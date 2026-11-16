package encoding

import (
	"encoding/json"
	"testing"
)

// TestUnescapeJSONKeepsChineseReadable pins the behaviour the browser door needed:
// a Chinese title arrives from Chrome as \uXXXX escapes, and the caller should get
// the characters instead of twelve bytes of escape per two characters.
func TestUnescapeJSONKeepsChineseReadable(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"plain", `{"value":"hello"}`, `{"value":"hello"}`},
		{"chinese", `{"value":"\u4e2d\u6587"}`, `{"value":"中文"}`},
		{"uppercase hex", `"\u4E2D"`, `"中"`},
		{"mixed", `"a\u4e2db"`, `"a中b"`},
		{"surrogate pair", `"\ud83c\udf89"`, `"🎉"`},
		// Content, not an escape: an escaped backslash followed by the letters u.
		{"escaped backslash", `"\\u4e2d"`, `"\\u4e2d"`},
		// Other escapes are the frame's business, not the decoder's.
		{"newline escape untouched", `"a\nb"`, `"a\nb"`},
		{"quote escape untouched", `"a\"b"`, `"a\"b"`},
		// Not decodable: leave exactly what the peer sent.
		{"lone high surrogate", `"\ud83c"`, `"\ud83c"`},
		{"bad hex", `"\u4e2z"`, `"\u4e2z"`},
		{"truncated", `"\u4e2"`, `"\u4e2"`},
		{"nothing to do", `{"method":"Page.enable"}`, `{"method":"Page.enable"}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := UnescapeJSON(tc.in); got != tc.want {
				t.Errorf("UnescapeJSON(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// TestUnescapeJSONStaysValidJSON is the invariant that makes the decoder safe to
// put in the reply path: decoding an escape into the character it names cannot
// turn a frame into something a JSON reader would refuse.
func TestUnescapeJSONStaysValidJSON(t *testing.T) {
	for _, s := range []string{
		`{"title":"\u4e2d\u6587\u9875\u9762","url":"https://example.com"}`,
		`{"s":"\ud83c\udf89 party","n":1}`,
	} {
		if got := UnescapeJSON(s); !jsonValid(got) {
			t.Errorf("UnescapeJSON(%q) = %q, which is not valid JSON", s, got)
		}
	}
}

func jsonValid(s string) bool {
	var v any
	return json.Unmarshal([]byte(s), &v) == nil
}

// The seam between two pages of one frame is a byte count this codebase picks, so
// it has to land on a boundary. These are the shapes that decide where.
func TestTrailingEscapeStart(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want int
	}{
		{"a whole escape", `a\u4e2d`, -1},
		{"three hex digits", `a\u4e2`, 1},
		{"one hex digit", `a\u4`, 1},
		{"the u alone", `a\u`, 1},
		{"a lone backslash", `a\`, 1},
		{"nothing cut", `plain ascii`, -1},
		{"no backslash at all", `{"id":5}`, -1},
		{"an escaped backslash is content", `a\\u4e2`, -1},
		{"a cut inside an escaped run", `a\\\u4`, 3},
		{"a complete escape after a cut one", `\u4e2d\u4e`, 6},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := TrailingEscapeStart(tc.in); got != tc.want {
				t.Errorf("TrailingEscapeStart(%q) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
