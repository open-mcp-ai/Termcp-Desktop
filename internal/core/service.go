package core

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	termconfig "github.com/open-mcp-ai/termcp/config"
	termcore "github.com/open-mcp-ai/termcp/core"
	guiConfig "github.com/open-mcp-ai/termcp/gui/internal/config"
	appLogging "github.com/open-mcp-ai/termcp/gui/internal/logging"
)

const (
	startupTimeout = 8 * time.Second
	stopTimeout    = 3 * time.Second
)

type Status struct {
	Running bool
	Managed bool
	Address string
	State   string
	Error   string
}

// Service owns the Desktop process's view of one local Core. Managed means this
// process owns the Core and must stop it. An attachment never owns or starts a
// second Core.
type Service struct {
	opMu sync.Mutex

	mu      sync.RWMutex
	host    string
	port    int
	running bool
	managed bool
	state   string
	lastErr string
	active  *runtime
}

func New(host string, port int) *Service {
	return &Service{host: host, port: port, state: "stopped"}
}

func (s *Service) BaseURL() string {
	return fmt.Sprintf("http://%s:%d", s.host, s.port)
}

func (s *Service) Status() Status {
	s.mu.RLock()
	status := Status{
		Running: s.running,
		Managed: s.managed,
		Address: s.BaseURL(),
		State:   s.state,
		Error:   s.lastErr,
	}
	s.mu.RUnlock()
	return status
}

// Start creates the sole managed Core and its Desktop-owned HTTP listener.
// Port validation, listener failure, Core startup failure, and readiness
// failure all release the resources acquired by the earlier steps.
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

	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.mu.Lock()
	if s.running {
		managed := s.managed
		s.mu.Unlock()
		if managed {
			return nil
		}
		resultErr = errors.New("Core is attached to an existing service; stop the attachment before starting an embedded Core")
		return resultErr
	}
	stale := s.active
	s.active = nil
	s.managed = false
	s.state = "starting"
	s.lastErr = ""
	s.mu.Unlock()
	if stale != nil {
		_ = stale.stop("replace stale desktop Core")
	}

	rt, err := newRuntime(s.host, s.port)
	if err != nil {
		resultErr = err
		s.setFailure(err)
		return err
	}
	rt.onStop = s.markRuntimeStopped
	rt.start()
	s.mu.Lock()
	s.active = rt
	s.managed = true
	s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), startupTimeout)
	defer cancel()
	if err := rt.waitReady(ctx); err != nil {
		_ = rt.stop("startup failed")
		s.clearRuntime(rt)
		resultErr = err
		s.setFailure(err)
		return err
	}

	s.mu.Lock()
	if s.active != rt {
		s.mu.Unlock()
		_ = rt.stop("superseded while starting")
		resultErr = errors.New("Core startup was superseded")
		return resultErr
	}
	s.running = true
	s.managed = true
	s.state = "running"
	s.lastErr = ""
	s.mu.Unlock()
	return nil
}

// Attach validates an already running service. It never creates a listener or a
// Core and therefore cannot race the service over the data directory or port.
func (s *Service) Attach(timeout time.Duration) error {
	started := time.Now()
	slog.Debug("Core lifecycle started", "operation", "attach", "address", s.BaseURL(), "timeout", timeout)
	var resultErr error
	defer func() {
		if resultErr != nil {
			slog.Error("Core lifecycle failed", "operation", "attach", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds(), "error", appLogging.ErrorText(resultErr))
		} else {
			slog.Debug("Core lifecycle completed", "operation", "attach", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds())
		}
	}()

	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.mu.Lock()
	if s.running {
		managed := s.managed
		s.mu.Unlock()
		if !managed {
			return nil
		}
		resultErr = errors.New("an embedded Core is running; stop it before attaching to the system service")
		return resultErr
	}
	s.managed = false
	s.state = "starting"
	s.lastErr = ""
	s.mu.Unlock()

	if timeout <= 0 {
		timeout = startupTimeout
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := waitForReady(ctx, s.BaseURL(), expectedVersion()); err != nil {
		resultErr = fmt.Errorf("等待兼容的 Core 系统服务失败: %w", err)
		s.setFailure(resultErr)
		return resultErr
	}
	s.mu.Lock()
	s.running = true
	s.managed = false
	s.state = "running"
	s.lastErr = ""
	s.mu.Unlock()
	return nil
}

// Stop shuts down only a Core this process owns. A service attachment is
// detached from the GUI and left running.
func (s *Service) Stop() error {
	started := time.Now()
	slog.Debug("Core lifecycle started", "operation", "stop", "address", s.BaseURL())
	var resultErr error
	defer func() {
		if resultErr != nil {
			slog.Error("Core lifecycle failed", "operation", "stop", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds(), "error", appLogging.ErrorText(resultErr))
		} else {
			slog.Debug("Core lifecycle completed", "operation", "stop", "address", s.BaseURL(), "duration_ms", time.Since(started).Milliseconds())
		}
	}()

	s.opMu.Lock()
	defer s.opMu.Unlock()

	s.mu.Lock()
	running, managed, rt := s.running, s.managed, s.active
	if running || rt != nil {
		s.state = "stopping"
	}
	s.mu.Unlock()

	if !running && rt == nil {
		s.mu.Lock()
		s.managed = false
		s.state = "stopped"
		s.lastErr = ""
		s.mu.Unlock()
		return nil
	}
	if !managed {
		s.mu.Lock()
		s.running = false
		s.managed = false
		s.state = "stopped"
		s.lastErr = ""
		s.mu.Unlock()
		slog.Debug("Core attachment detached without stopping the service", "address", s.BaseURL())
		return nil
	}
	if rt != nil {
		if err := rt.stop("desktop stop"); err != nil {
			resultErr = err
		}
	}
	s.mu.Lock()
	if s.active == rt {
		s.active = nil
	}
	s.running = false
	s.managed = false
	if resultErr != nil {
		s.state = "error"
		s.lastErr = resultErr.Error()
	} else {
		s.state = "stopped"
		s.lastErr = ""
	}
	s.mu.Unlock()
	return resultErr
}

// WaitDetached waits for a service process to release the endpoint. It does not
// mutate ownership: callers invoke it after the service manager has asked the
// external process to stop.
func (s *Service) WaitDetached(timeout time.Duration) {
	s.mu.Lock()
	if s.managed {
		s.mu.Unlock()
		return
	}
	s.running = false
	s.state = "stopped"
	s.mu.Unlock()

	if timeout <= 0 {
		return
	}
	deadline := time.Now().Add(timeout)
	client := &http.Client{Timeout: 250 * time.Millisecond}
	for time.Now().Before(deadline) {
		resp, err := client.Get(s.BaseURL() + "/api/version")
		if err != nil {
			return
		}
		_ = resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
}

func (s *Service) setFailure(err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.running = false
	s.managed = false
	s.state = "error"
	s.lastErr = err.Error()
}

func (s *Service) clearRuntime(rt *runtime) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active == rt {
		s.active = nil
	}
}

func (s *Service) markRuntimeStopped(rt *runtime, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != rt {
		return
	}
	s.active = nil
	s.running = false
	s.managed = false
	s.state = "error"
	s.lastErr = err.Error()
}

type runtime struct {
	core      *termcore.Server
	http      *http.Server
	listener  net.Listener
	websocket *websocketRegistry
	serveDone chan struct{}
	serveErr  atomic.Value
	stopOnce  sync.Once
	stopErr   error
	onStop    func(*runtime, error)
}

func newRuntime(host string, port int) (*runtime, error) {
	dataDir := guiConfig.DataDir()
	if strings.TrimSpace(dataDir) == "" {
		return nil, errors.New("Core data directory is empty")
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return nil, fmt.Errorf("create Core data directory: %w", err)
	}

	cfg := termconfig.Default()
	cfg.DataDir = dataDir
	cfg.Host = host
	cfg.Port = port
	cfg.Headless = false
	// Do not inherit TERMCP_ASSETS_DIR or TERMCP_THEMES_DIR. A no_webui build must
	// not recover upstream pages from an external resource directory.
	cfg.AssetsDir = ""
	cfg.ThemesDir = ""
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("invalid Desktop Core configuration: %w", err)
	}

	server, err := termcore.New(termcore.Options{Config: cfg, Version: expectedVersion()})
	if err != nil {
		return nil, fmt.Errorf("construct Core v%s: %w", guiConfig.CoreVersion, err)
	}
	if err := server.Start(); err != nil {
		server.Close("embedded Core startup failed")
		return nil, fmt.Errorf("start Core v%s: %w", guiConfig.CoreVersion, err)
	}

	listener, err := net.Listen("tcp", server.Addr())
	if err != nil {
		server.Close("listener unavailable")
		return nil, fmt.Errorf("bind Core at %s: %w", server.Addr(), err)
	}

	registry := newWebsocketRegistry()
	httpServer := &http.Server{
		Addr:              server.Addr(),
		Handler:           appLogging.HTTPMiddleware(registry.middleware(server.Handler())),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}
	rt := &runtime{
		core:      server,
		http:      httpServer,
		listener:  listener,
		websocket: registry,
		serveDone: make(chan struct{}),
	}
	return rt, nil
}

func (r *runtime) start() {
	go func() {
		err := r.http.Serve(r.listener)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			r.serveErr.Store(err)
		}
		close(r.serveDone)
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			_ = r.stop("desktop HTTP listener failed")
			if r.onStop != nil {
				r.onStop(r, err)
			}
		}
	}()
}

func (r *runtime) waitReady(ctx context.Context) error {
	return waitForReady(ctx, "http://"+r.http.Addr, expectedVersion())
}

func (r *runtime) stop(reason string) error {
	r.stopOnce.Do(func() {
		r.websocket.closeAll()
		var errs []error
		httpCtx, cancelHTTP := context.WithTimeout(context.Background(), stopTimeout)
		if err := r.http.Shutdown(httpCtx); err != nil {
			slog.Warn("desktop HTTP graceful shutdown timed out; forcing active connections closed", "error", appLogging.ErrorText(err))
			if closeErr := r.http.Close(); closeErr != nil && !errors.Is(closeErr, http.ErrServerClosed) {
				errs = append(errs, fmt.Errorf("force close desktop HTTP server: %w", closeErr))
			} else if !errors.Is(err, context.DeadlineExceeded) {
				errs = append(errs, fmt.Errorf("shutdown desktop HTTP server: %w", err))
			}
		}
		cancelHTTP()

		coreDone := make(chan struct{})
		go func() {
			r.core.Close(reason)
			close(coreDone)
		}()
		coreCtx, cancelCore := context.WithTimeout(context.Background(), stopTimeout)
		select {
		case <-coreDone:
		case <-coreCtx.Done():
			errs = append(errs, fmt.Errorf("close Core resources: %w", coreCtx.Err()))
		}
		cancelCore()

		serveCtx, cancelServe := context.WithTimeout(context.Background(), stopTimeout)
		select {
		case <-r.serveDone:
		case <-serveCtx.Done():
			_ = r.listener.Close()
			select {
			case <-r.serveDone:
			case <-time.After(250 * time.Millisecond):
				errs = append(errs, errors.New("desktop HTTP server did not exit after close"))
			}
		}
		cancelServe()
		if err, _ := r.serveErr.Load().(error); err != nil {
			errs = append(errs, fmt.Errorf("desktop HTTP server: %w", err))
		}
		r.stopErr = errors.Join(errs...)
	})
	return r.stopErr
}

func expectedVersion() string {
	return "v" + guiConfig.CoreVersion
}

func waitForReady(ctx context.Context, baseURL, version string) error {
	var lastErr error
	for {
		lastErr = checkReady(ctx, baseURL, version)
		if lastErr == nil {
			return nil
		}
		if errors.Is(lastErr, errCoreVersionMismatch) {
			return lastErr
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("Core readiness failed: %w (last check: %v)", ctx.Err(), lastErr)
		case <-time.After(40 * time.Millisecond):
		}
	}
}

var errCoreVersionMismatch = errors.New("Core version mismatch")

func checkReady(ctx context.Context, baseURL, version string) error {
	var metadata struct {
		Version string `json:"version"`
	}
	status, body, err := getJSON(ctx, baseURL+"/api/version", &metadata)
	if err != nil {
		return err
	}
	if status != http.StatusOK {
		return fmt.Errorf("GET /api/version returned %d: %s", status, body)
	}
	if metadata.Version != version {
		return fmt.Errorf("%w: Core version is %q, Desktop requires %q", errCoreVersionMismatch, metadata.Version, version)
	}

	for _, path := range []string{
		"/api/connections",
		"/api/sessions",
		"/api/extensions",
		"/api/approvals",
		"/api/clients",
		"/api/activity",
	} {
		status, body, err := getJSON(ctx, baseURL+path, nil)
		if err != nil {
			return err
		}
		if status != http.StatusOK {
			return fmt.Errorf("GET %s returned %d: %s", path, status, body)
		}
	}

	status, body, err = getJSON(ctx, baseURL+"/api.md", nil)
	if err != nil {
		return err
	}
	if status != http.StatusOK || !strings.Contains(body, "TERM/1") {
		return fmt.Errorf("GET /api.md returned %d without the TERM/1 reference", status)
	}

	status, body, err = getJSON(ctx, baseURL+"/api/ui/ws", nil)
	if err != nil {
		return err
	}
	if status != http.StatusBadRequest && status != http.StatusUpgradeRequired {
		return fmt.Errorf("GET /api/ui/ws without Upgrade returned %d: %s", status, body)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/term", strings.NewReader(`{"cmd":""}`))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Termcp-Desktop-Ready", "1")
	var reply struct {
		Protocol string `json:"protocol"`
	}
	status, body, err = doJSON(req, &reply)
	if err != nil {
		return err
	}
	if status != http.StatusOK || reply.Protocol != "TERM/1" {
		return fmt.Errorf("POST /api/term returned %d, protocol %q: %s", status, reply.Protocol, body)
	}
	return nil
}

func getJSON(ctx context.Context, endpoint string, target any) (int, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, "", err
	}
	req.Header.Set("X-Termcp-Desktop-Ready", "1")
	return doJSON(req, target)
}

func doJSON(req *http.Request, target any) (int, string, error) {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return resp.StatusCode, "", err
	}
	body := strings.TrimSpace(string(data))
	if target != nil && resp.StatusCode >= 200 && resp.StatusCode < 300 && len(data) > 0 {
		if err := json.Unmarshal(data, target); err != nil {
			return resp.StatusCode, body, fmt.Errorf("decode %s: %w", req.URL.Path, err)
		}
	}
	return resp.StatusCode, body, nil
}

type websocketRegistry struct {
	mu    sync.Mutex
	conns map[net.Conn]struct{}
}

func newWebsocketRegistry() *websocketRegistry {
	return &websocketRegistry{conns: make(map[net.Conn]struct{})}
}

func (r *websocketRegistry) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(&trackingResponseWriter{ResponseWriter: w, registry: r}, req)
	})
}

func (r *websocketRegistry) add(conn net.Conn) net.Conn {
	tracked := &trackedConn{Conn: conn, registry: r}
	r.mu.Lock()
	r.conns[tracked] = struct{}{}
	r.mu.Unlock()
	return tracked
}

func (r *websocketRegistry) remove(conn net.Conn) {
	r.mu.Lock()
	delete(r.conns, conn)
	r.mu.Unlock()
}

func (r *websocketRegistry) closeAll() {
	r.mu.Lock()
	connections := make([]net.Conn, 0, len(r.conns))
	for conn := range r.conns {
		connections = append(connections, conn)
	}
	r.conns = make(map[net.Conn]struct{})
	r.mu.Unlock()
	for _, conn := range connections {
		_ = conn.Close()
	}
}

type trackingResponseWriter struct {
	http.ResponseWriter
	registry *websocketRegistry
}

func (w *trackingResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *trackingResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("HTTP connection does not support hijacking")
	}
	conn, readWriter, err := hijacker.Hijack()
	if err != nil {
		return nil, nil, err
	}
	return w.registry.add(conn), readWriter, nil
}

func (w *trackingResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *trackingResponseWriter) ReadFrom(reader io.Reader) (int64, error) {
	if source, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		return source.ReadFrom(reader)
	}
	return io.Copy(struct{ io.Writer }{w}, reader)
}

func (w *trackingResponseWriter) Push(target string, options *http.PushOptions) error {
	if pusher, ok := w.ResponseWriter.(http.Pusher); ok {
		return pusher.Push(target, options)
	}
	return http.ErrNotSupported
}

type trackedConn struct {
	net.Conn
	registry *websocketRegistry
	once     sync.Once
}

func (c *trackedConn) Close() error {
	err := c.Conn.Close()
	c.once.Do(func() { c.registry.remove(c) })
	return err
}
