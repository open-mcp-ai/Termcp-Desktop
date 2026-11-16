package encoding

import (
	"bytes"
	"strings"
	"testing"
)

// UTF-16 IS WHAT WINDOWS WRITES BY DEFAULT.
//
// PowerShell 5.1's `>` and `Out-File` with no -Encoding write UTF-16LE with a BOM, so
// "redirect a long command to a file and poll it" — the pattern this tool recommends
// for work that outlives a session — produced a file the build could not read. The
// reply said `mode: text` and handed back `\xff\xfeP\x00S\x00...`: true, and useless
// (issue #177).
func TestUTF16DecodesWhatPowerShellWrites(t *testing.T) {
	// Exactly what `'PSModulePath' | Out-File -Encoding Unicode` writes: a BOM and
	// then every ASCII byte followed by a zero.
	raw := []byte{0xFF, 0xFE}
	for _, r := range "PSModulePath" {
		raw = append(raw, byte(r), 0)
	}
	raw = append(raw, '\r', 0, '\n', 0)

	for _, name := range []string{"utf-16", "utf-16le"} {
		out, err := Decode(name, raw)
		if err != nil {
			t.Fatalf("Decode(%q): %v", name, err)
		}
		if got := strings.TrimSpace(strings.TrimPrefix(string(out), "\ufeff")); got != "PSModulePath" {
			t.Fatalf("Decode(%q) = %q, want PSModulePath", name, got)
		}
	}

	// The BOM decides the endianness when the caller does not say, which is the case
	// that matters: a caller reading a log usually has no idea how it was written.
	be := []byte{0xFE, 0xFF, 0, 'P', 0, 'S'}
	out, err := Decode("utf-16", be)
	if err != nil {
		t.Fatalf("Decode(utf-16, BE): %v", err)
	}
	if string(out) != "PS" {
		t.Fatalf("utf-16 with a big-endian BOM decoded to %q, want PS", out)
	}
}

// A BYTE ORDER MARK IS PROOF; a round trip is not.
//
// UTF-16 decoding is permissive — almost any even-length byte string decodes to
// something — so Sniff cannot use its round-trip test there. The two bytes the writer
// chose are unambiguous, and they are what the refusal message quotes back.
func TestSniffRecognisesAUTF16BOM(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"little endian", []byte{0xFF, 0xFE, 'P', 0, 'S', 0}, "utf-16le"},
		{"big endian", []byte{0xFE, 0xFF, 0, 'P', 0, 'S'}, "utf-16be"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Sniff(c.data); got != c.want {
				t.Fatalf("Sniff = %q, want %q", got, c.want)
			}
		})
	}
	// UTF-8 with a BOM is not UTF-16, and the answer must not claim it is.
	if got := Sniff([]byte{0xEF, 0xBB, 0xBF, 'h', 'i'}); strings.HasPrefix(got, "utf-16") {
		t.Fatalf("Sniff claimed %q for a UTF-8 BOM", got)
	}
}

// THE REFUSAL NAMES A FLAG THIS BUILD ANSWERS.
//
// The message that tells a caller "pass --encoding <charset>" is only useful if the
// charset it suggests is in the table: it used to list a set that did not include the
// encoding Windows writes by default.
func TestTheRefusalNamesASpellingTheTableAccepts(t *testing.T) {
	msg := (&UnknownCharsetError{Name: "utf-32"}).Error()
	for _, name := range []string{"utf-16", "utf-16le", "utf-16be"} {
		if !strings.Contains(msg, name) {
			t.Fatalf("the refusal does not mention %q: %s", name, msg)
		}
		if _, ok := KnownCharset(name); !ok {
			t.Fatalf("the refusal names %q but the table does not accept it", name)
		}
	}
}

// THE BYTES ARE STILL THE BYTES. Decoding is a rendering, and a caller paging a file by
// offset must not be handed a length that changed underneath it.
func TestDecodingUTF16LeavesTheInputAlone(t *testing.T) {
	raw := []byte{0xFF, 0xFE, 'a', 0}
	before := append([]byte(nil), raw...)
	if _, err := Decode("utf-16", raw); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if !bytes.Equal(raw, before) {
		t.Fatalf("Decode modified its input: %v -> %v", before, raw)
	}
}
