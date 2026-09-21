//go:build linux

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type linuxManager struct {
	executable string
	home       string
}

func newPlatformManager(executable string) Manager {
	home, _ := os.UserHomeDir()
	return &linuxManager{executable: executable, home: home}
}

func (m *linuxManager) unitPath() string {
	return filepath.Join(m.home, ".config", "systemd", "user", Label+".service")
}

func (m *linuxManager) logPath() string {
	return filepath.Join(m.home, ".termcp", "logs")
}

func (m *linuxManager) Status() (Status, error) {
	status := Status{Supported: true, Platform: "linux", Label: Label, Definition: m.unitPath(), LogPath: m.logPath(), Executable: m.executable, Description: "systemd 用户服务"}
	if _, err := os.Stat(m.unitPath()); err == nil {
		status.Installed = true
	} else if !os.IsNotExist(err) {
		return status, err
	}
	if !status.Installed {
		return status, nil
	}
	if output, err := m.systemctl("is-enabled"); err == nil && strings.TrimSpace(output) == "enabled" {
		status.Autostart = true
	}
	if output, err := m.systemctl("show", "--property=ActiveState", "--property=MainPID", "--value"); err == nil {
		lines := strings.Fields(output)
		for _, line := range lines {
			if line == "active" {
				status.Running = true
				continue
			}
			if pid, parseErr := strconv.Atoi(line); parseErr == nil && pid > 0 {
				status.PID = pid
			}
		}
	}
	return status, nil
}

func (m *linuxManager) Install(autostart bool) error {
	if strings.TrimSpace(m.executable) == "" {
		return errors.New("无法确定 Termcp 可执行文件路径")
	}
	if err := os.MkdirAll(filepath.Dir(m.unitPath()), 0o755); err != nil {
		return err
	}
	data := renderSystemdUnit(m.executable, filepath.Join(m.home, ".termcp"))
	temp := m.unitPath() + ".tmp"
	if err := os.WriteFile(temp, []byte(data), 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, m.unitPath()); err != nil {
		return err
	}
	if _, err := m.systemctlGlobal("daemon-reload"); err != nil {
		return err
	}
	return m.SetAutostart(autostart)
}

func (m *linuxManager) Uninstall() error {
	_, _ = m.systemctl("disable", "--now")
	if err := os.Remove(m.unitPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	_, err := m.systemctlGlobal("daemon-reload")
	return err
}

func (m *linuxManager) Start() error {
	status, err := m.Status()
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("Core 系统服务尚未注册")
	}
	_, err = m.systemctl("start")
	return err
}

func (m *linuxManager) Stop() error {
	status, err := m.Status()
	if err != nil || !status.Running {
		return err
	}
	_, err = m.systemctl("stop")
	return err
}

func (m *linuxManager) Restart() error {
	status, err := m.Status()
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("Core 系统服务尚未注册")
	}
	_, err = m.systemctl("restart")
	return err
}

func (m *linuxManager) SetAutostart(enabled bool) error {
	status, err := m.Status()
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("Core 系统服务尚未注册")
	}
	action := "disable"
	if enabled {
		action = "enable"
	}
	_, err = m.systemctl(action)
	return err
}

func (m *linuxManager) systemctl(args ...string) (string, error) {
	commandArgs := append([]string{"--user"}, args...)
	commandArgs = append(commandArgs, Label+".service")
	return runSystemctl(commandArgs, args)
}

func (m *linuxManager) systemctlGlobal(args ...string) (string, error) {
	commandArgs := append([]string{"--user"}, args...)
	return runSystemctl(commandArgs, args)
}

func runSystemctl(commandArgs, displayArgs []string) (string, error) {
	output, err := exec.Command("systemctl", commandArgs...).CombinedOutput()
	if err != nil {
		return string(output), fmt.Errorf("systemctl %s: %w: %s", strings.Join(displayArgs, " "), err, strings.TrimSpace(string(output)))
	}
	return string(output), nil
}

func renderSystemdUnit(executable, dataDir string) string {
	return strings.Join([]string{
		"[Unit]",
		"Description=termcp Core managed by Termcp",
		"",
		"[Service]",
		"Type=simple",
		"ExecStart=" + systemdQuote(executable) + " --core-service --core-data-dir " + systemdQuote(dataDir),
		"Restart=on-failure",
		"RestartSec=2",
		"Environment=TERMCP_DESKTOP_SERVICE=1",
		"",
		"[Install]",
		"WantedBy=default.target",
		"",
	}, "\n")
}

func systemdQuote(value string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(value) + `"`
}
