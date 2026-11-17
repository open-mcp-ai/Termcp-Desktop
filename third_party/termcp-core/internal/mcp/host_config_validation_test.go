package mcp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/sshconfig"
)

// seedRawProfile writes a config.toml straight to disk, bypassing Save.
//
// Save validates, which is the point of Save — but these tests are about a file
// that is already on disk and never went through it: an entry the user hand-edited,
// or one written by an older version that accepted it. That is exactly the state
// issue #154 found, where every such entry appeared in `ls host` looking fine.
func seedRawProfile(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, "ssh_configs", name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(path, "config.toml"), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestLsHostSaysWhichEntryIsBroken is the first half of issue #154.
//
// Every entry used to be printed as a bare name whether or not it parsed, so the
// only way to learn that a profile was unusable was to try connecting to it — and
// an inventory or a runbook built from `ls host` claimed hosts that could never be
// reached. The name stays in the listing (an entry the user wrote and cannot see is
// worse than one they can see is broken) and the reason travels with it.
func TestLsHostSaysWhichEntryIsBroken(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	seedRawProfile(t, dir, "zz-badkind", "kind = \"bogus\"\nhost = \"1.2.3.4\"\nuser = \"x\"\n")
	seedRawProfile(t, dir, "zz-good", "kind = \"remote\"\nhost = \"10.0.0.9\"\nuser = \"u\"\npassword = \"p\"\n")
	s.sshConfigs = sshconfig.NewStore(dir)

	out := termText(t, s, map[string]any{"cmd": "ls host"})
	if !strings.Contains(out, "zz-badkind") {
		t.Fatalf("ls host hid the broken entry instead of marking it:\n%s", out)
	}
	if !strings.Contains(out, "broken:") {
		t.Fatalf("ls host did not say the entry is broken:\n%s", out)
	}
	if !strings.Contains(out, "unknown kind") {
		t.Fatalf("ls host did not say WHY it is broken:\n%s", out)
	}
	// The column is for the operator, not for the Go error chain: the sentinel that
	// classified the failure must not ride along with the sentence.
	if strings.Contains(out, "invalid ssh config") {
		t.Fatalf("ls host leaked the error sentinel into the listing:\n%s", out)
	}
	// The good entry must not be tarred with the same brush: a column that marks
	// everything marks nothing.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "zz-good") && strings.Contains(line, "broken:") {
			t.Fatalf("a working entry was reported broken: %q", line)
		}
	}
}

// TestABadProfileIsTheCallersError is the second half of issue #154.
//
// `connect` answered 500 operation_failed for a profile the CALLER wrote badly. The
// message was already client-facing validation ("unknown kind ... (use ...)"); the
// status said the server broke, which invites a retry that cannot succeed. 4xx is
// the whole difference between "fix your config" and "try again".
func TestABadProfileIsTheCallersError(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	seedRawProfile(t, dir, "zz-kindless", "description = \"no kind here\"\n")
	seedRawProfile(t, dir, "zz-badmode", "kind = \"internal\"\ndefault_mode = \"bogus\"\n")
	seedRawProfile(t, dir, "zz-dialfield", "kind = \"internal\"\nhost = \"10.99.99.99\"\n")
	s.sshConfigs = sshconfig.NewStore(dir)

	for _, name := range []string{"zz-kindless", "zz-badmode", "zz-dialfield"} {
		out := termText(t, s, map[string]any{"cmd": "connect " + name})
		if code := statusCode(t, out); code != 400 {
			t.Errorf("connect %s answered %d, want 400:\n%s", name, code, out)
		}
	}
}

// TestInternalProfileRefusesDialFields pins the fourth finding of issue #154.
//
// `kind = "internal"` with host/user/port beside it connected to localhost and never
// contacted 10.99.99.99. That is the one combination where the caller's mental model
// and reality diverge with no signal at all, so the combination is refused and the
// message names the fields to delete.
func TestInternalProfileRefusesDialFields(t *testing.T) {
	_, err := sshconfig.ParseAndValidate([]byte("kind = \"internal\"\nhost = \"10.99.99.99\"\nuser = \"ghost\"\nport = 2222\n"))
	if err == nil {
		t.Fatal("a loopback profile carrying a host address was accepted")
	}
	msg := err.Error()
	for _, want := range []string{"host", "user", "port"} {
		if !strings.Contains(msg, want) {
			t.Errorf("the refusal does not name %q: %s", want, msg)
		}
	}
	// A caller branches on the sentinel, so it has to be in the chain.
	if !strings.Contains(msg, sshconfig.ErrInvalid.Error()) {
		t.Errorf("the refusal does not carry ErrInvalid, so it would answer 500: %s", msg)
	}
}

// TestAProfileWhoseShellIsMissingIsNotAConnectedSession pins the third finding of
// issue #154.
//
// A profile with default_shell = "nosuchshell_xyz" answered 200 with a handle, and
// `ls` a moment later showed the channel already `exited`. The caller was promised a
// pty and got a corpse, with the reason only visible by reading the channel. A 200
// means "you have a shell", and that is the one thing it must mean.
//
// The check is on the PROFILE's shell, not on the caller's command: `--argv echo hi`
// is a working one-shot whose process exits immediately, so a session that dies
// quickly is not by itself a failure. A profile is different — it names the shell
// every session on that host gets, so a missing program means every one of them is
// dead on arrival, and that is a fact about the config.
func TestAProfileWhoseShellIsMissingIsNotAConnectedSession(t *testing.T) {
	s := newTestServer(t)
	dir := t.TempDir()
	seedRawProfile(t, dir, "zz-badshell", "kind = \"internal\"\ndefault_shell = \"nosuchshell_xyz\"\n")
	s.sshConfigs = sshconfig.NewStore(dir)

	out := termText(t, s, map[string]any{"cmd": "connect zz-badshell"})
	if code := statusCode(t, out); code != 404 {
		t.Fatalf("connect answered %d for a shell that cannot run; want 404:\n%s", code, out)
	}
	if !strings.Contains(out, "nosuchshell_xyz") {
		t.Fatalf("the refusal does not name the program to fix:\n%s", out)
	}
}

// TestAOneShotCommandIsStillASession is the other half of that rule, and the
// reason it is not a "did it exit quickly" check.
//
// `connect internal --argv echo hi` is a working session whose process finishes in
// milliseconds. A guard built on watching for an early exit would report it as a
// failure — and it would be a guard that fails on the fastest machines first.
func TestAOneShotCommandIsStillASession(t *testing.T) {
	s := newTestServer(t)
	out := termText(t, s, map[string]any{"cmd": "connect internal --argv echo hi"})
	if code := statusCode(t, out); code != 200 {
		t.Fatalf("connect answered %d for a one-shot command; want 200:\n%s", code, out)
	}
}
