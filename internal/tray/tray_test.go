package tray

import (
	"errors"
	"strings"
	"testing"

	"github.com/open-mcp-ai/termcp/gui/internal/config"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
)

func TestFormatStateRunningService(t *testing.T) {
	state := formatState(
		model.CoreStatus{Running: true, Address: "http://127.0.0.1:18765"},
		config.Status{Supported: true, Installed: true, Running: true, Autostart: true},
		nil,
		"zh-CN",
	)
	if !state.CoreRunning || !state.ServiceInstalled || !state.ServiceAutostart {
		t.Fatalf("unexpected state flags: %+v", state)
	}
	for _, expected := range []string{"运行中", "127.0.0.1:18765"} {
		if !strings.Contains(state.CoreLine, expected) {
			t.Fatalf("CoreLine %q does not contain %q", state.CoreLine, expected)
		}
	}
	for _, expected := range []string{"已注册", "运行中", "开机自启"} {
		if !strings.Contains(state.ServiceLine, expected) {
			t.Fatalf("ServiceLine %q does not contain %q", state.ServiceLine, expected)
		}
	}
}

func TestFormatStateErrorsAreSafeLabels(t *testing.T) {
	state := formatState(
		model.CoreStatus{State: "error", Error: "端口被占用"},
		config.Status{},
		errors.New("status unavailable"),
		"zh-CN",
	)
	if !strings.Contains(state.CoreLine, "端口被占用") {
		t.Fatalf("unexpected CoreLine: %q", state.CoreLine)
	}
	if state.ServiceLine != "系统服务：状态读取失败" {
		t.Fatalf("unexpected ServiceLine: %q", state.ServiceLine)
	}
}

func TestFormatStateEnglish(t *testing.T) {
	state := formatState(
		model.CoreStatus{Running: true, Address: "http://127.0.0.1:18765"},
		config.Status{Supported: true, Installed: true, Running: false, Autostart: false},
		nil,
		"en",
	)
	for _, expected := range []string{"Core: Running", "System service: Registered", "Stopped", "Autostart off"} {
		if !strings.Contains(state.CoreLine+state.ServiceLine, expected) {
			t.Fatalf("tray labels %q / %q do not contain %q", state.CoreLine, state.ServiceLine, expected)
		}
	}
	if state.ShowWindowLabel != "Show Termcp" || state.QuitLabel != "Quit Termcp" {
		t.Fatalf("unexpected action labels: %+v", state)
	}
}
