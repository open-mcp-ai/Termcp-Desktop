package mcp

import (
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/sshconfig"
)

// TestEveryHostFlagReachesTheStoredConfig walks the flag list of `create host` and
// checks that each one lands in the config file.
//
// THIS IS THE BUG CLASS IT EXISTS FOR: a flag declared in the table, accepted by the
// parser, and never carried into the handler's arguments. `create host prod --user
// deploy` answered 200 with the user silently dropped, and nothing failed because the
// flag parsed and the call succeeded. The mapping between the two vocabularies
// (--jump-host -> jump.host, --default-shell -> default_shell) is hand-written, so it
// is exactly the kind of thing that rots.
//
// Omissions are deliberate and listed below: the password is never read back (it is a
// secret and the store may hash or keep it out of band), and the copy form speaks
// --source rather than describing the destination.
func TestEveryHostFlagReachesTheStoredConfig(t *testing.T) {
	dir := t.TempDir()
	store := sshconfig.NewStore(dir)
	s := New(nil, nil, store, nil, "test")
	s.ManageSSHConfigs = true

	cases := []struct {
		flag  string
		value string
		check func(t *testing.T, e *sshconfig.Entry)
	}{
		{"host", "192.0.2.10", func(t *testing.T, e *sshconfig.Entry) { eq(t, "host", e.Host, "192.0.2.10") }},
		{"user", "deploy", func(t *testing.T, e *sshconfig.Entry) { eq(t, "user", e.User, "deploy") }},
		{"port", "2222", func(t *testing.T, e *sshconfig.Entry) { eq(t, "port", e.Port, 2222) }},
		{"private-key", "/keys/id_ed25519", func(t *testing.T, e *sshconfig.Entry) { eq(t, "private_key", e.PrivateKey, "/keys/id_ed25519") }},
		{"key-passphrase", "pp", func(t *testing.T, e *sshconfig.Entry) { eq(t, "key_passphrase", e.KeyPassphrase, "pp") }},
		{"known-hosts", "/kh", func(t *testing.T, e *sshconfig.Entry) { eq(t, "known_hosts", e.KnownHosts, "/kh") }},
		{"proxy", "socks5://h:1080", func(t *testing.T, e *sshconfig.Entry) { eq(t, "proxy", e.Proxy, "socks5://h:1080") }},
		{"description", "prod box", func(t *testing.T, e *sshconfig.Entry) { eq(t, "description", e.Description, "prod box") }},
		{"default-shell", "/bin/bash", func(t *testing.T, e *sshconfig.Entry) { eq(t, "default_shell", e.DefaultShell, "/bin/bash") }},
		{"default-mode", "pipe", func(t *testing.T, e *sshconfig.Entry) { eq(t, "default_mode", e.DefaultMode, "pipe") }},
		{"dial-timeout-seconds", "45", func(t *testing.T, e *sshconfig.Entry) { eq(t, "dial_timeout_seconds", e.DialTimeoutSeconds, 45) }},
		// Each jump case repeats --jump-user because a bastion without a user is refused
		// by validation, and the point here is the flag reaching the file.
		{"jump-host", "bastion", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatalf("jump was dropped")
			}
			eq(t, "jump.host", e.Jump.Host, "bastion")
		}},
		{"jump-user", "ops", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatalf("jump was dropped")
			}
			eq(t, "jump.user", e.Jump.User, "ops")
		}},
		{"jump-port", "2200", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatalf("jump was dropped")
			}
			eq(t, "jump.port", e.Jump.Port, 2200)
		}},
		// The bastion's own credential: it must land on the JUMP spec, not on the entry,
		// or the bastion is dialled with the target's secret.
		{"jump-private-key", "/keys/jump_id", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatalf("jump was dropped")
			}
			eq(t, "jump.private_key", e.Jump.PrivateKey, "/keys/jump_id")
			if e.PrivateKey != "" {
				t.Errorf("the bastion's key leaked onto the entry: %q", e.PrivateKey)
			}
		}},
		{"jump-key-passphrase", "jpp", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatalf("jump was dropped")
			}
			eq(t, "jump.key_passphrase", e.Jump.KeyPassphrase, "jpp")
		}},
		{"trust-unknown-host", "", func(t *testing.T, e *sshconfig.Entry) {
			if e.TrustUnknownHost == nil || !*e.TrustUnknownHost {
				t.Fatalf("trust_unknown_host was not stored as true")
			}
		}},
		{"default-approval", "", func(t *testing.T, e *sshconfig.Entry) {
			if !e.DefaultApproval {
				t.Fatalf("default_approval was not stored as true")
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			name := "host-" + strings.ReplaceAll(tc.flag, "-", "_")
			// A remote entry must carry a credential, so every case starts from a
			// complete one and adds the flag under test. Without this the refusals are
			// about validation, not about the flag being carried.
			cmd := "create host " + name + " --host 192.0.2.1 --user u --password p"
			if strings.HasPrefix(tc.flag, "jump-") {
				// A bastion must carry a host, a user and a credential of its own; until
				// this test existed `create host` had no flag for the credential, so
				// every jump it could express was refused by validation and the three
				// jump flags were reachable only in theory.
				if tc.flag != "jump-host" {
					cmd += " --jump-host bastion"
				}
				cmd += " --jump-user j --jump-password jp"
			}
			switch {
			case tc.value == "":
				cmd += " --" + tc.flag
			case strings.ContainsAny(tc.value, " "):
				// A value with a space must be quoted, or the parser splits it and the
				// test measures the quoting rules instead of the flag mapping.
				cmd += " --" + tc.flag + " '" + tc.value + "'"
			default:
				cmd += " --" + tc.flag + " " + tc.value
			}
			if got := termText(t, s, map[string]any{"cmd": cmd}); strings.HasPrefix(got, "TERM/1 4") {
				t.Fatalf("%q was refused:\n%s", cmd, got)
			}
			e, err := store.Load(name)
			if err != nil {
				t.Fatalf("%s was not stored: %v", name, err)
			}
			tc.check(t, e)
		})
	}
}

func eq[T comparable](t *testing.T, what string, got, want T) {
	t.Helper()
	if got != want {
		t.Errorf("%s = %v, want %v", what, got, want)
	}
}
