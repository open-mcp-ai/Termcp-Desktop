package sftp

import (
	"runtime"
	"testing"
)

// TestNativeLocalPathPreservesPOSIXAndRewritesWindowsGitBash pins that Git Bash
// spellings (/d/home/x) become what os.Open accepts (D:\home\x) on Windows only.
func TestNativeLocalPathPreservesPOSIXAndRewritesWindowsGitBash(t *testing.T) {
	if runtime.GOOS != "windows" {
		if got := NativeLocalPath("/d/home/test"); got != "/d/home/test" {
			t.Errorf("on non-Windows paths must not be rewritten: %q", got)
		}
		return
	}
	for _, tc := range []struct{ in, want string }{
		{"/c/Users/test", "C:\\Users\\test"},
		{"/d/work/repo", "D:\\work\\repo"},
		{"/D/work/repo", "D:\\work\\repo"},
		{"D:\\native\\path", "D:\\native\\path"},
		{"relative/path", "relative/path"},
		{"", ""},
		{"/", "/"},
		{"/etc/passwd", "/etc/passwd"}, // not a drive letter: single char only
	} {
		if got := NativeLocalPath(tc.in); got != tc.want {
			t.Errorf("NativeLocalPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
