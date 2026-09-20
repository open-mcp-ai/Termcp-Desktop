package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/systemservice"
	"github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx        context.Context
	core       *corepkg.Service
	client     *http.Client
	transfer   *http.Client
	service    systemservice.Manager
	tray       *trayController
	dataDir    string
	lifecycle  sync.Mutex
	language   sync.RWMutex
	uiLanguage string
}

type CoreStatus struct {
	Running bool   `json:"running"`
	Managed bool   `json:"managed"`
	Address string `json:"address"`
	State   string `json:"state"`
	Error   string `json:"error,omitempty"`
}

type ServiceStatus struct {
	Supported   bool       `json:"supported"`
	Platform    string     `json:"platform"`
	Installed   bool       `json:"installed"`
	Running     bool       `json:"running"`
	Autostart   bool       `json:"autostart"`
	PID         int        `json:"pid,omitempty"`
	Label       string     `json:"label"`
	Definition  string     `json:"definition,omitempty"`
	LogPath     string     `json:"log_path,omitempty"`
	Executable  string     `json:"executable,omitempty"`
	Description string     `json:"description,omitempty"`
	DataDir     string     `json:"data_dir"`
	Core        CoreStatus `json:"core"`
}

type Connection struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Description string `json:"description,omitempty"`
	Host        string `json:"host,omitempty"`
	User        string `json:"user,omitempty"`
	Port        int    `json:"port,omitempty"`
}

type Shell struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Status   string `json:"status"`
	Mode     string `json:"mode"`
	ExitCode *int   `json:"exit_code,omitempty"`
}

type Session struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	Mode        string  `json:"mode"`
	SSHEndpoint string  `json:"ssh_endpoint,omitempty"`
	CreatedAt   string  `json:"created_at,omitempty"`
	UpdatedAt   string  `json:"updated_at,omitempty"`
	Shells      []Shell `json:"shells"`
}

type HistoryEntry struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Status      string   `json:"status"`
	SSHEndpoint string   `json:"ssh_endpoint,omitempty"`
	Reason      string   `json:"reason,omitempty"`
	UpdatedAt   string   `json:"updated_at,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type Forward struct {
	ForwardID string `json:"forward_id"`
	SessionID string `json:"session_id"`
	Direction string `json:"direction"`
	SSHConfig string `json:"ssh_config"`
	Listen    string `json:"listen_addr"`
	Target    string `json:"target_addr"`
	Status    string `json:"status"`
	CreatedAt string `json:"created_at"`
}

type Snapshot struct {
	Core        CoreStatus     `json:"core"`
	Connections []Connection   `json:"connections"`
	Sessions    []Session      `json:"sessions"`
	History     []HistoryEntry `json:"history"`
	Forwards    []Forward      `json:"forwards"`
	FetchedAt   string         `json:"fetched_at"`
}

type CreateSessionRequest struct {
	Connection string `json:"connection"`
	Name       string `json:"name"`
	Command    string `json:"command"`
}

func NewApp() *App {
	app := newApp(corepkg.New("127.0.0.1", 18765))
	app.tray = newTrayController(app)
	return app
}

func newApp(core *corepkg.Service) *App {
	executable, _ := os.Executable()
	return newAppWithService(core, systemservice.New(executable))
}

func newAppWithService(core *corepkg.Service, service systemservice.Manager) *App {
	dataDir := strings.TrimSpace(os.Getenv("TERMCP_DATA_DIR"))
	if dataDir == "" {
		if home, err := os.UserHomeDir(); err == nil {
			dataDir = filepath.Join(home, ".termcp")
		}
	}
	return &App{
		core:       core,
		client:     &http.Client{Timeout: 30 * time.Second},
		transfer:   &http.Client{},
		service:    service,
		dataDir:    dataDir,
		uiLanguage: defaultUILanguage(),
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

func (a *App) CoreStatus() CoreStatus {
	s := a.core.Status()
	return CoreStatus{Running: s.Running, Managed: s.Managed, Address: s.Address, State: s.State, Error: s.Error}
}

func (a *App) GetServiceStatus() (ServiceStatus, error) {
	status, err := a.service.Status()
	return ServiceStatus{
		Supported: status.Supported, Platform: status.Platform, Installed: status.Installed,
		Running: status.Running, Autostart: status.Autostart, PID: status.PID, Label: status.Label,
		Definition: status.Definition, LogPath: status.LogPath, Executable: status.Executable,
		Description: status.Description, DataDir: a.dataDir, Core: a.CoreStatus(),
	}, err
}

func (a *App) GetSnapshot() (Snapshot, error) {
	out := Snapshot{Core: a.CoreStatus(), Connections: []Connection{}, Sessions: []Session{}, History: []HistoryEntry{}, Forwards: []Forward{}, FetchedAt: time.Now().Format(time.RFC3339)}
	if !out.Core.Running {
		if out.Core.Error == "" {
			out.Core.Error = "Core 尚未就绪"
		}
		return out, nil
	}
	var connections struct {
		Connections []Connection `json:"connections"`
	}
	if err := a.getJSON("/api/connections", &connections); err != nil {
		return out, err
	}
	out.Connections = connections.Connections
	var sessions struct {
		Sessions []Session `json:"sessions"`
	}
	if err := a.getJSON("/api/sessions", &sessions); err != nil {
		return out, err
	}
	out.Sessions = sessions.Sessions
	for i := range out.Sessions {
		var shellList struct {
			Shells []Shell `json:"shells"`
		}
		if err := a.getJSON("/api/sessions/"+url.PathEscape(out.Sessions[i].ID)+"/shells", &shellList); err == nil {
			out.Sessions[i].Shells = shellList.Shells
		}
		if out.Sessions[i].Shells == nil {
			out.Sessions[i].Shells = []Shell{}
		}
	}
	var history struct {
		Sessions []HistoryEntry `json:"sessions"`
	}
	if a.getJSON("/api/history", &history) == nil {
		out.History = history.Sessions
	}
	var forwards struct {
		Forwards []Forward `json:"forwards"`
	}
	if a.getJSON("/api/forwards", &forwards) == nil {
		out.Forwards = forwards.Forwards
	}
	return out, nil
}

func (a *App) CreateSession(req CreateSessionRequest) error {
	if strings.TrimSpace(req.Connection) == "" {
		return errors.New("请选择连接配置")
	}
	body := map[string]any{"ssh_config": req.Connection, "name": strings.TrimSpace(req.Name), "command": strings.TrimSpace(req.Command), "mode": "pty", "rows": 24, "cols": 100}
	return a.doJSON(http.MethodPost, "/api/sessions", body, nil)
}

func (a *App) CreateShell(sessionID, name string) error {
	if strings.TrimSpace(sessionID) == "" {
		return errors.New("缺少会话 ID")
	}
	body := map[string]any{"name": strings.TrimSpace(name), "mode": "pty", "rows": 24, "cols": 100}
	return a.doJSON(http.MethodPost, "/api/sessions/"+url.PathEscape(sessionID)+"/shells", body, nil)
}

func (a *App) TerminateSession(sessionID string) error {
	return a.doJSON(http.MethodPost, "/api/sessions/"+url.PathEscape(sessionID)+"/terminate", nil, nil)
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
	if err := a.service.Install(autostart); err != nil {
		_ = a.core.Start()
		return err
	}
	if err := a.service.Start(); err != nil {
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

func (a *App) getJSON(path string, target any) error {
	return a.doJSON(http.MethodGet, path, nil, target)
}

func (a *App) doJSON(method, path string, body any, target any) error {
	var reader io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, a.core.BaseURL()+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := a.client.Do(req)
	if err != nil {
		return fmt.Errorf("Core 请求失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 16<<10))
		return fmt.Errorf("Core 返回 %s: %s", resp.Status, strings.TrimSpace(string(msg)))
	}
	if target != nil && resp.StatusCode != http.StatusNoContent {
		if err := json.NewDecoder(resp.Body).Decode(target); err != nil {
			return fmt.Errorf("解析 Core 响应: %w", err)
		}
	}
	return nil
}
