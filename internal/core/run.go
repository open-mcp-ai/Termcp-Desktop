package core

// Run starts a local Core and blocks until the process is asked to stop. It is
// the entry point used when the desktop app is invoked as a system service.
func Run() error {
	service := New("127.0.0.1", 18765)
	return runPlatform(service.Start, service.Stop)
}
