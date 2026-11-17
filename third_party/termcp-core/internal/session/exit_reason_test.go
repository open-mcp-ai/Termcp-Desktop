package session

import (
	"strings"
	"testing"
	"time"

	"github.com/open-mcp-ai/termcp/internal/message"
	"github.com/open-mcp-ai/termcp/internal/sshserver"
	"github.com/open-mcp-ai/termcp/internal/storage"
	"github.com/open-mcp-ai/termcp/pkg/api"
)

// WHY A SESSION DIED IS A FACT THAT MUST OUTLIVE THE PROCESS.
//
// Issue #176: a session whose transport died left nothing to diagnose it with. Every
// surface said it was gone, none said why, and the only record was a transcript sentence
// that read the same whether the network dropped, the far side closed, or the relay
// restarted — plus a Debug line below the daemon's default level. The caller asking `get`
// about a session that is no longer running is asking exactly this question, and after a
// restart it is the ONLY question they can ask, because the transcript may be gone with
// the directory.
//
// So the reason is recorded on the session, persisted with the manifest, and restored
// with it. This test kills the transport, waits for DEAD, and then reads the manifest
// back through a NEW manager — the same path a restarted daemon takes.
func TestWhyASessionDiedIsRecordedAndSurvivesARestart(t *testing.T) {
	store := storage.New(t.TempDir())
	// THE STORE OWNS OPEN FILE HANDLES, so it has to be closed before the temp
	// directory is removed. On Windows an open handle makes RemoveAll fail and the
	// failure is reported as the TEST's, not the cleanup's — the assertion that
	// matters had already passed.
	t.Cleanup(func() { _ = store.Close() })
	mm := message.NewManager(store)
	srv := sshserver.New()
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	m := NewManager(mm, store, srv)
	sess, err := m.Create(Config{Command: testShell(), Mode: api.ModePipe, Name: "brief"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := sess.ID

	// The transport IS the SSH connection, so stopping the server is what kills it.
	_ = srv.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && sess.Info().Status == api.SessionRunning {
		time.Sleep(50 * time.Millisecond)
	}
	info := sess.Info()
	if info.Status == api.SessionRunning {
		t.Fatalf("the session never noticed its transport was gone")
	}
	// ANTI-SPIN GUARD: the reason has to be there in the LIVE session first, or the
	// restart half below would pass on a manifest that never carried it.
	if info.ExitReason == "" {
		t.Fatalf("a session whose transport died recorded no reason (status %s)", info.Status)
	}
	if !strings.Contains(info.ExitReason, "SSH connection lost") {
		t.Fatalf("the recorded reason does not name the event: %q", info.ExitReason)
	}

	// THE RESTART. A fresh manager over the same directory restores the session as DEAD;
	// "why" is the one thing it cannot reconstruct from the bytes it has.
	//
	// WAIT FOR THE MANIFEST, NOT FOR THE STATUS. markDeadLocked sets the reason under
	// the session's lock and unlocks — which is what makes DEAD observable — and only
	// then calls the dead hook that persists. So "the status flipped" is not "the reason
	// is on disk", and a restart inside that window reads the PREVIOUS manifest. That
	// window is one function call wide in the product, but a test that restarts
	// immediately lands in it about once in ten CI runs: measured on ubuntu and macos,
	// live "❌ SSH connection lost: EOF" against restored "".
	//
	// Polling the disk is the honest assertion: it checks that the reason IS persisted
	// (a reason that is never written still fails, by timing out), without claiming an
	// order the product does not promise.
	wantReason := info.ExitReason
	writeDeadline := time.Now().Add(10 * time.Second)
	for {
		rec, err := store.ReadSession(id)
		if err == nil && rec != nil && rec.ExitReason == wantReason {
			break
		}
		if time.Now().After(writeDeadline) {
			onDisk := "(no manifest)"
			if rec, err := store.ReadSession(id); err == nil && rec != nil {
				onDisk = rec.ExitReason
			}
			// A FAILURE HAS TO SAY WHICH FAILURE. The first version of this message printed
			// the live reason twice (a typo put wantReason in both places), so a red run
			// could not be told apart from a read that returned the wrong session.
			t.Fatalf("the reason never reached the manifest: live %q, on disk %q", wantReason, onDisk)
		}
		time.Sleep(20 * time.Millisecond)
	}

	m2 := NewManager(message.NewManager(store), store, nil)
	if err := m2.RestoreDead(); err != nil {
		t.Fatalf("restore: %v", err)
	}
	restored := m2.Get(id)
	if restored == nil {
		t.Fatalf("the restored manager does not hold %s", id)
	}
	got := restored.Info()
	if got.Status != api.SessionExited {
		t.Fatalf("restored status = %q, want %q", got.Status, api.SessionExited)
	}
	if got.ExitReason != info.ExitReason {
		t.Fatalf("the reason did not survive the restart: live %q, restored %q",
			info.ExitReason, got.ExitReason)
	}
}

// A SESSION THE CALLER CLOSED SAYS SO, rather than leaving the field empty.
//
// "No reason recorded" and "the caller asked for this" are different states, and a
// caller reading `get` after a `close` needs the second: otherwise a deliberate close is
// indistinguishable from a session that never had its reason recorded at all.
func TestAClosedOnRequestSessionSaysSo(t *testing.T) {
	store := storage.New(t.TempDir())
	// THE STORE OWNS OPEN FILE HANDLES, so it has to be closed before the temp
	// directory is removed. On Windows an open handle makes RemoveAll fail and the
	// failure is reported as the TEST's, not the cleanup's — the assertion that
	// matters had already passed.
	t.Cleanup(func() { _ = store.Close() })
	mm := message.NewManager(store)
	srv := sshserver.New()
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	m := NewManager(mm, store, srv)
	sess, err := m.Create(Config{Command: testShell(), Mode: api.ModePipe, Name: "brief"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer m.Delete(sess.ID)

	m.Terminate(sess.ID, true, 0)
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && sess.Info().Status == api.SessionRunning {
		time.Sleep(20 * time.Millisecond)
	}
	if got := sess.Info(); got.ExitReason != exitReasonRequested {
		t.Fatalf("a deliberately closed session recorded %q, want %q", got.ExitReason, exitReasonRequested)
	}
}

// THE REGISTRY'S 404 CAN BE CHECKED AGAINST THE DISK.
//
// This is the fact `sessionNotFoundMessage` needs to tell a typo from an id that belongs
// to another instance: the manifest is there, the registry is not holding it.
func TestPersistedSessionFindsARecordTheRegistryDoesNotHold(t *testing.T) {
	store := storage.New(t.TempDir())
	// THE STORE OWNS OPEN FILE HANDLES, so it has to be closed before the temp
	// directory is removed. On Windows an open handle makes RemoveAll fail and the
	// failure is reported as the TEST's, not the cleanup's — the assertion that
	// matters had already passed.
	t.Cleanup(func() { _ = store.Close() })
	mm := message.NewManager(store)
	srv := sshserver.New()
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	m := NewManager(mm, store, srv)
	sess, err := m.Create(Config{Command: testShell(), Mode: api.ModePipe, Name: "brief"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	id := sess.ID
	_ = srv.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) && sess.Info().Status == api.SessionRunning {
		time.Sleep(50 * time.Millisecond)
	}

	// A second manager over the same directory: it holds nothing, and the record is
	// still on disk. That is the shape of the 404 in issue #176.
	other := NewManager(message.NewManager(store), store, nil)
	if other.Get(id) != nil {
		t.Fatalf("the fresh manager unexpectedly holds %s", id)
	}
	persisted := other.PersistedSession(id)
	if persisted == nil {
		t.Fatalf("the manifest on disk was not found for %s", id)
	}
	if persisted.ID != id {
		t.Fatalf("PersistedSession returned %q, want %q", persisted.ID, id)
	}
	// And an id that was never created is still nothing, so the 404 keeps its plain
	// shape for a typo.
	if other.PersistedSession("nosuchsession") != nil {
		t.Fatal("PersistedSession invented a record for an id that was never created")
	}
	// A path-shaped id must not walk out of the sessions directory.
	if other.PersistedSession("../sessions") != nil {
		t.Fatal("PersistedSession accepted a path-shaped id")
	}
}
