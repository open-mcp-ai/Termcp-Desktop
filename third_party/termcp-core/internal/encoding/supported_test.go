package encoding

import (
	"fmt"
	"strings"
	"testing"
)

// TestAdvertisedListCannotDrift is the guard the hand-written refusal sentence
// needed and did not have: a name this build offers must resolve, and an encoding
// this build accepts must be reachable from one of the names it offers.
//
// Both directions matter. A name in the list that does not resolve sends a caller
// to a refusal; an encoding in the table that no advertised name reaches is one a
// caller is told does not exist while the decoder sits in the binary — which is
// exactly what "use utf-8, gbk, gb18030, big5, shift_jis, euc-kr or utf-16" did
// to gb2312, cp950, cp949, euc-jp, latin-1 and utf-16le.
func TestAdvertisedListCannotDrift(t *testing.T) {
	for _, name := range advertised {
		if _, ok := charsets[normaliseCharset(name)]; !ok {
			t.Errorf("advertised charset %q is not in the table", name)
		}
	}
	reachable := map[string]bool{}
	for _, name := range advertised {
		reachable[encoderKey(charsets[normaliseCharset(name)])] = true
	}
	for name, enc := range charsets {
		if name == "" {
			continue
		}
		if !reachable[encoderKey(enc)] {
			t.Errorf("charset %q is accepted but no advertised name reaches its encoding", name)
		}
	}
}

// encoderKey identifies an encoding by the object behind it. The table maps six
// spellings onto one encoder, so the identity has to come from the value, not from
// the name.
func encoderKey(enc interface{}) string {
	if enc == nil {
		return "utf-8"
	}
	return fmt.Sprintf("%p", enc)
}

// TestGB2312IsTheConsoleCodePage pins the mapping that was silently wrong: a
// caller asking for gb2312 means the code page their zh-CN console writes, which is
// CP936, and HZGB2312 — the 7-bit escape form from mail — returned plausible
// garbage for the same bytes with no error at all.
func TestGB2312IsTheConsoleCodePage(t *testing.T) {
	gbkBytes := []byte{0xD6, 0xD0, 0xCE, 0xC4} // 中文 in CP936
	for _, name := range []string{"gbk", "cp936", "gb2312", "euc-cn", "gb18030"} {
		out, err := Decode(name, gbkBytes)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if string(out) != "中文" {
			t.Errorf("%s decoded the console's bytes as %q, want 中文", name, out)
		}
	}
	// The escape encoding is still reachable, under a name that means it.
	if _, ok := charsets["hz-gb-2312"]; !ok {
		t.Error("hz-gb-2312 should still resolve: it is a real encoding, just not what gb2312 means")
	}
}

// TestUnknownCharsetOffersWhatExists pins the refusal itself: it names encodings
// this build has, and it does not name any it does not.
func TestUnknownCharsetOffersWhatExists(t *testing.T) {
	_, err := Decode("klingon", []byte("x"))
	if err == nil {
		t.Fatal("an unknown charset should be refused rather than read as UTF-8")
	}
	msg := err.Error()
	if !strings.Contains(msg, "klingon") {
		t.Errorf("the refusal should name what was asked for: %q", msg)
	}
	for _, want := range []string{"gbk", "big5", "shift_jis", "euc-kr", "utf-16"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal should offer %s: %q", want, msg)
		}
	}
}
