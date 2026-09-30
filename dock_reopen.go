package main

import "sync"

// windowReopenController bridges platform activation events to the Wails
// window lifecycle without retaining an App after Wails has shut down.
type windowReopenController struct {
	mu      sync.RWMutex
	handler func()
}

func (controller *windowReopenController) set(handler func()) {
	controller.mu.Lock()
	controller.handler = handler
	controller.mu.Unlock()
}

func (controller *windowReopenController) reopen() {
	controller.mu.RLock()
	handler := controller.handler
	controller.mu.RUnlock()
	if handler != nil {
		handler()
	}
}

var dockReopen windowReopenController

func setDockReopenHandler(handler func()) { dockReopen.set(handler) }
func triggerDockReopen()                  { dockReopen.reopen() }
