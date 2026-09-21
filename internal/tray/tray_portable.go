//go:build windows || linux

package tray

import (
	"runtime"
	"sync"

	"github.com/gogpu/systray"
	"github.com/open-mcp-ai/termcp/gui/internal/config"
)

type portableTray struct {
	dispatch  func(action)
	icon      []byte
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

func newNativeTray(icon []byte, dispatch func(action)) nativeTray {
	return &portableTray{dispatch: dispatch, icon: icon, ready: make(chan struct{})}
}

func (t *portableTray) Start() error {
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()

		tray := systray.New()
		menu := systray.NewMenu()
		product := menu.Add(config.ProductName+" "+config.ProductVersion+" · "+config.ProjectName, nil)
		product.SetDisabled(true)
		core := menu.Add("Core：正在读取状态", nil)
		core.SetDisabled(true)
		service := menu.Add("系统服务：正在读取状态", nil)
		service.SetDisabled(true)
		menu.AddSeparator()
		show := menu.Add("Termcp", func() { t.dispatch(actionShowWindow) })
		workspace := menu.Add("SSH", func() { t.dispatch(actionOpenWorkspace) })
		manage := menu.Add("Service", func() { t.dispatch(actionOpenService) })
		menu.AddSeparator()
		start := menu.Add("启动 Core", func() { t.dispatch(actionStartCore) })
		stop := menu.Add("停止 Core", func() { t.dispatch(actionStopCore) })
		restart := menu.Add("重启 Core", func() { t.dispatch(actionRestartCore) })
		autostart := menu.AddCheckbox("开机自启", false, func() { t.dispatch(actionToggleAutostart) })
		menu.AddSeparator()
		about := menu.Add("About", func() { t.dispatch(actionShowAbout) })
		quit := menu.Add("Quit", func() { t.dispatch(actionQuit) })

		t.mu.Lock()
		t.tray, t.product, t.core, t.service = tray, product, core, service
		t.show, t.workspace, t.manage = show, workspace, manage
		t.start, t.stop, t.restart, t.autostart = start, stop, restart, autostart
		t.about, t.quit = about, quit
		t.mu.Unlock()

		configured := tray.
			SetAppName(config.ProductName).
			SetTooltip(config.ProductName).
			SetMenu(menu).
			OnClick(func() { t.dispatch(actionShowWindow) })
		if len(t.icon) > 0 {
			configured = configured.SetIcon(t.icon)
		}
		configured.Show()
		close(t.ready)
		_ = tray.Run()
	}()
	<-t.ready
	return nil
}

func (t *portableTray) Update(state State) {
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
