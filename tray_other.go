//go:build !darwin && !windows && !linux

package main

type unsupportedTray struct{}

func newNativeTray(func(trayAction)) nativeTray { return unsupportedTray{} }
func (unsupportedTray) Start() error            { return nil }
func (unsupportedTray) Update(trayState)        {}
func (unsupportedTray) Stop()                   {}
