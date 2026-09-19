//go:build darwin

package systemservice

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderPlistProducesValidLaunchAgent(t *testing.T) {
	data, err := renderPlist(`/Applications/termcp gui.app/Contents/MacOS/termcp-gui`, `/tmp/termcp & core.log`, true)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, expected := range []string{Label, "--core-service", "<true/>", "/Applications/termcp gui.app", "/tmp/termcp &amp; core.log"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("plist does not contain %q:\n%s", expected, text)
		}
	}
	path := filepath.Join(t.TempDir(), "core.plist")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("plutil", "-lint", path).CombinedOutput(); err != nil {
		t.Fatalf("plutil rejected generated plist: %v: %s", err, output)
	}
}

func TestPlistStatusParsers(t *testing.T) {
	if !plistBool([]byte(`<key>RunAtLoad</key><true/><key>KeepAlive</key><false/>`), "RunAtLoad") {
		t.Fatal("RunAtLoad true was not detected")
	}
	if plistBool([]byte(`<key>RunAtLoad</key><false/>`), "RunAtLoad") {
		t.Fatal("RunAtLoad false was detected as true")
	}
	if pid := launchdPID("state = running\n\tpid = 4812\n"); pid != 4812 {
		t.Fatalf("pid = %d", pid)
	}
}
