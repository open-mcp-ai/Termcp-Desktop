package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	guiConfig "github.com/open-mcp-ai/termcp/gui/internal/config"
	appLogging "github.com/open-mcp-ai/termcp/gui/internal/logging"
	"github.com/open-mcp-ai/termcp/internal/approval"
	"github.com/open-mcp-ai/termcp/internal/config"
	"github.com/open-mcp-ai/termcp/internal/forward"
	mcpmod "github.com/open-mcp-ai/termcp/internal/mcp"
	"github.com/open-mcp-ai/termcp/internal/message"
	"github.com/open-mcp-ai/termcp/internal/session"
	termcpsftp "github.com/open-mcp-ai/termcp/internal/sftp"
	"github.com/open-mcp-ai/termcp/internal/sshconfig"
	"github.com/open-mcp-ai/termcp/internal/sshserver"
	"github.com/open-mcp-ai/termcp/internal/storage"
	"github.com/open-mcp-ai/termcp/internal/webui"
)

type Status struct {
	Running bool
	Managed bool
	Address string
	State   string
	Error   string
}

type Service struct {
	mu       sync.RWMutex
	host     string
	port     int
	running  bool
	managed  bool
	state    string
	lastErr  string
	mcp      *mcpmod.Server
	sessions *session.Manager
	ssh      *sshserver.Server
	store    *storage.Store
}

func New(host string, port int) *Service { return &Service{host: host, port: port, state: "stopped"} }
func (s *Service) BaseURL() string       { return fmt.Sprintf("http://%s:%d", s.host, s.port) }

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Running: s.running, Managed: s.managed, Address: s.BaseURL(), State: s.state, Error: s.lastErr}
}

func (s *Service) Start() error {
	started := time.Now()
	slog.Debug("Core lifecycle started", "operation", "start", "address", s.BaseURL())
	var resultErr error
	defer func() {
		if resultErr != nil {
			slog.Error("Core lifecycle failed", "operation", "start", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds(), "error", appLogging.ErrorText(resultErr))
		} else {
			slog.Debug("Core lifecycle completed", "operation", "start", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds())
		}
	}()
	s.mu.RLock()
	running, managed := s.running, s.managed
	s.mu.RUnlock()
	if running && managed {
		return nil
	}
	if running && probe(s.BaseURL()+"/api/sessions") {
		return nil
	}

	// Either nothing is running, or we are attached to a service process that has
	// already gone away. Clear that stale attachment before deciding how to start.
	s.mu.Lock()
	s.running, s.managed = false, false
	s.state, s.lastErr = "starting", ""
	s.mu.Unlock()

	if probe(s.BaseURL() + "/api/sessions") {
		s.setReady(false, nil)
		return nil
	}

	dataDir, err := config.DefaultDataDir()
	if err != nil {
		resultErr = err
		s.setReady(false, err)
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		resultErr = err
		s.setReady(false, err)
		return err
	}

	sshSrv := sshserver.New()
	if err := sshSrv.Start(); err != nil {
		resultErr = err
		s.setReady(false, err)
		return err
	}
	store := storage.New(dataDir)
	msgMgr := message.NewManager(store)
	sessMgr := session.NewManager(msgMgr, store, sshSrv)
	if err := sessMgr.RestoreDead(); err != nil {
		slog.Warn("restore dead sessions failed", "error", appLogging.ErrorText(err))
	}
	sshStore := sshconfig.NewStore(dataDir)
	forwardMgr := forward.NewForwardManager()

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	mux := http.NewServeMux()
	httpServer := &http.Server{Addr: addr, Handler: appLogging.HTTPMiddleware(mux), ReadHeaderTimeout: 10 * time.Second}
	mcpSrv := mcpmod.New(sessMgr, msgMgr, sshStore, forwardMgr, "v"+guiConfig.ProductVersion, mcpmod.WithHTTPServer(httpServer))
	mux.Handle("GET /sse", mcpSrv.SSEHandler())
	mux.Handle("POST /message", mcpSrv.MessageHandler())
	mux.Handle("/stream", mcpSrv.StreamableHTTPHandler())
	web := &webui.Handler{
		Sessions: sessMgr, SSH: sshStore, ForwardMgr: forwardMgr, NotifyMgr: mcpSrv.NotifyManager(),
		Version: "v" + guiConfig.ProductVersion, StartedAt: started.UTC().Format(time.RFC3339Nano),
	}
	web.ExecuteOperation = mcpSrv.ExecuteApprovedOperation
	registerDesktopRoutes(mux, sessMgr)
	web.Register(mux)
	mcpSrv.SetUINotifier(web.BroadcastUINotify)
	sessMgr.AddApprovalListener(func(_ string, req approval.Request) {
		if req.ShellID != "" {
			mcpSrv.NotifyManager().OnApprovalChange(req.ShellID, "approval "+string(req.State))
		}
	})

	s.mu.Lock()
	s.mcp, s.sessions, s.ssh, s.store, s.managed = mcpSrv, sessMgr, sshSrv, store, true
	s.mu.Unlock()
	errCh := make(chan error, 1)
	go func() { errCh <- mcpSrv.Start(addr) }()
	deadline := time.Now().Add(4 * time.Second)
	for time.Now().Before(deadline) {
		if probe(s.BaseURL() + "/api/sessions") {
			s.setReady(true, nil)
			return nil
		}
		select {
		case startErr := <-errCh:
			if startErr != nil && !errors.Is(startErr, http.ErrServerClosed) {
				resultErr = startErr
				sshSrv.Stop()
				_ = store.Close()
				s.clearManagedResources()
				s.setReady(false, startErr)
				return startErr
			}
		default:
		}
		time.Sleep(40 * time.Millisecond)
	}
	err = errors.New("Core 启动超时")
	resultErr = err
	_ = mcpSrv.Stop()
	sshSrv.Stop()
	_ = store.Close()
	s.clearManagedResources()
	s.setReady(false, err)
	return err
}

func registerDesktopRoutes(mux *http.ServeMux, sessions *session.Manager) {
	mux.HandleFunc("GET /api/sessions/{id}/files/default-directory", func(w http.ResponseWriter, r *http.Request) {
		sess := sessions.Get(r.PathValue("id"))
		if sess == nil {
			writeDirectoryError(w, http.StatusNotFound, "session not found")
			return
		}
		var (
			directory string
			err       error
		)
		if sess.SSHEndpoint == "internal" {
			directory, err = os.UserHomeDir()
		} else if client := sess.SSHClient(); client != nil {
			var sftpClient *termcpsftp.Client
			sftpClient, err = termcpsftp.NewClient(client)
			if err == nil {
				defer sftpClient.Close()
				directory, err = sftpClient.Getwd()
			}
		} else {
			err = errors.New("session has no active SSH connection")
		}
		if err != nil {
			slog.Error("default file directory failed", "session", sess.ID, "error", appLogging.ErrorText(err))
			writeDirectoryError(w, http.StatusInternalServerError, err.Error())
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"directory": directory})
	})
}

func writeDirectoryError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": message})
}

// Attach waits for a Core that is owned by the operating system service manager.
// It never starts an in-process Core when the endpoint is unavailable.
func (s *Service) Attach(timeout time.Duration) error {
	started := time.Now()
	slog.Debug("Core lifecycle started", "operation", "attach", "address", s.BaseURL(), "timeout", timeout)
	s.mu.Lock()
	if s.running && !s.managed {
		s.mu.Unlock()
		return nil
	}
	s.state, s.lastErr = "starting", ""
	s.mu.Unlock()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if probe(s.BaseURL() + "/api/sessions") {
			s.setReady(false, nil)
			slog.Debug("Core lifecycle completed", "operation", "attach", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds())
			return nil
		}
		time.Sleep(80 * time.Millisecond)
	}
	err := errors.New("等待 Core 系统服务启动超时")
	s.setReady(false, err)
	slog.Error("Core lifecycle failed", "operation", "attach", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds(), "error", appLogging.ErrorText(err))
	return err
}

func (s *Service) Stop() error {
	started := time.Now()
	slog.Debug("Core lifecycle started", "operation", "stop", "address", s.BaseURL())
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		slog.Debug("Core lifecycle completed", "operation", "stop", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds(), "already_stopped", true)
		return nil
	}
	if !s.managed {
		s.running, s.state = false, "stopped"
		s.mu.Unlock()
		slog.Debug("Core lifecycle completed", "operation", "stop", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds(), "detached", true)
		return nil
	}
	s.state = "stopping"
	mcpSrv, sessMgr, sshSrv, store := s.mcp, s.sessions, s.ssh, s.store
	s.mu.Unlock()
	if sessMgr != nil {
		sessMgr.MarkAllDead()
	}
	if sshSrv != nil {
		sshSrv.Stop()
	}
	var err error
	if mcpSrv != nil {
		err = mcpSrv.Stop()
	}
	if errors.Is(err, http.ErrServerClosed) {
		err = nil
	}
	if store != nil {
		// Core keeps log.bin open for appends; release it before shutdown completes.
		if closeErr := store.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close Core output logs: %w", closeErr))
		}
	}
	s.clearManagedResources()
	s.mu.Lock()
	s.running, s.state = false, "stopped"
	s.mu.Unlock()
	if err != nil {
		slog.Error("Core lifecycle failed", "operation", "stop", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds(), "error", appLogging.ErrorText(err))
	} else {
		slog.Debug("Core lifecycle completed", "operation", "stop", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds())
	}
	return err
}

func (s *Service) clearManagedResources() {
	s.mu.Lock()
	s.mcp, s.sessions, s.ssh, s.store, s.managed = nil, nil, nil, nil, false
	s.mu.Unlock()
}

// WaitDetached drops a stale attachment and waits until no Core answers on the
// endpoint. It is used after removing the system service so that a freshly
// started Core does not race with the service process that is still shutting down.
func (s *Service) WaitDetached(timeout time.Duration) {
	s.mu.Lock()
	if s.managed {
		s.mu.Unlock()
		return
	}
	s.running, s.state = false, "stopped"
	s.mu.Unlock()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !probe(s.BaseURL() + "/api/sessions") {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func (s *Service) setReady(managed bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.running, s.managed, s.state, s.lastErr = false, false, "error", err.Error()
		return
	}
	s.running, s.managed, s.state, s.lastErr = true, managed, "running", ""
}

func probe(endpoint string) bool {
	client := &http.Client{Timeout: 250 * time.Millisecond}
	resp, err := client.Get(endpoint)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
