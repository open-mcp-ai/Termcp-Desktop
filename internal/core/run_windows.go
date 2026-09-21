//go:build windows

package core

import (
	"os"
	"os/signal"
	"syscall"

	appconfig "github.com/open-mcp-ai/termcp/gui/internal/config"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
)

type serviceHandler struct {
	start func() error
	stop  func() error
}

func runPlatform(start, stop func() error) error {
	isService, err := svc.IsWindowsService()
	if err != nil {
		return err
	}
	if !isService {
		if err := start(); err != nil {
			return err
		}
		signals := make(chan os.Signal, 1)
		signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(signals)
		<-signals
		return stop()
	}
	return svc.Run(appconfig.WindowsServiceName, &serviceHandler{start: start, stop: stop})
}

func (h *serviceHandler) Execute(_ []string, requests <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	logger, _ := eventlog.Open(appconfig.WindowsServiceName)
	if logger != nil {
		defer logger.Close()
	}
	statuses <- svc.Status{State: svc.StartPending, WaitHint: 5000}
	if err := h.start(); err != nil {
		if logger != nil {
			_ = logger.Error(1, "termcp Core 启动失败: "+err.Error())
		}
		return true, 1
	}
	if logger != nil {
		_ = logger.Info(2, "termcp Core 已启动")
	}
	statuses <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for request := range requests {
		switch request.Cmd {
		case svc.Interrogate:
			statuses <- request.CurrentStatus
		case svc.Stop, svc.Shutdown:
			statuses <- svc.Status{State: svc.StopPending, WaitHint: 5000}
			if err := h.stop(); err != nil {
				if logger != nil {
					_ = logger.Error(3, "termcp Core 停止失败: "+err.Error())
				}
				return true, 2
			}
			if logger != nil {
				_ = logger.Info(4, "termcp Core 已停止")
			}
			return false, 0
		}
	}
	return false, 0
}
