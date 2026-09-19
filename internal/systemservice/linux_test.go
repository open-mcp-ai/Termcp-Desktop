//go:build linux

package systemservice

import (
	"strings"
	"testing"
)

func TestRenderSystemdUnit(t *testing.T) {
	unit := renderSystemdUnit(`/opt/termcp gui/termcp-gui`)
	for _, expected := range []string{
		`ExecStart="/opt/termcp gui/termcp-gui" --core-service`,
		"Restart=on-failure",
		"Environment=TERMCP_GUI_SERVICE=1",
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
