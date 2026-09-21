package config

import (
	"fmt"
	"os"
)

// RunAction executes a system service action requested from the command line
// (for example `Termcp --system-service-action uninstall`).
func RunAction(action string, autostart bool) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	manager := NewManager(executable)
	switch action {
	case "install":
		return manager.Install(autostart)
	case "uninstall":
		return manager.Uninstall()
	case "start":
		return manager.Start()
	case "stop":
		return manager.Stop()
	case "restart":
		return manager.Restart()
	case "enable-autostart":
		return manager.SetAutostart(true)
	case "disable-autostart":
		return manager.SetAutostart(false)
	default:
		return fmt.Errorf("未知系统服务操作: %s", action)
	}
}
