package configcrypt

import (
	"bytes"
	"strings"
	"testing"
)

// A config export leaves the machine, so the properties that matter are the ones
// an attacker gets to test: the ciphertext must not leak the plaintext, a wrong
// password must be refused rather than producing garbage, and a modified file
// must not open. These are checked here because they cannot be checked by reading
// the export route.
func TestEncryptSealsAndDecryptOpens(t *testing.T) {
	// Real-looking TOML with a password in it: the exact thing this protects.
	plain := []byte("[[connections]]\nname = \"prod\"\nhost = \"10.0.0.5\"\npassword = \"hunter2\"\n")

	doc, err := Encrypt(plain, "correct horse battery staple")
	if err != nil {
		t.Fatal(err)
	}

	// The envelope announces itself, so an importer can tell it needs a password
	// rather than reporting a TOML parse error.
	if !IsEncrypted(doc) {
		t.Fatal("an encrypted document must carry the header")
	}
	if !bytes.HasPrefix(doc, []byte(Header)) {
		t.Fatalf("document must start with %q", Header)
	}

	// The secret must not be readable in the export. Both the password and a
	// distinctive host are checked: base64-encoding the whole payload is what
	// makes a naive "it looks scrambled" test pass on a plaintext file.
	for _, secret := range []string{"hunter2", "10.0.0.5", "connections"} {
		if bytes.Contains(doc, []byte(secret)) {
			t.Errorf("the export leaks %q", secret)
		}
	}

	got, err := Decrypt(doc, "correct horse battery staple")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatalf("round trip changed the document:\n got %q\nwant %q", got, plain)
	}
}

// A wrong password must be refused. GCM authenticates, so this is a real check
// rather than a guess — and it must not "succeed" with garbage, which is what an
// unauthenticated mode would have done.
func TestDecryptRefusesAWrongPassword(t *testing.T) {
	doc, err := Encrypt([]byte("secret config"), "right")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Decrypt(doc, "wrong"); err == nil {
		t.Fatal("a wrong password must not decrypt")
	}
	// An empty password is a wrong password, not an invitation to skip the check.
	if _, err := Decrypt(doc, ""); err == nil {
		t.Fatal("an empty password must not decrypt")
	}
}

// Tampering must be detected. Flipping one character of the ciphertext is what a
// careless edit or a truncated paste looks like.
func TestDecryptRefusesTamperedCiphertext(t *testing.T) {
	doc, err := Encrypt([]byte("secret config"), "pw")
	if err != nil {
		t.Fatal(err)
	}
	// Flip a byte inside the base64 payload (past the header line), keeping the
	// document valid base64 so it reaches the authentication check rather than
	// failing as corrupt text.
	lines := strings.Split(string(doc), "\n")
	last := len(lines) - 2 // the final element is the trailing newline
	target := []byte(lines[last])
	if target[0] == 'A' {
		target[0] = 'B'
	} else {
		target[0] = 'A'
	}
	lines[last] = string(target)
	if _, err := Decrypt([]byte(strings.Join(lines, "\n")), "pw"); err == nil {
		t.Fatal("a tampered document must not decrypt")
	}
}

// A plain TOML export is not an error: it is the old format, and the importer
// needs to tell "not encrypted" from "wrong password" to give the right advice.
func TestPlainDocumentIsNotMistakenForEncrypted(t *testing.T) {
	plain := []byte("[[connections]]\nname = \"prod\"\n")
	if IsEncrypted(plain) {
		t.Fatal("plain TOML must not report as encrypted")
	}
	if _, err := Decrypt(plain, "pw"); err != ErrNotEncrypted {
		t.Fatalf("Decrypt(plain) = %v, want ErrNotEncrypted", err)
	}

	// A file that begins with the header word but is not ours still reports as
	// encrypted (the prefix is the format marker) and fails with a decode error
	// rather than being silently imported as TOML.
	fake := []byte(Header + "\nnot base64!!\n")
	if !IsEncrypted(fake) {
		t.Fatal("the header prefix is what marks a file as encrypted")
	}
	if _, err := Decrypt(fake, "pw"); err == nil {
		t.Fatal("a malformed envelope must not decrypt")
	}
}

// Encrypting the same input twice must produce different files: a fresh salt and
// nonce mean an operator can diff two exports without learning whether the
// config changed.
func TestEncryptIsNonDeterministic(t *testing.T) {
	plain := []byte("same config")
	a, err := Encrypt(plain, "pw")
	if err != nil {
		t.Fatal(err)
	}
	b, err := Encrypt(plain, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(a, b) {
		t.Fatal("two exports of the same config are identical; the salt or nonce is not fresh")
	}
	// Both still open with the same password.
	for _, doc := range [][]byte{a, b} {
		if _, err := Decrypt(doc, "pw"); err != nil {
			t.Fatalf("Decrypt: %v", err)
		}
	}
}

// An empty password must be refused rather than producing a file that looks
// protected and is not.
func TestEncryptRequiresAPassword(t *testing.T) {
	if _, err := Encrypt([]byte("x"), ""); err == nil {
		t.Fatal("encrypting with no password must fail; the caller exports plain instead")
	}
}

// A large export (the whole ssh store with many profiles) must round-trip: the
// line wrapping has to be lossless across many lines, not just one.
func TestEncryptHandlesADocumentLongerThanOneLine(t *testing.T) {
	plain := []byte(strings.Repeat("[[connections]]\nname = \"n\"\nhost = \"h\"\n", 500))
	doc, err := Encrypt(plain, "pw")
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(string(doc), "\n"); n < 10 {
		t.Fatalf("a %d-byte document produced only %d lines; the wrapping is wrong", len(plain), n)
	}
	got, err := Decrypt(doc, "pw")
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}
	if !bytes.Equal(got, plain) {
		t.Fatal("a long document did not survive the round trip")
	}
}
