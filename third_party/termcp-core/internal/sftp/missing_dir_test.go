package sftp

import "testing"

// TestAMissingParentDirectoryIsNamedNotTheFile pins issue #147's decision point.
//
// Writing to a path whose directory does not exist answered
//
//	open remote file: file does not exist
//
// which, for a WRITE, is a loop: the file is the thing the caller is creating, and
// creating a new file in an existing directory works. The missing piece is the
// directory — so the directory is what has to be derived, and the derivation is
// where this can go wrong.
//
// It has to work for BOTH separators because the path is remote: `path.Dir` only
// knows "/", and a Windows target hands over `C:\Users\x\agent.conf`. The
// end-to-end message is asserted in internal/mcp, where a real session exists.
func TestAMissingParentDirectoryIsNamedNotTheFile(t *testing.T) {
	cases := []struct {
		path string
		dir  string
		ok   bool
	}{
		{"/tmp/nodir_zz/p2.txt", "/tmp/nodir_zz", true},
		{`C:\Users\x\nodir\p2.txt`, `C:\Users\x\nodir`, true},
		{`C:/Users/x/nodir/p2.txt`, "C:/Users/x/nodir", true},
		{"a/b.txt", "a", true},
		// A file directly under the root: the parent is the root, and keeping the
		// separator is what makes it absolute instead of "".
		{"/p2.txt", "/", true},
		// A bare name has no directory this can name: its parent is the server's
		// working directory, which is not in the string. Saying nothing is right;
		// inventing "." would name a directory the server may not use.
		{"p2.txt", "", false},
	}
	for _, c := range cases {
		dir, ok := parentOf(c.path)
		if ok != c.ok || dir != c.dir {
			t.Errorf("parentOf(%q) = (%q, %v), want (%q, %v)", c.path, dir, ok, c.dir, c.ok)
		}
	}
}
