//go:build !windows

package storage

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// TestRenamingOverAnOpenFileIsImmediateOnPosix pins the platform half of the
// manifest-publish fix.
//
// The retry in renameOver exists for Windows, where a rename over a file another
// process holds open is refused. A POSIX rename has no such rule, so the loop is
// configured to try once and there must be no waiting at all: a retry budget
// leaking onto Linux and macOS would silently add latency to every manifest write
// on the platforms that never needed it (and the project's CI runs all three).
//
// This is asserted by behaviour rather than by reading the constant, so a future
// change that made the predicate or the loop platform-unaware fails here instead
// of shipping the delay.
func TestRenamingOverAnOpenFileIsImmediateOnPosix(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "manifest.json")
	if err := os.WriteFile(target, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	tmp := filepath.Join(dir, ".tmp-new")
	if err := os.WriteFile(tmp, []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Hold the target open: on POSIX this must not block the rename.
	held, err := os.Open(target)
	if err != nil {
		t.Fatal(err)
	}
	defer held.Close()

	if err := renameOver(tmp, target); err != nil {
		t.Fatalf("renaming over an open file must succeed on POSIX: %v", err)
	}
	// NO WALL-CLOCK ASSERTION HERE, and the reason is not that it was flaky — it is
	// that the number could never have tested the thing it claimed.
	//
	// "The retry budget must not be spent on POSIX" reads as a timing property, but
	// renameOver only sleeps after IsSharingViolation says yes, and on POSIX nothing
	// is a sharing violation (pinned by TestNoRenameErrorIsRetriedOnPosix). With
	// renameAttempts=1 and renameBackoff=0 the loop cannot spend a budget even if it
	// wanted to, so a fast rename proves nothing and a slow one disproves nothing:
	// the 5ms budget this replaced was measuring the scheduler, and on a loaded
	// ubuntu runner it reported "took 12.69ms" for a rename that never retried.
	//
	// What is left is the behaviour that is real: the rename went through while the
	// file was held open, and the target holds the new bytes. The predicate side —
	// which is the part that would regress — is asserted next door.
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("target holds %q, want the new contents", got)
	}
}

// TestNoRenameErrorIsRetriedOnPosix pins the predicate itself: on POSIX nothing
// is a sharing violation, so a genuine failure comes straight back instead of
// being retried (which would delay it and then fail anyway).
func TestNoRenameErrorIsRetriedOnPosix(t *testing.T) {
	for _, err := range []error{
		os.ErrNotExist,
		os.ErrPermission,
		errors.New("disk full"),
	} {
		if isSharingViolation(err) {
			t.Errorf("isSharingViolation(%v) = true; only Windows refuses a rename because the target is open", err)
		}
	}
}
