package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/open-mcp-ai/termcp/gui/internal/bridge"
	"github.com/open-mcp-ai/termcp/gui/internal/config"
	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/fonts"
	"github.com/open-mcp-ai/termcp/gui/internal/model"
	"github.com/open-mcp-ai/termcp/gui/internal/tray"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	core       *corepkg.Service
	bridge     *bridge.Client
	service    config.Manager
	tray       *tray.Controller
	dataDir    string
	lifecycle  sync.Mutex
	language   sync.RWMutex
	uiLanguage string
}

func NewApp() *App {
	app := newApp(corepkg.New("127.0.0.1", 18765))
	app.tray = tray.New(trayHost(app), trayIconPNG)
	return app
}

func newApp(core *corepkg.Service) *App {
	executable, _ := os.Executable()
	return newAppWithService(core, config.NewManager(executable))
}

func newAppWithService(core *corepkg.Service, service config.Manager) *App {
	dataDir := strings.TrimSpace(os.Getenv("TERMCP_DATA_DIR"))
	if dataDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataDir = filepath.Join(home, ".termcp")
		}
	}
	return &App{
		core:       core,
		bridge:     bridge.New(core),
		service:    service,
		dataDir:    dataDir,
		uiLanguage: defaultUILanguage(),
	}
}

// trayHost adapts the App to the tray package through callbacks so the tray
// does not depend on the binding layer.
func trayHost(app *App) tray.Host {
	return tray.Host{
		Context:       func() context.Context { return app.ctx },
		Language:      app.UILanguage,
		CoreStatus:    app.CoreStatus,
		ServiceStatus: app.service.Status,
		StartCore:     app.StartLocalCore,
		StopCore:      app.StopLocalCore,
		RestartCore:   app.RestartLocalCore,
		SetAutostart:  app.SetCoreAutostart,
		ShowWindow:    app.showWindow,
	}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	status, statusErr := a.service.Status()
	if statusErr != nil {
		runtime.LogErrorf(ctx, "读取 Core 系统服务状态失败: %v", statusErr)
	}
	var err error
	if status.Installed {
		if status.Autostart && !status.Running {
			err = a.service.Start()
		}
		if err == nil && (status.Running || status.Autostart) {
			err = a.core.Attach(12 * time.Second)
		}
	} else {
		err = a.core.Start()
	}
	if err != nil {
		runtime.LogErrorf(ctx, "Core 启动失败: %v", err)
	}
	if a.tray != nil {
		if err := a.tray.Start(); err != nil {
			runtime.LogErrorf(ctx, "系统托盘启动失败: %v", err)
		}
	}
}

func (a *App) shutdown(context.Context) {
	if a.tray != nil {
		a.tray.Stop()
	}
	if err := a.core.Stop(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		fmt.Printf("termcp core stop: %v\n", err)
	}
}

func (a *App) WindowMinimise()       { runtime.WindowMinimise(a.ctx) }
func (a *App) WindowToggleMaximise() { runtime.WindowToggleMaximise(a.ctx) }
func (a *App) WindowClose()          { runtime.WindowHide(a.ctx) }

func defaultUILanguage() string {
	locale := strings.ToLower(os.Getenv("LANG"))
	if strings.HasPrefix(locale, "zh") {
		return "zh-CN"
	}
	return "en"
}

func (a *App) UILanguage() string {
	a.language.RLock()
	defer a.language.RUnlock()
	return a.uiLanguage
}

func (a *App) SetUILanguage(language string) string {
	if language != "en" {
		language = "zh-CN"
	}
	a.language.Lock()
	a.uiLanguage = language
	a.language.Unlock()
	a.refreshTray()
	return language
}

func (a *App) CoreStatus() model.CoreStatus {
	s := a.core.Status()
	return model.CoreStatus{Running: s.Running, Managed: s.Managed, Address: s.Address, State: s.State, Error: s.Error}
}

func (a *App) GetServiceStatus() (model.ServiceStatus, error) {
	status, err := a.service.Status()
	return model.ServiceStatus{
		Supported: status.Supported, Platform: status.Platform, Installed: status.Installed,
		Running: status.Running, Autostart: status.Autostart, PID: status.PID, Label: status.Label,
		Definition: status.Definition, LogPath: status.LogPath, Executable: status.Executable,
		Description: status.Description, DataDir: a.dataDir, Core: a.CoreStatus(),
	}, err
}

func (a *App) GetSnapshot() (model.Snapshot, error) {
	out := model.Snapshot{Core: a.CoreStatus(), Connections: []model.Connection{}, Sessions: []model.Session{}, History: []model.HistoryEntry{}, Forwards: []model.Forward{}, FetchedAt: time.Now().Format(time.RFC3339)}
	if !out.Core.Running {
		if out.Core.Error == "" {
			out.Core.Error = "Core 尚未就绪"
		}
		return out, nil
	}
	var connections struct {
		Connections []model.Connection `json:"connections"`
	}
	if err := a.bridge.GetJSON("/api/connections", &connections); err != nil {
		return out, err
	}
	out.Connections = connections.Connections
	var sessions struct {
		Sessions []model.Session `json:"sessions"`
	}
	if err := a.bridge.GetJSON("/api/sessions", &sessions); err != nil {
		return out, err
	}
	out.Sessions = sessions.Sessions
	for i := range out.Sessions {
		var shellList struct {
			Shells []model.Shell `json:"shells"`
		}
		if err := a.bridge.GetJSON("/api/sessions/"+url.PathEscape(out.Sessions[i].ID)+"/shells", &shellList); err == nil {
			out.Sessions[i].Shells = shellList.Shells
		}
		if out.Sessions[i].Shells == nil {
			out.Sessions[i].Shells = []model.Shell{}
		}
	}
	var history struct {
		Sessions []model.HistoryEntry `json:"sessions"`
	}
	if a.bridge.GetJSON("/api/history", &history) == nil {
		out.History = history.Sessions
	}
	var forwards struct {
		Forwards []model.Forward `json:"forwards"`
	}
	if a.bridge.GetJSON("/api/forwards", &forwards) == nil {
		out.Forwards = forwards.Forwards
	}
	return out, nil
}

func (a *App) CreateSession(req model.CreateSessionRequest) error {
	if strings.TrimSpace(req.Connection) == "" {
		return errors.New("请选择连接配置")
	}
	body := map[string]any{"ssh_config": req.Connection, "name": strings.TrimSpace(req.Name), "command": strings.TrimSpace(req.Command), "mode": "pty", "rows": 24, "cols": 100}
	return a.bridge.DoJSON(http.MethodPost, "/api/sessions", body, nil)
}

func (a *App) CreateShell(sessionID, name string) error {
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("缺少会话 ID")
	}
	body := map[string]any{"name": strings.TrimSpace(name), "mode": "pty", "rows": 24, "cols": 100}
	return a.bridge.DoJSON(http.MethodPost, "/api/sessions/"+url.PathEscape(sessionID)+"/shells", body, nil)
}

func (a *App) TerminateSession(sessionID string) error {
	return a.bridge.DoJSON(http.MethodPost, "/api/sessions/"+url.PathEscape(sessionID)+"/terminate", nil, nil)
}

func (a *App) RestartCore() error {
	return a.RestartLocalCore()
}

func (a *App) InstallCoreService(autostart bool) error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	defer a.refreshTray()
	if err := a.core.Stop(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	a.core.WaitDetached(5 * time.Second)
	if err := a.service.Install(autostart); err != nil {
		_ = a.core.Start()
		return err
	}
	if err := a.service.Start(); err != nil {
		_ = a.core.Start()
		return err
	}
	return a.core.Attach(15 * time.Second)
}

func (a *App) UninstallCoreService() error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	defer a.refreshTray()
	_ = a.core.Stop()
	if err := a.service.Uninstall(); err != nil {
		return err
	}
	a.core.WaitDetached(8 * time.Second)
	return a.core.Start()
}

func (a *App) StartLocalCore() error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	defer a.refreshTray()
	status, err := a.service.Status()
	if err != nil {
		return err
	}
	if !status.Installed {
		return a.core.Start()
	}
	if err := a.service.Start(); err != nil {
		return err
	}
	return a.core.Attach(15 * time.Second)
}

func (a *App) StopLocalCore() error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	defer a.refreshTray()
	status, err := a.service.Status()
	if err != nil {
		return err
	}
	if err := a.core.Stop(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if status.Installed {
		return a.service.Stop()
	}
	return nil
}

func (a *App) RestartLocalCore() error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	defer a.refreshTray()
	status, err := a.service.Status()
	if err != nil {
		return err
	}
	if err := a.core.Stop(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	if status.Installed {
		if err := a.service.Restart(); err != nil {
			return err
		}
		return a.core.Attach(15 * time.Second)
	}
	return a.core.Start()
}

func (a *App) SetCoreAutostart(enabled bool) error {
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	defer a.refreshTray()
	return a.service.SetAutostart(enabled)
}

func (a *App) API(input model.APIRequest) (model.APIResponse, error) {
	return a.bridge.API(input)
}

func (a *App) ChooseAndUploadFile(sessionID, remoteDirectory string) (model.UploadResult, error) {
	return a.bridge.ChooseAndUploadFile(a.ctx, sessionID, remoteDirectory)
}

func (a *App) SaveAPIResource(apiPath, suggestedName string) (string, error) {
	return a.bridge.SaveAPIResource(a.ctx, apiPath, suggestedName)
}

func (a *App) CoreWebSocketURL() string {
	return a.bridge.WebSocketURL()
}

func (a *App) SystemFonts() []string {
	return fonts.Families()
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
