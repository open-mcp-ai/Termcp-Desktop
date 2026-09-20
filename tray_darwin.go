//go:build darwin

package main

/*
#cgo CFLAGS: -fobjc-arc
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>

void termcp_tray_start(const void *icon, int icon_length);
void termcp_tray_update(const char *product, const char *core, const char *service,
                       const char *tooltip, const char *show_window,
                       const char *workspace, const char *manage_service,
                       const char *start_core, const char *stop_core,
                       const char *restart_core, const char *autostart_label,
                       const char *about, const char *quit,
                       int core_running, int service_supported,
                       int service_installed, int autostart);
void termcp_tray_stop(void);
*/
import "C"

import (
	"sync/atomic"
	"unsafe"
)

type darwinTray struct {
	dispatch func(trayAction)
}

var activeDarwinTray atomic.Pointer[darwinTray]

func newNativeTray(dispatch func(trayAction)) nativeTray {
	return &darwinTray{dispatch: dispatch}
}

func (t *darwinTray) Start() error {
	activeDarwinTray.Store(t)
	if len(trayIconPNG) == 0 {
		C.termcp_tray_start(nil, 0)
		return nil
	}
	C.termcp_tray_start(unsafe.Pointer(&trayIconPNG[0]), C.int(len(trayIconPNG)))
	return nil
}

func (t *darwinTray) Update(state trayState) {
	product := C.CString(state.ProductLine)
	core := C.CString(state.CoreLine)
	service := C.CString(state.ServiceLine)
	tooltip := C.CString(state.Tooltip)
	showWindow := C.CString(state.ShowWindowLabel)
	workspace := C.CString(state.WorkspaceLabel)
	manageService := C.CString(state.ServiceLabel)
	startCore := C.CString(state.StartCoreLabel)
	stopCore := C.CString(state.StopCoreLabel)
	restartCore := C.CString(state.RestartCoreLabel)
	autostart := C.CString(state.AutostartLabel)
	about := C.CString(state.AboutLabel)
	quit := C.CString(state.QuitLabel)
	defer C.free(unsafe.Pointer(product))
	defer C.free(unsafe.Pointer(core))
	defer C.free(unsafe.Pointer(service))
	defer C.free(unsafe.Pointer(tooltip))
	defer C.free(unsafe.Pointer(showWindow))
	defer C.free(unsafe.Pointer(workspace))
	defer C.free(unsafe.Pointer(manageService))
	defer C.free(unsafe.Pointer(startCore))
	defer C.free(unsafe.Pointer(stopCore))
	defer C.free(unsafe.Pointer(restartCore))
	defer C.free(unsafe.Pointer(autostart))
	defer C.free(unsafe.Pointer(about))
	defer C.free(unsafe.Pointer(quit))
	C.termcp_tray_update(product, core, service, tooltip, showWindow, workspace,
		manageService, startCore, stopCore, restartCore, autostart, about, quit,
		boolInt(state.CoreRunning), boolInt(state.ServiceSupported),
		boolInt(state.ServiceInstalled), boolInt(state.ServiceAutostart))
}

func (t *darwinTray) Stop() {
	activeDarwinTray.CompareAndSwap(t, nil)
	C.termcp_tray_stop()
}

func boolInt(value bool) C.int {
	if value {
		return 1
	}
	return 0
}

//export termcp_tray_action
func termcp_tray_action(action C.int) {
	tray := activeDarwinTray.Load()
	if tray != nil {
		tray.dispatch(trayAction(action))
	}
}
