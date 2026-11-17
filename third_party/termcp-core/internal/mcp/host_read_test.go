package mcp

import (
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/sshconfig"
)

// TestGetHostReadsOneEntryBack covers the hole that made a partial edit a guess: a host
// could be created, edited, listed and deleted, and never read.
//
// Reading is not a mutation, so it is available without --mcp-manage-ssh-configs: the
// listing already is, and hiding the fields while showing the name protects nothing.
func TestGetHostReadsOneEntryBack(t *testing.T) {
	dir := t.TempDir()
	store := sshconfig.NewStore(dir)
	s := New(nil, nil, store, nil, "test")
	// Deliberately NOT enabling ManageSSHConfigs: reading must work regardless.
	if err := store.Save("box", []byte(`kind = "remote"
host = "10.0.0.5"
port = 2222
user = "deploy"
password = "topsecret"
description = "prod box"
`)); err != nil {
		t.Fatal(err)
	}

	got := termText(t, s, map[string]any{"cmd": "get host box"})
	if !strings.HasPrefix(got, "TERM/1 2") {
		t.Fatalf("get host was refused:\n%s", got)
	}
	for _, want := range []string{"10.0.0.5", "2222", "deploy", "prod box"} {
		if !strings.Contains(got, want) {
			t.Errorf("the reply omits %q:\n%s", want, got)
		}
	}

	// THE PASSWORD MUST NOT COME BACK. It would land in a transcript, and the
	// caller's real question — "is one set?" — is answered by the boolean.
	if strings.Contains(got, "topsecret") {
		t.Errorf("the password was returned:\n%s", got)
	}
	if !strings.Contains(got, "has_password") {
		t.Errorf("the reply does not say whether a password is set:\n%s", got)
	}

	// A missing entry is a 404 naming the entry, not a blank success.
	missing := termText(t, s, map[string]any{"cmd": "get host nosuch-entry"})
	if !strings.HasPrefix(missing, "TERM/1 404") {
		t.Errorf("a missing host was not a 404:\n%s", missing)
	}
}

// TestLsHostRefusesAName pins that a name passed to `ls host` is answered, not dropped.
//
// The handler lists every entry, so `ls host prod` returned the whole table and the
// caller had no way to tell that the word had been ignored.
func TestLsHostRefusesAName(t *testing.T) {
	dir := t.TempDir()
	store := sshconfig.NewStore(dir)
	s := New(nil, nil, store, nil, "test")
	if err := store.Save("box", []byte("kind = \"remote\"\nhost = \"h\"\nuser = \"u\"\npassword = \"p\"\n")); err != nil {
		t.Fatal(err)
	}

	got := termText(t, s, map[string]any{"cmd": "ls host box"})
	if !strings.HasPrefix(got, "TERM/1 4") {
		t.Fatalf("ls host with a name was accepted:\n%s", got)
	}
	if !strings.Contains(got, "get host box") {
		t.Errorf("the refusal does not point at the verb that does this:\n%s", got)
	}
}
