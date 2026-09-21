//go:build linux

package config

import (
	"strings"
	"testing"
)

func TestRenderSystemdUnit(t *testing.T) {
	unit := renderSystemdUnit(`/opt/Termcp-Desktop/Termcp`, `/home/example/.termcp`)
	for _, expected := range []string{
		`ExecStart="/opt/Termcp-Desktop/Termcp" --core-service --core-data-dir "/home/example/.termcp"`,
		"Restart=on-failure",
		"Environment=TERMCP_DESKTOP_SERVICE=1",
		"WantedBy=default.target",
	} {
		if !strings.Contains(unit, expected) {
			t.Fatalf("unit does not contain %q:\n%s", expected, unit)
		}
	}
}

func TestSystemdQuoteEscapesSpecialCharacters(t *testing.T) {
	if got := systemdQuote(`/opt/a "quoted"/termcp`); got != `"/opt/a \"quoted\"/termcp"` {
		t.Fatalf("quoted path = %q", got)
	}
}
