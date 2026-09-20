package main

import (
	"testing"

	"github.com/open-mcp-ai/termcp/gui/internal/systemservice"
)

type fakeSystemService struct {
	status         systemservice.Status
	installCalls   int
	startCalls     int
	stopCalls      int
	restartCalls   int
	uninstallCalls int
	autostartCalls int
}

func (f *fakeSystemService) Status() (systemservice.Status, error) { return f.status, nil }
func (f *fakeSystemService) Install(autostart bool) error {
	f.installCalls++
	f.status.Installed, f.status.Autostart = true, autostart
	return nil
}
func (f *fakeSystemService) Uninstall() error {
	f.uninstallCalls++
	f.status.Installed, f.status.Running, f.status.Autostart = false, false, false
	return nil
}
func (f *fakeSystemService) Start() error {
	f.startCalls++
	f.status.Running = true
	return nil
}
func (f *fakeSystemService) Stop() error {
	f.stopCalls++
	f.status.Running = false
	return nil
}
func (f *fakeSystemService) Restart() error {
	f.restartCalls++
	f.status.Running = true
	return nil
}
func (f *fakeSystemService) SetAutostart(enabled bool) error {
	f.autostartCalls++
	f.status.Autostart = enabled
	return nil
}

func TestServiceStatusAndAutostartBinding(t *testing.T) {
	fake := &fakeSystemService{status: systemservice.Status{
		Supported: true, Platform: "linux", Installed: true, Running: true,
		Autostart: true, PID: 42, Label: systemservice.Label, Description: "systemd 用户服务",
	}}
	app := newAppWithService(serviceForURL(t, "http://127.0.0.1:18765"), fake)
	status, err := app.GetServiceStatus()
	if err != nil {
		t.Fatal(err)
	}
	if !status.Supported || !status.Installed || !status.Running || status.Platform != "linux" || status.PID != 42 {
		t.Fatalf("unexpected service status: %+v", status)
	}
	if err := app.SetCoreAutostart(false); err != nil {
		t.Fatal(err)
	}
	if fake.autostartCalls != 1 || fake.status.Autostart {
		t.Fatalf("autostart was not updated: %+v", fake)
	}
}
