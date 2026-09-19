package main

import (
	"fmt"
	"os"

	"github.com/open-mcp-ai/termcp/gui/internal/systemservice"
)

func runSystemServiceAction(action string) error {
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	manager := systemservice.New(executable)
	switch action {
	case "install":
		return manager.Install(hasArgument("--autostart"))
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
