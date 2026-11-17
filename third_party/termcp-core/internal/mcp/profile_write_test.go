package mcp

import (
	"strings"
	"testing"
)

func profilesTestServer(t *testing.T) *Server {
	t.Helper()
	s := newTestServer(t)
	s.ManageSSHConfigs = true
	return s
}

// Deleting a profile that does not exist must say so.
//
// Store.Delete is idempotent ON PURPOSE — an internal cleanup that runs twice should
// not fail — but the surface inherited that: `profile --action delete --name typo`
// answered 200 success, so a mistyped name looked like a successful removal. Every
// other delete in this system answers 404 (`ext delete`, `rm <session>`,
// `fwd --action close`), which is what makes this one wrong rather than a style.
func TestProfileDeleteOnAMissingNameIsRefused(t *testing.T) {
	s := profilesTestServer(t)
	text := termText(t, s, map[string]any{"cmd": "rm host never-existed"})
	if strings.Contains(text, "success") {
		t.Fatalf("deleting a missing profile answered success:\n%s", text)
	}
	if !strings.Contains(strings.ToLower(text), "not found") {
		t.Errorf("the refusal does not say the profile is missing:\n%s", text)
	}
}

// And deleting one that DOES exist still works, so the check did not simply block
// the operation.
func TestProfileDeleteOnAnExistingNameSucceeds(t *testing.T) {
	s := profilesTestServer(t)
	if text := termText(t, s, map[string]any{
		"cmd": "create host victim --host 192.0.2.1 --user u --password p",
	}); !strings.Contains(text, "success") {
		t.Fatalf("create failed:\n%s", text)
	}
	if text := termText(t, s, map[string]any{
		"cmd": "rm host victim",
	}); !strings.Contains(text, "success") {
		t.Fatalf("deleting an existing profile failed:\n%s", text)
	}
	// THE LISTING MADE THE ASSERTION MEANINGLESS. This read `edit host --action list`,
	// a form the command surface no longer has: the reply was a 400 about an unknown
	// flag, which of course does not contain the profile name, so "it is gone" passed
	// whatever the store held. Reading the real listing, and checking that it listed
	// anything at all, is what makes the absence below evidence.
	list := termText(t, s, map[string]any{"cmd": "ls host"})
	if !strings.HasPrefix(list, "TERM/1 2") {
		t.Fatalf("listing hosts was refused, so the assertion below proves nothing:\n%s", list)
	}
	if strings.Contains(list, "victim") {
		t.Fatalf("the profile is still listed after delete:\n%s", list)
	}
}
