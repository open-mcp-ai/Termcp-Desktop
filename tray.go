package main

import (
	_ "embed"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/open-mcp-ai/termcp/gui/internal/systemservice"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

const (
	productName    = "Termcp"
	projectName    = "Termcp-Desktop"
	productVersion = "0.1.0"
)

//go:embed build/trayicon.png
var trayIconPNG []byte

type trayAction int

const (
	trayShowWindow trayAction = iota + 1
	trayOpenWorkspace
	trayOpenService
	trayStartCore
	trayStopCore
	trayRestartCore
	trayToggleAutostart
	trayShowAbout
	trayQuit
)

type trayState struct {
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

type nativeTray interface {
	Start() error
	Update(trayState)
	Stop()
}

type trayController struct {
	app      *App
	native   nativeTray
	stop     chan struct{}
	stopOnce sync.Once
	wait     sync.WaitGroup
}

func newTrayController(app *App) *trayController {
	controller := &trayController{app: app, stop: make(chan struct{})}
	controller.native = newNativeTray(controller.dispatch)
	return controller
}

func (t *trayController) Start() error {
	if err := t.native.Start(); err != nil {
		return err
	}
	t.Refresh()
	t.wait.Add(1)
	go func() {
		defer t.wait.Done()
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.Refresh()
			case <-t.stop:
				return
			}
		}
	}()
	return nil
}

func (t *trayController) Stop() {
	t.stopOnce.Do(func() { close(t.stop) })
	t.wait.Wait()
	t.native.Stop()
}

func (t *trayController) Refresh() {
	service, err := t.app.service.Status()
	t.native.Update(formatTrayState(t.app.CoreStatus(), service, err, t.app.UILanguage()))
}

func formatTrayState(core CoreStatus, service systemservice.Status, serviceErr error, language string) trayState {
	state := trayState{
		ProductLine:      fmt.Sprintf("%s %s · %s", productName, productVersion, projectName),
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
	state.Tooltip = productName + " · " + strings.TrimPrefix(strings.TrimPrefix(state.CoreLine, "Core："), "Core: ")

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

func (t *trayController) dispatch(action trayAction) {
	go t.app.handleTrayAction(action)
}

func (a *App) handleTrayAction(action trayAction) {
	language := a.UILanguage()
	var message string
	var err error
	switch action {
	case trayShowWindow:
		a.showWindow("")
		return
	case trayOpenWorkspace:
		a.showWindow("workspace")
		return
	case trayOpenService:
		a.showWindow("service")
		return
	case trayStartCore:
		message, err = trayText(language, "Core 已启动", "Core started"), a.StartLocalCore()
	case trayStopCore:
		message, err = trayText(language, "Core 已停止", "Core stopped"), a.StopLocalCore()
	case trayRestartCore:
		message, err = trayText(language, "Core 已重启", "Core restarted"), a.RestartLocalCore()
	case trayToggleAutostart:
		status, statusErr := a.service.Status()
		if statusErr != nil {
			err = statusErr
		} else if !status.Installed {
			err = errors.New(trayText(language, "请先注册 Core 系统服务", "Register the Core system service first"))
		} else {
			err = a.SetCoreAutostart(!status.Autostart)
			if status.Autostart {
				message = trayText(language, "已关闭开机自启", "Autostart disabled")
			} else {
				message = trayText(language, "已开启开机自启", "Autostart enabled")
			}
		}
	case trayShowAbout:
		a.showWindow("")
		_, err = runtime.MessageDialog(a.ctx, runtime.MessageDialogOptions{
			Type:    runtime.InfoDialog,
			Title:   trayText(language, "关于 Termcp", "About Termcp"),
			Message: fmt.Sprintf("%s %s\n%s\n\n%s\nApache License 2.0", productName, productVersion, projectName, trayText(language, "本机 termcp Core 管理端与 SSH 工作台", "Local termcp Core manager and SSH workspace")),
			Buttons: []string{trayText(language, "确定", "OK")},
		})
	case trayQuit:
		runtime.Quit(a.ctx)
		return
	}
	if a.tray != nil {
		a.tray.Refresh()
	}
	if err != nil {
		runtime.EventsEmit(a.ctx, "termcp:tray-result", "", err.Error())
		return
	}
	if message != "" {
		runtime.EventsEmit(a.ctx, "termcp:tray-result", message, "")
	}
}

func (a *App) showWindow(section string) {
	runtime.WindowShow(a.ctx)
	runtime.WindowUnminimise(a.ctx)
	if section != "" {
		runtime.EventsEmit(a.ctx, "termcp:navigate", section)
	}
}

func (a *App) refreshTray() {
	if a.tray != nil {
		a.tray.Refresh()
	}
}
