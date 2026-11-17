package session

import (
	"log/slog"
	"testing"
	"time"

	"github.com/open-mcp-ai/termcp/internal/sshserver"
	"github.com/open-mcp-ai/termcp/pkg/api"
)

// THE REASON A SESSION DIED MUST REACH THE OPERATOR'S LOG.
//
// The line that recorded it was written at Debug, which is BELOW the daemon's default
// level (Info): on a normal run it was never written at all. The session is then gone
// from every surface a caller can see, and the only remaining record was a transcript
// sentence that said the same thing whatever had happened (issue #176). Four sessions
// were lost in one afternoon, and the error that would have answered why was in the
// line nobody could read.
//
// This pins the LEVEL, not the wording: the default handler drops anything below Info,
// so a regression to Debug fails here.
func TestATransportThatDiesIsLoggedAboveTheDefaultLevel(t *testing.T) {
	cap := &createLogCapture{}
	prev := slog.Default()
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// The INTERNAL SSH SERVER is the transport here: a session's transport is the SSH
	// connection, not the process on it, so stopping the server is what makes it die.
	srv := sshserver.New()
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = srv.Stop() })

	m := NewManager(nil, nil, srv)
	sess, err := m.Create(Config{Command: testShell(), Mode: api.ModePipe, Name: "brief"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer m.Delete(sess.ID)

	_ = srv.Stop()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if rec := cap.findQuietly(slog.LevelWarn, "session transport lost"); rec != nil {
			// THE ERROR IS THE REASON, and without it the line says only that something
			// ended. That is the whole value of the entry: four sessions were lost in one
			// afternoon and the cause had to be reconstructed from the outside.
			if _, ok := recordAttr(*rec, "err"); !ok {
				t.Fatal("the transport-loss line does not carry the error that caused it")
			}
			if _, ok := recordAttr(*rec, "session_id"); !ok {
				t.Fatal("the transport-loss line does not name the session")
			}
			return
		}
		if rec := cap.findQuietly(slog.LevelInfo, "session transport closed"); rec != nil {
			// A clean end is a different event and is logged at Info; either way it is
			// ABOVE the daemon's default level, which is what the Debug line failed.
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("a session whose transport ended logged nothing at Info or above")
}
