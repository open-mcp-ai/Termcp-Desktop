package config

import (
	"log/slog"
	"time"

	"github.com/open-mcp-ai/termcp/gui/internal/logging"
)

const (
	Label              = "ai.openmcp.termcp.desktop.core"
	WindowsServiceName = "termcp-desktop-core"
)

type Status struct {
	Supported   bool   `json:"supported"`
	Platform    string `json:"platform"`
	Installed   bool   `json:"installed"`
	Running     bool   `json:"running"`
	Autostart   bool   `json:"autostart"`
	PID         int    `json:"pid,omitempty"`
	Label       string `json:"label"`
	Definition  string `json:"definition,omitempty"`
	LogPath     string `json:"log_path,omitempty"`
	Executable  string `json:"executable,omitempty"`
	Description string `json:"description,omitempty"`
}

type Manager interface {
	Status() (Status, error)
	Install(autostart bool) error
	Uninstall() error
	Start() error
	Stop() error
	Restart() error
	SetAutostart(enabled bool) error
}

func NewManager(executable string) Manager {
	return &loggedManager{inner: newPlatformManager(executable)}
}

type loggedManager struct{ inner Manager }

func beginServiceOperation(operation string, attributes ...any) func(error) {
	started := time.Now()
	fields := append([]any{"operation", operation}, attributes...)
	slog.Debug("system service operation started", fields...)
	return func(err error) {
		fields := append(fields, "duration_ms", time.Since(started).Milliseconds())
		if err != nil {
			slog.Error("system service operation failed", append(fields, "error", logging.ErrorText(err))...)
			return
		}
		slog.Debug("system service operation completed", fields...)
	}
}

func (manager *loggedManager) Status() (status Status, err error) {
	done := beginServiceOperation("status")
	defer func() { done(err) }()
	return manager.inner.Status()
}

func (manager *loggedManager) Install(autostart bool) (err error) {
	done := beginServiceOperation("install", "autostart", autostart)
	defer func() { done(err) }()
	return manager.inner.Install(autostart)
}

func (manager *loggedManager) Uninstall() (err error) {
	done := beginServiceOperation("uninstall")
	defer func() { done(err) }()
	return manager.inner.Uninstall()
}

func (manager *loggedManager) Start() (err error) {
	done := beginServiceOperation("start")
	defer func() { done(err) }()
	return manager.inner.Start()
}

func (manager *loggedManager) Stop() (err error) {
	done := beginServiceOperation("stop")
	defer func() { done(err) }()
	return manager.inner.Stop()
}

func (manager *loggedManager) Restart() (err error) {
	done := beginServiceOperation("restart")
	defer func() { done(err) }()
	return manager.inner.Restart()
}

func (manager *loggedManager) SetAutostart(enabled bool) (err error) {
	done := beginServiceOperation("set_autostart", "enabled", enabled)
	defer func() { done(err) }()
	return manager.inner.SetAutostart(enabled)
}
