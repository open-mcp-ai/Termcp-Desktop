package main

import (
	"embed"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if err := configureTermcpDataDir(""); err != nil {
		log.Fatal(err)
	}
	if action, ok := argumentValue("--system-service-action"); ok {
		if err := runSystemServiceAction(action); err != nil {
			log.Fatal(err)
		}
		return
	}
	if hasArgument("--core-service") {
		if dataDir, ok := argumentValue("--core-data-dir"); ok {
			if err := configureTermcpDataDir(dataDir); err != nil {
				log.Fatal(err)
			}
		}
		if err := runCoreService(); err != nil {
			log.Fatal(err)
		}
		return
	}
	app := NewApp()
	err := wails.Run(&options.App{
		Title:             productName,
		Width:             1360,
		Height:            860,
		MinWidth:          980,
		MinHeight:         640,
		DisableResize:     false,
		Frameless:         true,
		StartHidden:       false,
		HideWindowOnClose: true,
		BackgroundColour:  &options.RGBA{R: 242, G: 244, B: 242, A: 1},
		AssetServer:       &assetserver.Options{Assets: assets},
		OnStartup:         app.startup,
		OnShutdown:        app.shutdown,
		Bind:              []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "ai.openmcp.termcp.desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				app.showWindow("")
			},
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About:    &mac.AboutInfo{Title: productName, Message: "本机 termcp Core 管理端与 SSH 工作台"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
}

func configureTermcpDataDir(explicit string) error {
	dataDir := strings.TrimSpace(explicit)
	if dataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("resolve user home for termcp data: %w", err)
		}
		dataDir = filepath.Join(home, ".termcp")
	}
	return os.Setenv("TERMCP_DATA_DIR", filepath.Clean(dataDir))
}

func argumentValue(name string) (string, bool) {
	for index, argument := range os.Args[1:] {
		if argument == name && index+2 <= len(os.Args[1:]) {
			return os.Args[index+2], true
		}
	}
	return "", false
}

func hasArgument(expected string) bool {
	for _, argument := range os.Args[1:] {
		if argument == expected {
			return true
		}
	}
	return false
}
