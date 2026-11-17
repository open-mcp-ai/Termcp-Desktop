package mcp

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/internal/sshconfig"
)

// TestAQuotedShellPathIsReadAsOneWord pins the fix for a hand-rolled word split.
//
// `default_shell` is a shell line — a program plus arguments — so the program name is a
// shell WORD. The check that a profile's shell exists split that line on whitespace, which
// reads `"C:\Program Files\pwsh\pwsh.exe" -NoLogo` as `"C:\Program` and reports the profile
// as broken when the only thing wrong was how the daemon read it. A path with a space is
// not an exotic case on Windows; it is where pwsh is installed by default.
func TestAQuotedShellPathIsReadAsOneWord(t *testing.T) {
	// A real executable with a SPACE in its path, so the assertion is about a shell that
	// exists rather than about a spelling. On Windows this is the default install location
	// for pwsh, and copying it is how the test gets one without assuming where it is.
	dir := t.TempDir()
	spaced := filepath.Join(dir, "Program Files", "shell with spaces")
	if err := os.MkdirAll(spaced, 0o755); err != nil {
		t.Fatal(err)
	}
	name := "sh"
	if runtime.GOOS == "windows" {
		name = "shell with spaces.exe"
	}
	src, err := os.Executable()
	if err != nil {
		t.Skipf("cannot find the test binary to copy: %v", err)
	}
	dest := filepath.Join(spaced, name)
	body, err := os.ReadFile(src)
	if err != nil {
		t.Skipf("cannot read the test binary: %v", err)
	}
	if err := os.WriteFile(dest, body, 0o755); err != nil {
		t.Skipf("cannot write a copy into a spaced path: %v", err)
	}

	// Quoted, with an argument after it: the shape a Windows profile actually has.
	line := `"` + dest + `" -NoLogo`
	if err := profileShellExists(&sshconfig.Entry{DefaultShell: line}); err != nil {
		t.Fatalf("a shell at %q was reported as not runnable:\n%v", dest, err)
	}

	// The narrowing that must survive: a shell that really is missing is still reported,
	// and the message names the program rather than a fragment of the line.
	missing := `"` + filepath.Join(dir, "no such dir", "nosuchshell") + `" -NoLogo`
	err = profileShellExists(&sshconfig.Entry{DefaultShell: missing})
	if err == nil {
		t.Fatal("a missing shell was accepted")
	}
	if !strings.Contains(err.Error(), "nosuchshell") {
		t.Errorf("the message does not name the program that is missing:\n%v", err)
	}
	// The whole path, not a fragment: a message that stopped at the first space would
	// send the caller looking for a program called `C:\Program`.
	if !strings.Contains(err.Error(), "no such dir") {
		t.Errorf("the message lost the part of the path after a space:\n%v", err)
	}
	if strings.HasSuffix(err.Error(), `"`) {
		t.Errorf("the message quotes a fragment of the line rather than the program:\n%v", err)
	}

	// A bare name still resolves through PATH, which is what every profile without a path
	// relies on.
	if err := profileShellExists(&sshconfig.Entry{DefaultShell: "nosuchshell_xyz"}); err == nil {
		t.Error("a name that is not on PATH was accepted")
	}
}
