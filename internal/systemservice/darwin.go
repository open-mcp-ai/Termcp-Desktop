//go:build darwin

package systemservice

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type darwinManager struct {
	executable string
	home       string
	uid        int
}

func newPlatformManager(executable string) Manager {
	home, _ := os.UserHomeDir()
	return &darwinManager{executable: executable, home: home, uid: os.Getuid()}
}

func (m *darwinManager) plistPath() string {
	return filepath.Join(m.home, "Library", "LaunchAgents", Label+".plist")
}
func (m *darwinManager) logPath() string {
	return filepath.Join(m.home, ".termcp", "logs", "termcp-desktop-core.log")
}
func (m *darwinManager) domainTarget() string { return fmt.Sprintf("gui/%d/%s", m.uid, Label) }
func (m *darwinManager) domain() string       { return fmt.Sprintf("gui/%d", m.uid) }

func (m *darwinManager) Status() (Status, error) {
	status := Status{Supported: true, Platform: "darwin", Label: Label, Definition: m.plistPath(), LogPath: m.logPath(), Executable: m.executable, Description: "macOS LaunchAgent"}
	data, err := os.ReadFile(m.plistPath())
	if err == nil {
		status.Installed = true
		status.Autostart = plistBool(data, "RunAtLoad")
	} else if !os.IsNotExist(err) {
		return status, err
	}
	output, printErr := exec.Command("launchctl", "print", m.domainTarget()).CombinedOutput()
	if printErr == nil {
		text := string(output)
		status.Running = strings.Contains(text, "state = running")
		status.PID = launchdPID(text)
	}
	return status, nil
}

func (m *darwinManager) Install(autostart bool) error {
	if strings.TrimSpace(m.executable) == "" {
		return errors.New("无法确定 Termcp 可执行文件路径")
	}
	if err := os.MkdirAll(filepath.Dir(m.plistPath()), 0o755); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(m.logPath()), 0o700); err != nil {
		return err
	}
	data, err := renderPlist(m.executable, m.logPath(), filepath.Join(m.home, ".termcp"), autostart)
	if err != nil {
		return err
	}
	temp := m.plistPath() + ".tmp"
	if err := os.WriteFile(temp, data, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temp, m.plistPath()); err != nil {
		return err
	}
	m.unload()
	var lastErr error
	for attempt := 0; attempt < 3; attempt++ {
		output, err := exec.Command("launchctl", "bootstrap", m.domain(), m.plistPath()).CombinedOutput()
		if err == nil {
			return nil
		}
		lastErr = fmt.Errorf("注册 LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
		// launchd can still be tearing down the previous instance; boot it out and retry.
		m.unload()
	}
	return lastErr
}

func (m *darwinManager) Uninstall() error {
	m.unload()
	if err := os.Remove(m.plistPath()); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// unload boots the service out of the user domain and waits until launchd has
// actually released the job, so an immediate bootstrap cannot fail with EIO.
func (m *darwinManager) unload() {
	_ = exec.Command("launchctl", "bootout", m.domainTarget()).Run()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if exec.Command("launchctl", "print", m.domainTarget()).Run() != nil {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func (m *darwinManager) Start() error {
	status, err := m.Status()
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("Core 系统服务尚未注册")
	}
	if output, err := exec.Command("launchctl", "kickstart", "-k", m.domainTarget()).CombinedOutput(); err != nil {
		return fmt.Errorf("启动 LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *darwinManager) Stop() error {
	status, err := m.Status()
	if err != nil || !status.Running {
		return err
	}
	if output, err := exec.Command("launchctl", "kill", "SIGTERM", m.domainTarget()).CombinedOutput(); err != nil {
		return fmt.Errorf("停止 LaunchAgent: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *darwinManager) Restart() error {
	return m.Start()
}

func (m *darwinManager) SetAutostart(enabled bool) error {
	status, err := m.Status()
	if err != nil {
		return err
	}
	if !status.Installed {
		return errors.New("Core 系统服务尚未注册")
	}
	wasRunning := status.Running
	if err := m.Install(enabled); err != nil {
		return err
	}
	if wasRunning && !enabled {
		return m.Start()
	}
	return nil
}

type plistDocument struct {
	XMLName xml.Name  `xml:"plist"`
	Version string    `xml:"version,attr"`
	Dict    plistDict `xml:"dict"`
}

type plistDict struct {
	Inner string `xml:",innerxml"`
}

func renderPlist(executable, logPath, dataDir string, autostart bool) ([]byte, error) {
	inner := strings.Join([]string{
		"<key>Label</key><string>" + xmlEscape(Label) + "</string>",
		"<key>ProgramArguments</key><array><string>" + xmlEscape(executable) + "</string><string>--core-service</string><string>--core-data-dir</string><string>" + xmlEscape(dataDir) + "</string></array>",
		"<key>RunAtLoad</key><" + strconv.FormatBool(autostart) + "/>",
		"<key>KeepAlive</key><false/>",
		"<key>ProcessType</key><string>Background</string>",
		"<key>StandardOutPath</key><string>" + xmlEscape(logPath) + "</string>",
		"<key>StandardErrorPath</key><string>" + xmlEscape(logPath) + "</string>",
		"<key>EnvironmentVariables</key><dict><key>TERMCP_DESKTOP_SERVICE</key><string>1</string></dict>",
	}, "")
	document := plistDocument{Version: "1.0", Dict: plistDict{Inner: inner}}
	data, err := xml.MarshalIndent(document, "", "  ")
	if err != nil {
		return nil, err
	}
	header := []byte(xml.Header + `<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">` + "\n")
	return append(header, data...), nil
}

func xmlEscape(value string) string {
	var buffer bytes.Buffer
	_ = xml.EscapeText(&buffer, []byte(value))
	return buffer.String()
}

func plistBool(data []byte, key string) bool {
	text := string(data)
	position := strings.Index(text, "<key>"+key+"</key>")
	if position < 0 {
		return false
	}
	rest := text[position:]
	return strings.Index(rest, "<true/>") >= 0 && (strings.Index(rest, "<false/>") < 0 || strings.Index(rest, "<true/>") < strings.Index(rest, "<false/>"))
}

func launchdPID(output string) int {
	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "pid = ") {
			continue
		}
		pid, _ := strconv.Atoi(strings.TrimSpace(strings.TrimPrefix(line, "pid = ")))
		return pid
	}
	return 0
}
