package main

import (
	"embed"
	"log"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	if action, ok := argumentValue("--system-service-action"); ok {
		if err := runSystemServiceAction(action); err != nil {
			log.Fatal(err)
		}
		return
	}
	if hasArgument("--core-service") {
		if dataDir, ok := argumentValue("--core-data-dir"); ok {
			if err := os.Setenv("TERMCP_DATA_DIR", dataDir); err != nil {
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
		Title:            "termcp gui",
		Width:            1360,
		Height:           860,
		MinWidth:         980,
		MinHeight:        640,
		DisableResize:    false,
		Frameless:        true,
		StartHidden:      false,
		BackgroundColour: &options.RGBA{R: 242, G: 244, B: 242, A: 1},
		AssetServer:      &assetserver.Options{Assets: assets},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind:             []interface{}{app},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About:    &mac.AboutInfo{Title: "termcp gui", Message: "termcp Core 的桌面管理端与 SSH 工作台"},
		},
	})
	if err != nil {
		log.Fatal(err)
	}
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
