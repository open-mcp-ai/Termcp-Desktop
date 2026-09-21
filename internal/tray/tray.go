// Package tray renders the system tray menu and routes its actions back to the
// desktop shell through Host callbacks.
package tray

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/open-mcp-ai/termcp/gui/internal/config"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type action int

const (
	actionShowWindow action = iota + 1
	actionOpenWorkspace
	actionOpenService
	actionStartCore
	actionStopCore
	actionRestartCore
	actionToggleAutostart
	actionShowAbout
	actionQuit
)

type State struct {
	ProductLine      string
	CoreLine         string
	ServiceLine      string
	Tooltip          string
	ShowWindowLabel  string
	WorkspaceLabel   string
	ServiceLabel     string
	StartCoreLabel   string
	StopCoreLabel    string
	RestartCoreLabel string
	AutostartLabel   string
	AboutLabel       string
	QuitLabel        string
	CoreRunning      bool
	ServiceSupported bool
	ServiceInstalled bool
	ServiceAutostart bool
}

// Host exposes the desktop shell operations the tray needs. The shell wires
// these callbacks in main so the tray does not depend on the binding layer.
type Host struct {
	Context       func() context.Context
	Language      func() string
	CoreStatus    func() model.CoreStatus
	ServiceStatus func() (config.Status, error)
	StartCore     func() error
	StopCore      func() error
	RestartCore   func() error
	SetAutostart  func(enabled bool) error
	ShowWindow    func(section string)
}

type nativeTray interface {
	Start() error
	Update(State)
	Stop()
}

type Controller struct {
	host     Host
	native   nativeTray
	stop     chan struct{}
	stopOnce sync.Once
	wait     sync.WaitGroup
}

func New(host Host, icon []byte) *Controller {
	controller := &Controller{host: host, stop: make(chan struct{})}
	controller.native = newNativeTray(icon, controller.dispatch)
	return controller
}

func (c *Controller) Start() error {
	if err := c.native.Start(); err != nil {
		return err
	}
	c.Refresh()
	c.wait.Add(1)
	go func() {
		defer c.wait.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.Refresh()
			case <-c.stop:
				return
			}
		}
	}()
	return nil
}

func (c *Controller) Stop() {
	c.stopOnce.Do(func() { close(c.stop) })
	c.wait.Wait()
	c.native.Stop()
}

func (c *Controller) Refresh() {
	service, err := c.host.ServiceStatus()
	c.native.Update(formatState(c.host.CoreStatus(), service, err, c.host.Language()))
}

func (c *Controller) dispatch(action action) {
	go c.handleAction(action)
}

func (c *Controller) handleAction(action action) {
	language := c.host.Language()
	var message string
	var err error
	switch action {
	case actionShowWindow:
		c.host.ShowWindow("")
		return
	case actionOpenWorkspace:
		c.host.ShowWindow("workspace")
		return
	case actionOpenService:
		c.host.ShowWindow("service")
		return
	case actionStartCore:
		message, err = trayText(language, "Core 已启动", "Core started"), c.host.StartCore()
	case actionStopCore:
		message, err = trayText(language, "Core 已停止", "Core stopped"), c.host.StopCore()
	case actionRestartCore:
		message, err = trayText(language, "Core 已重启", "Core restarted"), c.host.RestartCore()
	case actionToggleAutostart:
		status, statusErr := c.host.ServiceStatus()
		if statusErr != nil {
			err = statusErr
		} else if !status.Installed {
			err = errors.New(trayText(language, "请先注册 Core 系统服务", "Register the Core system service first"))
		} else {
			err = c.host.SetAutostart(!status.Autostart)
			if status.Autostart {
				message = trayText(language, "已关闭开机自启", "Autostart disabled")
			} else {
				message = trayText(language, "已开启开机自启", "Autostart enabled")
			}
		}
	case actionShowAbout:
		c.host.ShowWindow("")
		_, err = runtime.MessageDialog(c.host.Context(), runtime.MessageDialogOptions{
			Type:    runtime.InfoDialog,
			Title:   trayText(language, "关于 Termcp", "About Termcp"),
			Message: fmt.Sprintf("%s %s\n%s\n\n%s\nApache License 2.0", config.ProductName, config.ProductVersion, config.ProjectName, trayText(language, "本机 termcp Core 管理端与 SSH 工作台", "Local termcp Core manager and SSH workspace")),
			Buttons: []string{trayText(language, "确定", "OK")},
		})
	case actionQuit:
		runtime.Quit(c.host.Context())
		return
	}
	c.Refresh()
	if err != nil {
		runtime.EventsEmit(c.host.Context(), "termcp:tray-result", "", err.Error())
		return
	}
	if message != "" {
		runtime.EventsEmit(c.host.Context(), "termcp:tray-result", message, "")
	}
}

func formatState(core model.CoreStatus, service config.Status, serviceErr error, language string) State {
	state := State{
		ProductLine:      fmt.Sprintf("%s %s · %s", config.ProductName, config.ProductVersion, config.ProjectName),
		ShowWindowLabel:  trayText(language, "显示 Termcp", "Show Termcp"),
		WorkspaceLabel:   trayText(language, "打开 SSH 工作台", "Open SSH workspace"),
		ServiceLabel:     trayText(language, "打开服务管理", "Open service management"),
		StartCoreLabel:   trayText(language, "启动 Core", "Start Core"),
		StopCoreLabel:    trayText(language, "停止 Core", "Stop Core"),
		RestartCoreLabel: trayText(language, "重启 Core", "Restart Core"),
		AutostartLabel:   trayText(language, "开机自启", "Autostart"),
		AboutLabel:       trayText(language, "关于 Termcp", "About Termcp"),
		QuitLabel:        trayText(language, "退出 Termcp", "Quit Termcp"),
		CoreRunning:      core.Running,
		ServiceSupported: service.Supported,
		ServiceInstalled: service.Installed,
		ServiceAutostart: service.Autostart,
	}
	address := strings.TrimPrefix(strings.TrimPrefix(core.Address, "http://"), "https://")
	if core.Running {
		state.CoreLine = trayText(language, "Core：运行中", "Core: Running")
		if address != "" {
			state.CoreLine += " · " + address
		}
	} else if core.Error != "" {
		state.CoreLine = trayText(language, "Core：异常 · ", "Core: Error · ") + core.Error
	} else {
		state.CoreLine = trayText(language, "Core：已停止", "Core: Stopped")
	}
	state.Tooltip = config.ProductName + " · " + strings.TrimPrefix(strings.TrimPrefix(state.CoreLine, "Core："), "Core: ")

	switch {
	case serviceErr != nil:
		state.ServiceLine = trayText(language, "系统服务：状态读取失败", "System service: Status unavailable")
	case !service.Supported:
		state.ServiceLine = trayText(language, "系统服务：当前平台不支持", "System service: Unsupported")
	case !service.Installed:
		state.ServiceLine = trayText(language, "系统服务：未注册", "System service: Not registered")
	default:
		running := trayText(language, "已停止", "Stopped")
		if service.Running {
			running = trayText(language, "运行中", "Running")
		}
		autostart := trayText(language, "自启关闭", "Autostart off")
		if service.Autostart {
			autostart = trayText(language, "开机自启", "Autostart")
		}
		state.ServiceLine = trayText(language, "系统服务：已注册 · ", "System service: Registered · ") + running + " · " + autostart
	}
	return state
}

func trayText(language, chinese, english string) string {
	if language == "en" {
		return english
	}
	return chinese
}
