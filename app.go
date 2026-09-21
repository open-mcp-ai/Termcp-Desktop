package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/open-mcp-ai/termcp/gui/internal/bridge"
	"github.com/open-mcp-ai/termcp/gui/internal/config"
	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/fonts"
	"github.com/open-mcp-ai/termcp/gui/internal/logging"
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

var interfaceSequence atomic.Uint64

func beginInterface(name string, attributes ...any) func(error) {
	requestID := interfaceSequence.Add(1)
	started := time.Now()
	fields := append([]any{"interface", name, "request_id", requestID}, attributes...)
	slog.Debug("interface started", fields...)
	return func(err error) {
		fields := append([]any{"interface", name, "request_id", requestID, "duration_ms", time.Since(started).Milliseconds()}, attributes...)
		if err != nil {
			slog.Error("interface failed", append(fields, "error", logging.ErrorText(err))...)
			return
		}
		slog.Debug("interface completed", fields...)
	}
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
	done := beginInterface("startup")
	var startupErr error
	defer func() { done(startupErr) }()
	a.ctx = ctx
	status, statusErr := a.service.Status()
	if statusErr != nil {
		startupErr = statusErr
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
		startupErr = err
		runtime.LogErrorf(ctx, "Core 启动失败: %v", err)
	}
	if a.tray != nil {
		if err := a.tray.Start(); err != nil {
			startupErr = err
			runtime.LogErrorf(ctx, "系统托盘启动失败: %v", err)
		}
	}
}

func (a *App) shutdown(context.Context) {
	done := beginInterface("shutdown")
	var shutdownErr error
	defer func() { done(shutdownErr) }()
	if a.tray != nil {
		a.tray.Stop()
	}
	if err := a.core.Stop(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		shutdownErr = err
		slog.Error("termcp Core stop failed", "error", logging.ErrorText(err))
	}
}

func (a *App) WindowMinimise() {
	done := beginInterface("WindowMinimise")
	defer done(nil)
	runtime.WindowMinimise(a.ctx)
}

func (a *App) WindowToggleMaximise() {
	done := beginInterface("WindowToggleMaximise")
	defer done(nil)
	runtime.WindowToggleMaximise(a.ctx)
}

func (a *App) WindowClose() {
	done := beginInterface("WindowClose")
	defer done(nil)
	runtime.WindowHide(a.ctx)
}

func defaultUILanguage() string {
	locale := strings.ToLower(os.Getenv("LANG"))
	if strings.HasPrefix(locale, "zh") {
		return "zh-CN"
	}
	return "en"
}

func (a *App) UILanguage() string {
	done := beginInterface("UILanguage")
	defer done(nil)
	a.language.RLock()
	defer a.language.RUnlock()
	return a.uiLanguage
}

func (a *App) SetUILanguage(language string) string {
	done := beginInterface("SetUILanguage", "language", language)
	defer done(nil)
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
	done := beginInterface("CoreStatus")
	defer done(nil)
	s := a.core.Status()
	return model.CoreStatus{Running: s.Running, Managed: s.Managed, Address: s.Address, State: s.State, Error: s.Error}
}

func (a *App) GetServiceStatus() (result model.ServiceStatus, err error) {
	done := beginInterface("GetServiceStatus")
	defer func() { done(err) }()
	status, err := a.service.Status()
	return model.ServiceStatus{
		Supported: status.Supported, Platform: status.Platform, Installed: status.Installed,
		Running: status.Running, Autostart: status.Autostart, PID: status.PID, Label: status.Label,
		Definition: status.Definition, LogPath: status.LogPath, Executable: status.Executable,
		Description: status.Description, DataDir: a.dataDir, Core: a.CoreStatus(),
	}, err
}

func (a *App) GetSnapshot() (out model.Snapshot, err error) {
	done := beginInterface("GetSnapshot")
	defer func() { done(err) }()
	out = model.Snapshot{Core: a.CoreStatus(), Connections: []model.Connection{}, Sessions: []model.Session{}, History: []model.HistoryEntry{}, Forwards: []model.Forward{}, FetchedAt: time.Now().Format(time.RFC3339)}
	if !out.Core.Running {
		if out.Core.Error == "" {
			out.Core.Error = "Core 尚未就绪"
		}
		return out, nil
	}
	var connections struct {
		Connections []model.Connection `json:"connections"`
	}
	if requestErr := a.bridge.GetJSON("/api/connections", &connections); requestErr != nil {
		return out, requestErr
	}
	out.Connections = connections.Connections
	var sessions struct {
		Sessions []model.Session `json:"sessions"`
	}
	if requestErr := a.bridge.GetJSON("/api/sessions", &sessions); requestErr != nil {
		return out, requestErr
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

func (a *App) CreateSession(req model.CreateSessionRequest) (err error) {
	done := beginInterface("CreateSession", "connection", req.Connection)
	defer func() { done(err) }()
	if strings.TrimSpace(req.Connection) == "" {
		return errors.New("请选择连接配置")
	}
	body := map[string]any{"ssh_config": req.Connection, "name": strings.TrimSpace(req.Name), "command": strings.TrimSpace(req.Command), "mode": "pty", "rows": 24, "cols": 100}
	return a.bridge.DoJSON(http.MethodPost, "/api/sessions", body, nil)
}

func (a *App) CreateShell(sessionID, name string) (err error) {
	done := beginInterface("CreateShell", "session_id", sessionID)
	defer func() { done(err) }()
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("缺少会话 ID")
	}
	body := map[string]any{"name": strings.TrimSpace(name), "mode": "pty", "rows": 24, "cols": 100}
	return a.bridge.DoJSON(http.MethodPost, "/api/sessions/"+url.PathEscape(sessionID)+"/shells", body, nil)
}

func (a *App) TerminateSession(sessionID string) (err error) {
	done := beginInterface("TerminateSession", "session_id", sessionID)
	defer func() { done(err) }()
	return a.bridge.DoJSON(http.MethodPost, "/api/sessions/"+url.PathEscape(sessionID)+"/terminate", nil, nil)
}

func (a *App) RestartCore() (err error) {
	done := beginInterface("RestartCore")
	defer func() { done(err) }()
	return a.RestartLocalCore()
}

func (a *App) InstallCoreService(autostart bool) (err error) {
	done := beginInterface("InstallCoreService", "autostart", autostart)
	defer func() { done(err) }()
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

func (a *App) UninstallCoreService() (err error) {
	done := beginInterface("UninstallCoreService")
	defer func() { done(err) }()
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

func (a *App) StartLocalCore() (err error) {
	done := beginInterface("StartLocalCore")
	defer func() { done(err) }()
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

func (a *App) StopLocalCore() (err error) {
	done := beginInterface("StopLocalCore")
	defer func() { done(err) }()
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

func (a *App) RestartLocalCore() (err error) {
	done := beginInterface("RestartLocalCore")
	defer func() { done(err) }()
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

func (a *App) SetCoreAutostart(enabled bool) (err error) {
	done := beginInterface("SetCoreAutostart", "enabled", enabled)
	defer func() { done(err) }()
	a.lifecycle.Lock()
	defer a.lifecycle.Unlock()
	defer a.refreshTray()
	return a.service.SetAutostart(enabled)
}

func (a *App) API(input model.APIRequest) (response model.APIResponse, err error) {
	done := beginInterface("API", "method", input.Method, "path", safeAPIPath(input.Path), "body_bytes", len(input.Body))
	defer func() { done(err) }()
	return a.bridge.API(input)
}

func (a *App) ChooseAndUploadFile(sessionID, remoteDirectory string) (result model.UploadResult, err error) {
	done := beginInterface("ChooseAndUploadFile", "session_id", sessionID, "remote_directory_set", strings.TrimSpace(remoteDirectory) != "")
	defer func() { done(err) }()
	return a.bridge.ChooseAndUploadFile(a.ctx, sessionID, remoteDirectory)
}

func (a *App) SaveAPIResource(apiPath, suggestedName string) (destination string, err error) {
	done := beginInterface("SaveAPIResource", "path", safeAPIPath(apiPath), "suggested_extension", filepath.Ext(suggestedName))
	defer func() { done(err) }()
	return a.bridge.SaveAPIResource(a.ctx, apiPath, suggestedName)
}

func (a *App) CoreWebSocketURL() string {
	done := beginInterface("CoreWebSocketURL")
	defer done(nil)
	return a.bridge.WebSocketURL()
}

func (a *App) SystemFonts() []string {
	done := beginInterface("SystemFonts")
	defer done(nil)
	return fonts.Families()
}

// LogFrontend is intentionally narrow: the browser can report an error and a
// short diagnostic string, but cannot inject arbitrary structured fields.
func (a *App) LogFrontend(level, message, details string) {
	done := beginInterface("LogFrontend", "level", level)
	defer done(nil)
	logging.LogFrontend(level, message, details)
}

func (a *App) showWindow(section string) {
	done := beginInterface("showWindow", "section", section)
	defer done(nil)
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

func safeAPIPath(raw string) string {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "invalid"
	}
	if parsed.RawQuery == "" {
		return parsed.Path
	}
	keys := make([]string, 0, len(parsed.Query()))
	for key := range parsed.Query() {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return parsed.Path + "?" + strings.Join(keys, "&")
}
