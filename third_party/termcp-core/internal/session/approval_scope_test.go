package session

import (
	"testing"
	"time"

	"github.com/open-mcp-ai/termcp/internal/approval"
	"github.com/open-mcp-ai/termcp/pkg/api"
)

// apiSession builds the embedded metadata these states hang off.
func apiSession(id string) api.Session { return api.Session{ID: id} }

// Review mode used to be one switch that held everything (issue #75). A session
// can now name the kinds it holds, and the whole risk of that is the direction
// nobody tests: an operation the operator did NOT scope must go straight through.
// A per-kind check that is really "is review mode on" would hold it anyway, which
// looks identical in the UI (the gate reads as on) and only shows up as an agent
// that cannot open the tunnel it was allowed to open.
func TestApprovalScopesHoldOnlyTheNamedKinds(t *testing.T) {
	newSess := func(t *testing.T) *Session {
		t.Helper()
		return &Session{Session: apiSession("scope-test")}
	}

	t.Run("no scopes holds everything", func(t *testing.T) {
		s := newSess(t)
		if err := s.EnableApproval(1, 0); err != nil {
			t.Fatal(err)
		}
		for _, k := range approval.AllKinds {
			if !s.GatesKind(k) {
				t.Errorf("the default must hold %q", k)
			}
		}
		if got := s.ApprovalScopes(); got != nil {
			t.Errorf("the default reports as nil (\"all\"), got %v", got)
		}
	})

	t.Run("a named subset holds only that", func(t *testing.T) {
		s := newSess(t)
		if err := s.EnableApprovalScoped(1, 0, []approval.Kind{approval.KindFileWrite, approval.KindFileDelete}); err != nil {
			t.Fatal(err)
		}
		if !s.GatesKind(approval.KindFileWrite) || !s.GatesKind(approval.KindFileDelete) {
			t.Error("the named kinds must be held")
		}
		// The direction that matters: everything else must pass.
		for _, k := range []approval.Kind{
			approval.KindShellInput, approval.KindForwardOpen, approval.KindForwardClose,
			approval.KindFilePerm, approval.KindFileMkdir,
		} {
			if s.GatesKind(k) {
				t.Errorf("%q was not in the scope list but is still held", k)
			}
		}
		// And the narrowed set is reported, in a stable order, so the UI can show
		// what is covered instead of a bare on/off.
		got := s.ApprovalScopes()
		if len(got) != 2 {
			t.Fatalf("scopes = %v, want the two named kinds", got)
		}
	})

	t.Run("an unknown scope is refused, not dropped", func(t *testing.T) {
		s := newSess(t)
		// A typo that silently did nothing would leave the operator believing an
		// operation is gated when it is not — the one failure a gate must not have.
		if err := s.EnableApprovalScoped(1, 0, []approval.Kind{"file_writes"}); err == nil {
			t.Fatal("a misspelled scope must be an error")
		}
		if s.ApprovalEnabled() {
			t.Error("a refused configuration must not leave the session in review mode")
		}
	})

	t.Run("off holds nothing", func(t *testing.T) {
		s := newSess(t)
		if err := s.EnableApproval(1, 0); err != nil {
			t.Fatal(err)
		}
		s.DisableApproval()
		for _, k := range approval.AllKinds {
			if s.GatesKind(k) {
				t.Errorf("review is off but %q is still held", k)
			}
		}
	})

	t.Run("a shell asks about its own kind", func(t *testing.T) {
		s := newSess(t)
		// Scoped to file operations, so typing is not reviewable: a session whose
		// policy never mentioned commands must not hold them.
		if err := s.EnableApprovalScoped(1, 0, []approval.Kind{approval.KindFileWrite}); err != nil {
			t.Fatal(err)
		}
		shell := &ChildShell{}
		shell.parent.Store(s)
		if shell.RequiresApproval() {
			t.Error("a session scoped to file writes must let shell input through")
		}
	})

	t.Run("scoping to shell input alone holds typing but not files", func(t *testing.T) {
		s := newSess(t)
		if err := s.EnableApprovalScoped(1, 0, []approval.Kind{approval.KindShellInput}); err != nil {
			t.Fatal(err)
		}
		shell := &ChildShell{}
		shell.parent.Store(s)
		if !shell.RequiresApproval() {
			t.Error("a session scoped to shell input must hold typing")
		}
		if s.GatesKind(approval.KindForwardOpen) {
			t.Error("it must not hold a port forward")
		}
	})
}

// The narrowed set has to survive into what every reader sees: the manifest the
// session list renders, and the REST payload the UI draws its lock from. A scope
// the UI cannot see is a gate the operator cannot trust.
func TestApprovalScopesReachTheSessionSnapshot(t *testing.T) {
	s := &Session{Session: apiSession("snapshot-test")}
	if err := s.EnableApprovalScoped(1, 0, []approval.Kind{approval.KindForwardOpen}); err != nil {
		t.Fatal(err)
	}
	info := s.Info()
	if !info.ApprovalMode {
		t.Fatal("the session is in review mode")
	}
	if len(info.ApprovalScopes) != 1 || info.ApprovalScopes[0] != string(approval.KindForwardOpen) {
		t.Fatalf("approval_scopes = %v, want [forward_open]", info.ApprovalScopes)
	}

	// The default reports no list, so an older reader sees the manifest it always
	// did and "empty means everything" stays true.
	all := &Session{Session: apiSession("snapshot-all")}
	if err := all.EnableApproval(1, time.Minute); err != nil {
		t.Fatal(err)
	}
	if got := all.Info().ApprovalScopes; got != nil {
		t.Errorf("the default must not carry a scope list, got %v", got)
	}
}
