package mcp

import (
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/sshconfig"
)

// TestEveryHostFlagLandsWhenEDITED walks the same flag list as the create test, but
// through `edit host`.
//
// A suite-wide count of flags actually exercised per directive found `edit` at 3 of
// its 32 declared flags: every `edit` in the suite exited on the write gate or on a
// stale flag, so the verb's real job was never measured. `create` and `edit` share the
// flag table and the mapping to stored fields, but they do NOT share the code that
// applies them — `edit` merges onto an existing entry, and a merge that drops a field
// is invisible from the reply, which says 200 success.
//
// The baseline deliberately carries a DIFFERENT value than the one being edited in, so
// "it landed" cannot be satisfied by what was already stored.
func TestEveryHostFlagLandsWhenEDITED(t *testing.T) {
	dir := t.TempDir()
	store := sshconfig.NewStore(dir)
	s := New(nil, nil, store, nil, "test")
	s.ManageSSHConfigs = true

	cases := []struct {
		flag  string
		value string
		check func(t *testing.T, e *sshconfig.Entry)
	}{
		{"host", "192.0.2.77", func(t *testing.T, e *sshconfig.Entry) { eq(t, "host", e.Host, "192.0.2.77") }},
		{"user", "deploy", func(t *testing.T, e *sshconfig.Entry) { eq(t, "user", e.User, "deploy") }},
		{"port", "2200", func(t *testing.T, e *sshconfig.Entry) { eq(t, "port", e.Port, 2200) }},
		{"private-key", "/keys/id_ed", func(t *testing.T, e *sshconfig.Entry) { eq(t, "private_key", e.PrivateKey, "/keys/id_ed") }},
		{"key-passphrase", "pp2", func(t *testing.T, e *sshconfig.Entry) { eq(t, "key_passphrase", e.KeyPassphrase, "pp2") }},
		{"known-hosts", "/kh2", func(t *testing.T, e *sshconfig.Entry) { eq(t, "known_hosts", e.KnownHosts, "/kh2") }},
		{"proxy", "socks5://h:1081", func(t *testing.T, e *sshconfig.Entry) { eq(t, "proxy", e.Proxy, "socks5://h:1081") }},
		{"dial-timeout-seconds", "60", func(t *testing.T, e *sshconfig.Entry) { eq(t, "dial_timeout", e.DialTimeoutSeconds, 60) }},
		{"default-shell", "/bin/zsh", func(t *testing.T, e *sshconfig.Entry) { eq(t, "default_shell", e.DefaultShell, "/bin/zsh") }},
		{"default-mode", "pipe", func(t *testing.T, e *sshconfig.Entry) { eq(t, "default_mode", e.DefaultMode, "pipe") }},
		{"description", "edited-by-test", func(t *testing.T, e *sshconfig.Entry) { eq(t, "description", e.Description, "edited-by-test") }},
		{"jump-host", "bastion2", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatal("jump was dropped")
			}
			eq(t, "jump.host", e.Jump.Host, "bastion2")
		}},
		{"jump-port", "2201", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatal("jump was dropped")
			}
			eq(t, "jump.port", e.Jump.Port, 2201)
		}},
		{"jump-user", "j2", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatal("jump was dropped")
			}
			eq(t, "jump.user", e.Jump.User, "j2")
		}},
		// The bastion's own credential: it must land on the JUMP spec, not on the entry,
		// or the bastion is dialled with the target's secret.
		{"jump-private-key", "/keys/jump2", func(t *testing.T, e *sshconfig.Entry) {
			if e.Jump == nil {
				t.Fatal("jump was dropped")
			}
			eq(t, "jump.private_key", e.Jump.PrivateKey, "/keys/jump2")
			if e.PrivateKey != "" {
				t.Errorf("the bastion's key leaked onto the entry: %q", e.PrivateKey)
			}
		}},
		{"trust-unknown-host", "", func(t *testing.T, e *sshconfig.Entry) {
			if e.TrustUnknownHost == nil || !*e.TrustUnknownHost {
				t.Fatal("trust_unknown_host was not stored as true")
			}
		}},
		{"default-approval", "", func(t *testing.T, e *sshconfig.Entry) {
			if !e.DefaultApproval {
				t.Fatal("default_approval was not stored as true")
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.flag, func(t *testing.T) {
			name := "edited-" + strings.ReplaceAll(tc.flag, "-", "_")
			base := "create host " + name + " --host 192.0.2.1 --user u --password p"
			if strings.HasPrefix(tc.flag, "jump-") {
				base += " --jump-host bastion --jump-user j --jump-password jp"
			}
			if got := termText(t, s, map[string]any{"cmd": base}); strings.HasPrefix(got, "TERM/1 4") {
				t.Fatalf("baseline %q was refused:\n%s", base, got)
			}

			cmd := "edit host " + name
			switch {
			case tc.value == "":
				cmd += " --" + tc.flag
			case strings.ContainsAny(tc.value, " "):
				cmd += " --" + tc.flag + " '" + tc.value + "'"
			default:
				cmd += " --" + tc.flag + " " + tc.value
			}
			if got := termText(t, s, map[string]any{"cmd": cmd}); strings.HasPrefix(got, "TERM/1 4") {
				t.Fatalf("%q was refused:\n%s", cmd, got)
			}

			e, err := store.Load(name)
			if err != nil {
				t.Fatalf("%s disappeared: %v", name, err)
			}
			tc.check(t, e)
			// The entry keeps what the edit did not mention: a merge that replaced the
			// whole record would pass every check above and lose everything else.
			if e.Host == "" {
				t.Error("the edit replaced the record instead of merging onto it")
			}
			if tc.flag != "host" && e.Host != "192.0.2.1" {
				t.Errorf("the edit changed the host it was not asked to change: %q", e.Host)
			}
		})
	}
}
