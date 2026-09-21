//go:build windows

package config

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

type windowsManager struct {
	executable string
	dataDir    string
}

func newPlatformManager(executable string) Manager {
	return &windowsManager{executable: executable, dataDir: DataDir()}
}

func (m *windowsManager) Status() (Status, error) {
	status := Status{Supported: true, Platform: "windows", Label: WindowsServiceName, Definition: "services.msc · " + WindowsServiceName, LogPath: filepath.Join(m.dataDir, "logs"), Executable: m.executable, Description: "Windows 系统服务"}
	managerHandle, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT)
	if err != nil {
		return status, fmt.Errorf("连接 Windows 服务管理器: %w", err)
	}
	defer windows.CloseServiceHandle(managerHandle)
	name, err := windows.UTF16PtrFromString(WindowsServiceName)
	if err != nil {
		return status, err
	}
	serviceHandle, err := windows.OpenService(managerHandle, name, windows.SERVICE_QUERY_CONFIG|windows.SERVICE_QUERY_STATUS)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return status, nil
		}
		return status, err
	}
	service := &mgr.Service{Name: WindowsServiceName, Handle: serviceHandle}
	defer service.Close()
	status.Installed = true
	if config, configErr := service.Config(); configErr == nil {
		status.Autostart = config.StartType == mgr.StartAutomatic
		status.Executable = config.BinaryPathName
	}
	if serviceStatus, queryErr := service.Query(); queryErr == nil {
		status.Running = serviceStatus.State == svc.Running || serviceStatus.State == svc.StartPending
		status.PID = int(serviceStatus.ProcessId)
	}
	return status, nil
}

func (m *windowsManager) Install(autostart bool) error {
	return m.withElevation("install", autostart, func() error { return m.install(autostart) })
}

func (m *windowsManager) install(autostart bool) error {
	if strings.TrimSpace(m.executable) == "" {
		return errors.New("无法确定 Termcp 可执行文件路径")
	}
	if err := os.MkdirAll(m.dataDir, 0o700); err != nil {
		return fmt.Errorf("创建 Core 数据目录: %w", err)
	}
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("连接 Windows 服务管理器: %w", err)
	}
	defer manager.Disconnect()
	startType := uint32(mgr.StartManual)
	if autostart {
		startType = mgr.StartAutomatic
	}
	config := mgr.Config{
		DisplayName:      "termcp Core",
		Description:      "Termcp 管理的本机 termcp Core 服务",
		StartType:        startType,
		ServiceStartName: `NT SERVICE\` + WindowsServiceName,
		SidType:          windows.SERVICE_SID_TYPE_UNRESTRICTED,
	}
	if existing, openErr := manager.OpenService(WindowsServiceName); openErr == nil {
		defer existing.Close()
		old, configErr := existing.Config()
		if configErr != nil {
			return configErr
		}
		old.DisplayName, old.Description, old.StartType = config.DisplayName, config.Description, config.StartType
		old.ServiceStartName = config.ServiceStartName
		old.BinaryPathName = windowsCommandLine(m.executable, m.dataDir)
		_ = eventlog.InstallAsEventCreate(WindowsServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
		if err := existing.UpdateConfig(old); err != nil {
			return err
		}
		if err := grantWindowsDataAccess(m.dataDir); err != nil {
			return err
		}
		if current, queryErr := existing.Query(); queryErr == nil && current.State != svc.Stopped {
			_, _ = existing.Control(svc.Stop)
			if err := waitWindowsState(existing, svc.Stopped, 15*time.Second); err != nil {
				return err
			}
		}
		if err := existing.Start(); err != nil {
			return err
		}
		return waitWindowsState(existing, svc.Running, 15*time.Second)
	}
	service, err := manager.CreateService(WindowsServiceName, m.executable, config, "--core-service", "--core-data-dir", m.dataDir)
	if err != nil {
		return fmt.Errorf("注册 Windows 服务: %w", err)
	}
	_ = eventlog.InstallAsEventCreate(WindowsServiceName, eventlog.Error|eventlog.Warning|eventlog.Info)
	defer service.Close()
	if err := grantWindowsDataAccess(m.dataDir); err != nil {
		return err
	}
	if err := service.Start(); err != nil {
		return err
	}
	return waitWindowsState(service, svc.Running, 15*time.Second)
}

func (m *windowsManager) Uninstall() error {
	return m.withElevation("uninstall", false, m.uninstall)
}

func (m *windowsManager) uninstall() error {
	manager, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer manager.Disconnect()
	service, err := manager.OpenService(WindowsServiceName)
	if err != nil {
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil
		}
		return err
	}
	defer service.Close()
	_, _ = service.Control(svc.Stop)
	_ = waitWindowsState(service, svc.Stopped, 10*time.Second)
	if err := service.Delete(); err != nil {
		return err
	}
	_ = eventlog.Remove(WindowsServiceName)
	_ = revokeWindowsDataAccess(m.dataDir)
	return nil
}

func (m *windowsManager) Start() error {
	if status, err := m.Status(); err == nil && status.Running {
		return nil
	}
	return m.withElevation("start", false, m.start)
}

func (m *windowsManager) start() error {
	service, cleanup, err := openWindowsService()
	if err != nil {
		return err
	}
	defer cleanup()
	if status, queryErr := service.Query(); queryErr == nil && status.State == svc.Running {
		return nil
	}
	if err := service.Start(); err != nil {
		return fmt.Errorf("启动 Windows 服务: %w", err)
	}
	return waitWindowsState(service, svc.Running, 15*time.Second)
}

func (m *windowsManager) Stop() error {
	if status, err := m.Status(); err == nil && !status.Running {
		return nil
	}
	return m.withElevation("stop", false, m.stop)
}

func (m *windowsManager) stop() error {
	service, cleanup, err := openWindowsService()
	if err != nil {
		return err
	}
	defer cleanup()
	status, err := service.Query()
	if err != nil || status.State == svc.Stopped {
		return err
	}
	if _, err = service.Control(svc.Stop); err != nil {
		return fmt.Errorf("停止 Windows 服务: %w", err)
	}
	return waitWindowsState(service, svc.Stopped, 15*time.Second)
}

func (m *windowsManager) Restart() error {
	return m.withElevation("restart", false, func() error {
		if err := m.stop(); err != nil {
			return err
		}
		return m.start()
	})
}

func (m *windowsManager) SetAutostart(enabled bool) error {
	action := "disable-autostart"
	if enabled {
		action = "enable-autostart"
	}
	return m.withElevation(action, false, func() error { return m.setAutostart(enabled) })
}

func (m *windowsManager) setAutostart(enabled bool) error {
	service, cleanup, err := openWindowsService()
	if err != nil {
		return err
	}
	defer cleanup()
	config, err := service.Config()
	if err != nil {
		return err
	}
	config.StartType = mgr.StartManual
	if enabled {
		config.StartType = mgr.StartAutomatic
	}
	return service.UpdateConfig(config)
}

func openWindowsService() (*mgr.Service, func(), error) {
	manager, err := mgr.Connect()
	if err != nil {
		return nil, func() {}, fmt.Errorf("连接 Windows 服务管理器: %w", err)
	}
	service, err := manager.OpenService(WindowsServiceName)
	if err != nil {
		manager.Disconnect()
		if errors.Is(err, windows.ERROR_SERVICE_DOES_NOT_EXIST) {
			return nil, func() {}, errors.New("Core 系统服务尚未注册")
		}
		return nil, func() {}, fmt.Errorf("打开 Windows 服务: %w", err)
	}
	return service, func() { service.Close(); manager.Disconnect() }, nil
}

func waitWindowsState(service *mgr.Service, expected svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		status, err := service.Query()
		if err != nil {
			return err
		}
		if status.State == expected {
			return nil
		}
		time.Sleep(150 * time.Millisecond)
	}
	return fmt.Errorf("等待 Windows 服务状态 %d 超时", expected)
}

func windowsCommandLine(executable, dataDir string) string {
	quote := func(value string) string { return `"` + strings.ReplaceAll(value, `"`, `\"`) + `"` }
	return quote(executable) + ` --core-service --core-data-dir ` + quote(dataDir)
}

func grantWindowsDataAccess(dataDir string) error {
	principal := `NT SERVICE\` + WindowsServiceName + `:(OI)(CI)M`
	output, err := exec.Command("icacls.exe", dataDir, "/grant", principal, "/T", "/C").CombinedOutput()
	if err != nil {
		return fmt.Errorf("授权 Core 服务访问数据目录: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func revokeWindowsDataAccess(dataDir string) error {
	principal := `NT SERVICE\` + WindowsServiceName
	output, err := exec.Command("icacls.exe", dataDir, "/remove", principal, "/T", "/C").CombinedOutput()
	if err != nil {
		return fmt.Errorf("移除 Core 服务数据目录权限: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *windowsManager) withElevation(action string, autostart bool, operation func() error) error {
	err := operation()
	if err == nil || !errors.Is(err, windows.ERROR_ACCESS_DENIED) || windows.GetCurrentProcessToken().IsElevated() {
		return err
	}
	return m.elevate(action, autostart)
}

func (m *windowsManager) elevate(action string, autostart bool) error {
	arguments := []string{"--system-service-action", action}
	if autostart {
		arguments = append(arguments, "--autostart")
	}
	quoted := make([]string, 0, len(arguments))
	for _, argument := range arguments {
		quoted = append(quoted, "'"+strings.ReplaceAll(argument, "'", "''")+"'")
	}
	executable := strings.ReplaceAll(m.executable, "'", "''")
	script := fmt.Sprintf("$p=Start-Process -FilePath '%s' -ArgumentList @(%s) -Verb RunAs -Wait -PassThru; exit $p.ExitCode", executable, strings.Join(quoted, ","))
	command := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	command.Env = os.Environ()
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("需要管理员权限执行 Windows 服务操作: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
