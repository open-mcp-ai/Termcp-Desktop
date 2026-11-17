// Package configcrypt encrypts a configuration document with a password, for
// export and import only.
//
// This is deliberately not the storage encryption of issue #108's first two
// items. Those ask for transparent at-rest encryption through the OS keychain
// (DPAPI, systemd-creds, macOS Keychain), which is a different problem with a
// different threat model: it protects a file from other users of the same
// machine, needs a fallback where no keychain exists (containers), and must stay
// invisible to the user. This package solves the third item — "carry the
// configuration to another machine" — where the only shared secret available is
// one the person types, and where being visible is the point: an exported file
// is about to leave the machine.
//
// The format is a single self-describing envelope, so a file that predates or
// postdates a parameter change still says what it needs:
//
//	TERMCP-ENC 1
//	<base64 of salt || nonce || AES-256-GCM ciphertext>
//
// Everything after the header is base64 so the document survives mail, chat and
// copy-paste; the header means an encrypted file is recognisable as one (an
// operator who tries to import it without the password gets "this is
// encrypted" rather than a TOML parse error).
package configcrypt

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
)

// Header is the first line of an encrypted document.
//
// The version is part of the file rather than of the build: a future change to
// the KDF or the cipher adds "TERMCP-ENC 2" and this reader keeps opening v1,
// which is what makes old exports importable after an upgrade.
const Header = "TERMCP-ENC 1"

// Parameters. They are fixed constants rather than tunable options: an export is
// read by an importer that has to guess nothing, and a knob would let two
// deployments produce files the other cannot open.
const (
	// saltLen is 16 bytes, the usual size for PBKDF2-HMAC-SHA256.
	saltLen = 16
	// keyLen is 32, i.e. AES-256.
	keyLen = 32
	// iterations is OWASP's 2023 floor for PBKDF2-HMAC-SHA256. It is written into
	// nothing and cannot be changed without a new header version, which is the
	// point: a file records its version, not its parameters, so there is exactly
	// one set of parameters per version.
	iterations = 600_000
)

// ErrNotEncrypted reports a document that was not produced by Encrypt. The caller
// treats it as "plain TOML, import normally".
var ErrNotEncrypted = errors.New("not an encrypted configuration document")

// ErrWrongPassword reports a failed authentication: the password is wrong, or the
// file was modified. The two are indistinguishable by design — GCM authenticates,
// it does not identify.
var ErrWrongPassword = errors.New("wrong password, or the file has been altered")

// IsEncrypted reports whether a document carries the envelope header. It is the
// one question an importer asks first, so it is deliberately cheap and total: a
// file that merely begins with the word TERMCP-ENC but is not ours still answers
// true here and fails later with a clear error.
func IsEncrypted(doc []byte) bool {
	return strings.HasPrefix(strings.TrimLeft(string(doc), "\r\n"), Header)
}

// Encrypt seals plaintext with password and returns the envelope as bytes.
//
// A password is required: an empty one would produce a file that looks protected
// and is not, which is worse than an uncompressed plain export the operator can
// see is plain. The caller decides what "no password" means (export unencrypted).
func Encrypt(plaintext []byte, password string) ([]byte, error) {
	if password == "" {
		return nil, errors.New("a password is required to encrypt a configuration export")
	}
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return nil, fmt.Errorf("generate salt: %w", err)
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyLen)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	// The header is authenticated as additional data: it names the format, so a
	// file rewritten to claim another version fails to open rather than being
	// reinterpreted under the wrong rules.
	sealed := gcm.Seal(nil, nonce, plaintext, []byte(Header))

	payload := make([]byte, 0, len(salt)+len(nonce)+len(sealed))
	payload = append(payload, salt...)
	payload = append(payload, nonce...)
	payload = append(payload, sealed...)

	var out strings.Builder
	out.WriteString(Header)
	out.WriteString("\n")
	// 64 columns: the line length a terminal, an editor and a mail client all
	// wrap at without inserting anything.
	encoded := base64.StdEncoding.EncodeToString(payload)
	for len(encoded) > 64 {
		out.WriteString(encoded[:64])
		out.WriteString("\n")
		encoded = encoded[64:]
	}
	out.WriteString(encoded)
	out.WriteString("\n")
	return []byte(out.String()), nil
}

// Decrypt opens an envelope produced by Encrypt.
func Decrypt(doc []byte, password string) ([]byte, error) {
	text := strings.TrimLeft(string(doc), "\r\n")
	if !strings.HasPrefix(text, Header) {
		return nil, ErrNotEncrypted
	}
	if password == "" {
		return nil, ErrWrongPassword
	}
	body := strings.TrimPrefix(text, Header)
	// Whitespace is not base64, and the line wrapping above is not significant.
	body = strings.Join(strings.Fields(body), "")
	payload, err := base64.StdEncoding.DecodeString(body)
	if err != nil {
		return nil, fmt.Errorf("corrupt envelope: %w", err)
	}
	if len(payload) < saltLen {
		return nil, errors.New("corrupt envelope: too short")
	}
	salt := payload[:saltLen]
	rest := payload[saltLen:]

	key, err := pbkdf2.Key(sha256.New, password, salt, iterations, keyLen)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("gcm: %w", err)
	}
	if len(rest) < gcm.NonceSize() {
		return nil, errors.New("corrupt envelope: missing nonce")
	}
	nonce, sealed := rest[:gcm.NonceSize()], rest[gcm.NonceSize():]
	plain, err := gcm.Open(nil, nonce, sealed, []byte(Header))
	if err != nil {
		return nil, ErrWrongPassword
	}
	return plain, nil
}
