//go:build darwin

package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa
int TermcpInstallDockReopenHandler(void);
*/
import "C"

func installDockReopenHandler() bool {
	return C.TermcpInstallDockReopenHandler() != 0
}

//export TermcpHandleDockReopen
func TermcpHandleDockReopen() {
	// Cocoa calls the delegate on its UI thread. Return to Cocoa immediately and
	// let Wails marshal the show/unminimise operations to the main thread.
	go triggerDockReopen()
}
