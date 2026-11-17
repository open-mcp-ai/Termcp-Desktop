package mcp

import (
	"os"
	"testing"
	"time"
)

// TestMain shortens the one budget the suite only has to OUTLAST.
//
// shellReadyBudget is a wait whose result its caller ignores: `open` hands over the
// handle whether or not the shell spoke. So nothing here asserts on the real three
// seconds, and paying them once per open that never speaks was most of this package's
// runtime — 240s of a 240s timeout, measured, which is why a slow shuffle order could
// push the package over the wall and fail a run that had no defect in it.
//
// The budget stays at its shipped value in production. Only the tests move it, and they
// move it here, once, rather than in the tests that happen to be slow today.
func TestMain(m *testing.M) {
	shellReadyBudget = 300 * time.Millisecond
	os.Exit(m.Run())
}
