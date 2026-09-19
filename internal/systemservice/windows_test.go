//go:build windows

package systemservice

import "testing"

func TestWindowsCommandLine(t *testing.T) {
	got := windowsCommandLine(`C:\Program Files\termcp-gui\termcp-gui.exe`, `C:\Users\Example User\.termcp`)
	want := `"C:\Program Files\termcp-gui\termcp-gui.exe" --core-service --core-data-dir "C:\Users\Example User\.termcp"`
	if got != want {
		t.Fatalf("command line = %q, want %q", got, want)
	}
}
