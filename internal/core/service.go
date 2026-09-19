package core

import (
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"
	"github.com/open-mcp-ai/termcp/internal/config"
	"github.com/open-mcp-ai/termcp/internal/forward"
	"github.com/open-mcp-ai/termcp/internal/history"
	mcpmod "github.com/open-mcp-ai/termcp/internal/mcp"
	"github.com/open-mcp-ai/termcp/internal/message"
	"github.com/open-mcp-ai/termcp/internal/session"
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
}

func New(host string, port int) *Service { return &Service{host: host, port: port, state: "stopped"} }
func (s *Service) BaseURL() string       { return fmt.Sprintf("http://%s:%d", s.host, s.port) }

func (s *Service) Status() Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Running: s.running, Managed: s.managed, Address: s.BaseURL(), State: s.state, Error: s.lastErr}
}

func (s *Service) Start() error {
	s.mu.Lock()
	if s.running {
		s.mu.Unlock()
		return nil
	}
	s.state, s.lastErr = "starting", ""
	s.mu.Unlock()

	if probe(s.BaseURL() + "/api/sessions") {
		s.setReady(false, nil)
		return nil
	}

	dataDir, err := config.DefaultDataDir()
	if err != nil {
		s.setReady(false, err)
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		s.setReady(false, err)
		return err
	}

	sshSrv := sshserver.New()
	if err := sshSrv.Start(); err != nil {
		s.setReady(false, err)
		return err
	}
	store := storage.New(dataDir)
	msgMgr := message.NewManager(store)
	historyMgr := history.New(store)
	if err := historyMgr.Load(); err != nil {
		sshSrv.Stop()
		s.setReady(false, err)
		return err
	}
	sessMgr := session.NewManager(msgMgr, store, sshSrv)
	sessMgr.SetHistory(historyMgr)
	if err := sessMgr.RestoreDead(); err != nil {
		fmt.Printf("restore dead sessions: %v\n", err)
	}
	sshStore := sshconfig.NewStore(dataDir)
	forwardMgr := forward.NewForwardManager()
	sessMgr.AddTerminateListener(func(sessionID string) { forwardMgr.CloseBySession(sessionID) })

	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	mux := http.NewServeMux()
	httpServer := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	mcpSrv := mcpmod.New(sessMgr, msgMgr, sshStore, forwardMgr, mcpserver.WithHTTPServer(httpServer))
	mcpSrv.SetHistory(historyMgr)
	mux.Handle("GET /sse", mcpSrv.SSEHandler())
	mux.Handle("POST /message", mcpSrv.MessageHandler())
	mux.Handle("/stream", mcpSrv.StreamableHTTPHandler())
	web := &webui.Handler{Sessions: sessMgr, History: historyMgr, SSH: sshStore, ForwardMgr: forwardMgr, NotifyMgr: mcpSrv.NotifyManager()}
	web.Register(mux)
	mcpSrv.SetUINotifier(web.BroadcastUINotify)

	s.mu.Lock()
	s.mcp, s.sessions, s.ssh, s.managed = mcpSrv, sessMgr, sshSrv, true
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
				sshSrv.Stop()
				s.setReady(false, startErr)
				return startErr
			}
		default:
		}
		time.Sleep(40 * time.Millisecond)
	}
	err = errors.New("Core 启动超时")
	_ = mcpSrv.Stop()
	sshSrv.Stop()
	s.setReady(false, err)
	return err
}

func (s *Service) Stop() error {
	s.mu.Lock()
	if !s.running {
		s.mu.Unlock()
		return nil
	}
	if !s.managed {
		s.running, s.state = false, "stopped"
		s.mu.Unlock()
		return nil
	}
	s.state = "stopping"
	mcpSrv, sessMgr, sshSrv := s.mcp, s.sessions, s.ssh
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
	s.mu.Lock()
	s.running, s.managed, s.state, s.mcp, s.sessions, s.ssh = false, false, "stopped", nil, nil, nil
	s.mu.Unlock()
	return err
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
