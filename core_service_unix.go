//go:build !windows

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func runPlatformCoreService(start, stop func() error) error {
	if err := start(); err != nil {
		return err
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signals)
	<-signals
	return stop()
}
