//go:build !darwin && !windows && !linux

package tray

type unsupportedTray struct{}

func newNativeTray([]byte, func(action)) nativeTray { return unsupportedTray{} }
func (unsupportedTray) Start() error                { return nil }
func (unsupportedTray) Update(State)                {}
func (unsupportedTray) Stop()                       {}
