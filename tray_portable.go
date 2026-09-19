//go:build windows || linux

package main

import (
	"runtime"
	"sync"

	"github.com/gogpu/systray"
)

type portableTray struct {
	dispatch  func(trayAction)
	tray      *systray.SystemTray
	product   *systray.MenuItem
	core      *systray.MenuItem
	service   *systray.MenuItem
	show      *systray.MenuItem
	workspace *systray.MenuItem
	manage    *systray.MenuItem
	start     *systray.MenuItem
	stop      *systray.MenuItem
	restart   *systray.MenuItem
	autostart *systray.MenuItem
	about     *systray.MenuItem
	quit      *systray.MenuItem
	ready     chan struct{}
	stopOnce  sync.Once
	mu        sync.RWMutex
}

func newNativeTray(dispatch func(trayAction)) nativeTray {
	return &portableTray{dispatch: dispatch, ready: make(chan struct{})}
}

func (t *portableTray) Start() error {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		tray := systray.New()
		menu := systray.NewMenu()
		product := menu.Add(productName+" "+productVersion+" · "+projectName, nil)
		product.SetDisabled(true)
		core := menu.Add("Core：正在读取状态", nil)
		core.SetDisabled(true)
		service := menu.Add("系统服务：正在读取状态", nil)
		service.SetDisabled(true)
		menu.AddSeparator()
		show := menu.Add("Termcp", func() { t.dispatch(trayShowWindow) })
		workspace := menu.Add("SSH", func() { t.dispatch(trayOpenWorkspace) })
		manage := menu.Add("Service", func() { t.dispatch(trayOpenService) })
		menu.AddSeparator()
		start := menu.Add("启动 Core", func() { t.dispatch(trayStartCore) })
		stop := menu.Add("停止 Core", func() { t.dispatch(trayStopCore) })
		restart := menu.Add("重启 Core", func() { t.dispatch(trayRestartCore) })
		autostart := menu.AddCheckbox("开机自启", false, func() { t.dispatch(trayToggleAutostart) })
		menu.AddSeparator()
		about := menu.Add("About", func() { t.dispatch(trayShowAbout) })
		quit := menu.Add("Quit", func() { t.dispatch(trayQuit) })

		t.mu.Lock()
		t.tray, t.product, t.core, t.service = tray, product, core, service
		t.show, t.workspace, t.manage = show, workspace, manage
		t.start, t.stop, t.restart, t.autostart = start, stop, restart, autostart
		t.about, t.quit = about, quit
		t.mu.Unlock()

		tray.SetIcon(trayIconPNG).
			SetAppName(productName).
			SetTooltip(productName).
			SetMenu(menu).
			OnClick(func() { t.dispatch(trayShowWindow) }).
			Show()
		close(t.ready)
		_ = tray.Run()
	}()
	<-t.ready
	return nil
}

func (t *portableTray) Update(state trayState) {
	<-t.ready
	t.mu.RLock()
	defer t.mu.RUnlock()
	if t.tray == nil {
		return
	}
	t.product.SetLabel(state.ProductLine)
	t.core.SetLabel(state.CoreLine)
	t.service.SetLabel(state.ServiceLine)
	t.show.SetLabel(state.ShowWindowLabel)
	t.workspace.SetLabel(state.WorkspaceLabel)
	t.manage.SetLabel(state.ServiceLabel)
	t.start.SetLabel(state.StartCoreLabel)
	t.stop.SetLabel(state.StopCoreLabel)
	t.restart.SetLabel(state.RestartCoreLabel)
	t.autostart.SetLabel(state.AutostartLabel)
	t.about.SetLabel(state.AboutLabel)
	t.quit.SetLabel(state.QuitLabel)
	t.start.SetDisabled(state.CoreRunning)
	t.stop.SetDisabled(!state.CoreRunning)
	t.restart.SetDisabled(false)
	t.autostart.SetDisabled(!state.ServiceSupported || !state.ServiceInstalled)
	t.autostart.SetChecked(state.ServiceAutostart)
	t.tray.SetTooltip(state.Tooltip)
}

func (t *portableTray) Stop() {
	t.stopOnce.Do(func() {
		<-t.ready
		t.mu.RLock()
		defer t.mu.RUnlock()
		if t.tray != nil {
			t.tray.Remove()
		}
	})
}
