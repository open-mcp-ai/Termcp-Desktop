package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// TestEveryFileOpIsReachable drives every op the SFTP surface accepts.
//
// The op vocabulary is two-level — an action group (perm, link, fs) hides behind a
// single name — and fileGroupForOp is the only place that mapping lives. It has been
// wrong twice: `--op urls` fell through to "does not know" because urls was missing
// from the table, and `--op stat` reached the fs group and was told the choices were
// truncate/realpath/statvfs. Both were invisible to a suite that never said the words.
//
// Every op is listed here by its USER-FACING spelling, with a file that suits it, so a
// new op cannot be added to one place and forgotten in the other.
func TestEveryFileOpIsReachable(t *testing.T) {
	// NOT SKIPPED AS A BLOCK. Most of these ops are portable — reading a stat, a
	// realpath, a truncate, a download — and skipping the whole set on Windows is how
	// `ReplaceText`'s remove-then-rename fallback went untested on the one platform that
	// needs it. Each case declares what it needs; the rest run everywhere.
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	sid := strings.SplitN(h, ":", 2)[0]
	dir := t.TempDir()

	// One file per op, so an op that mutates cannot spoil the next one.
	file := func(name, body string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(body), 0644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	cases := []struct {
		op    string
		posix bool // needs POSIX semantics: modes, ownership, symlink targets
		cmd   func(p, dir string) string
	}{
		{"stat", false, func(p, _ string) string { return "get " + sid + " file " + p + " --op stat" }},
		{"getwd", false, func(_, _ string) string { return "get " + sid + " file . --op getwd" }},
		{"urls", false, func(p, _ string) string { return "get " + sid + " file " + p + " --op urls" }},
		{"realpath", false, func(p, _ string) string { return "get " + sid + " file " + p + " --op realpath" }},
		{"statvfs", false, func(_, d string) string { return "get " + sid + " file " + d + " --op statvfs" }},
		{"readlink", true, func(_, d string) string { return "get " + sid + " file " + filepath.Join(d, "ln") + " --op readlink" }},
		{"chmod", true, func(p, _ string) string { return "write " + sid + " file " + p + " --op chmod --mode 416" }},
		{"chown", true, func(p, _ string) string { return "write " + sid + " file " + p + " --op chown --uid 0 --gid 0" }},
		{"chtimes", true, func(p, _ string) string {
			return "write " + sid + " file " + p + " --op chtimes --atime 1600000000 --mtime 1600000001"
		}},
		{"truncate", false, func(p, _ string) string { return "write " + sid + " file " + p + " --op truncate --size 2" }},
		// The two links are CREATE operations, not write ops: the node's key is the new
		// name and --target is what it refers to. Listing them under write was what sent
		// a caller to a verb that could not take --target.
		{"symlink", false, func(_, d string) string {
			return "create " + sid + " file " + filepath.Join(d, "sym-new") + " --target " + filepath.Join(d, "target.txt")
		}},
		{"link", false, func(_, d string) string {
			return "create " + sid + " file " + filepath.Join(d, "hard-new") + " --op link --target " + filepath.Join(d, "target.txt")
		}},
	}

	// A target and a symlink for the ops that need one to already exist.
	file("target.txt", "body")
	if err := os.Symlink(filepath.Join(dir, "target.txt"), filepath.Join(dir, "ln")); err != nil {
		t.Fatal(err)
	}

	for _, tc := range cases {
		t.Run(tc.op, func(t *testing.T) {
			if tc.posix && runtime.GOOS == "windows" {
				t.Skip("needs POSIX modes/ownership/symlinks")
			}
			p := file("op-"+tc.op+".txt", "0123456789")
			cmd := tc.cmd(p, dir)
			got := termText(t, s, map[string]any{"cmd": cmd})
			// A 4xx here means the op never reached the layer that does the work. Some
			// ops legitimately fail afterwards (an unprivileged chown), and that is a
			// 4xx too — so the tell is the code: unknown-op and bad-argument are the two
			// that mean "this spelling does not exist".
			for _, dead := range []string{"does not know", "unknown flag", "must be one of"} {
				if strings.Contains(got, dead) {
					t.Fatalf("%q is not a spelling the server accepts:\n%s", cmd, got)
				}
			}
		})
	}
}

// TestCreateFileMakesASymlinkForTarget pins `create … file --target`, where the KEY is
// the link and --target is what it points at.
//
// The two are one transposition away from each other and both orders "work": reversed,
// the call answers 200 and creates a link in the wrong place, pointing the wrong way.
func TestCreateFileMakesASymlinkForTarget(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	sid := strings.SplitN(h, ":", 2)[0]
	dir := t.TempDir()

	target := filepath.Join(dir, "real.txt")
	if err := os.WriteFile(target, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	linkPath := filepath.Join(dir, "alias.txt")

	got := termText(t, s, map[string]any{"cmd": "create " + sid + " file " + linkPath + " --target " + target})
	if !strings.HasPrefix(got, "TERM/1 2") {
		if runtime.GOOS == "windows" {
			// A Windows runner may lack the privilege to create a symlink. That is a
			// property of the runner, not of the directive — but the attempt has to be
			// made, or the branch is never exercised on the platform whose paths differ.
			t.Skipf("this runner cannot create symlinks: %s", got)
		}
		t.Fatalf("create … file --target was refused:\n%s", got)
	}

	// The link exists at the KEY and resolves to --target. Checking only that "a link
	// exists" is what a transposition would pass.
	fi, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("the link was not created at the key:\n%s", got)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("%s is not a symlink", linkPath)
	}
	if resolved, err := os.Readlink(linkPath); err != nil || resolved != target {
		t.Errorf("the link points at %q, want %q", resolved, target)
	}
}

// TestCreateFileMakesAHardLink pins `create … file --op link`, which had no spelling
// before: the handler wanted existing_path and new_path, neither of which was a flag of
// write, so the only hard link the surface could express was an error message.
func TestCreateFileMakesAHardLink(t *testing.T) {
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})
	sid := strings.SplitN(h, ":", 2)[0]
	dir := t.TempDir()

	existing := filepath.Join(dir, "original.txt")
	if err := os.WriteFile(existing, []byte("shared"), 0644); err != nil {
		t.Fatal(err)
	}
	newPath := filepath.Join(dir, "second-name.txt")

	got := termText(t, s, map[string]any{"cmd": "create " + sid + " file " + newPath + " --op link --target " + existing})
	if !strings.HasPrefix(got, "TERM/1 2") {
		if runtime.GOOS == "windows" {
			t.Skipf("this runner cannot create hard links: %s", got)
		}
		t.Fatalf("create … file --op link was refused:\n%s", got)
	}

	a, err := os.Stat(existing)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.Stat(newPath)
	if err != nil {
		t.Fatalf("the hard link was not created at the key:\n%s", got)
	}
	// A hard link shares the inode; a COPY would look identical by content, so the
	// inode number is the only honest witness.
	if !os.SameFile(a, b) {
		t.Errorf("the two names are not the same file on disk")
	}
}
