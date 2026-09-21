package main

import (
	"embed"
	"fmt"
	"log/slog"
	"os"

	"github.com/open-mcp-ai/termcp/gui/internal/config"
	"github.com/open-mcp-ai/termcp/gui/internal/core"
	"github.com/open-mcp-ai/termcp/gui/internal/logging"
	"github.com/wailsapp/wails/v2"
	wailslogger "github.com/wailsapp/wails/v2/pkg/logger"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/trayicon.png
var trayIconPNG []byte

func main() {
	if err := configureTermcpDataDir(""); err != nil {
		_, _ = fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	component := "termcp-desktop"
	if hasArgument("--core-service") {
		component = "termcp-core-service"
	} else if _, ok := argumentValue("--system-service-action"); ok {
		component = "termcp-service-action"
	}
	logConfig := logging.Config{DataDir: config.DataDir(), Component: component, Level: slog.LevelDebug}
	if component == "termcp-desktop" {
		logConfig.Stderr = os.Stderr
	}
	logRuntime, err := logging.Configure(logConfig)
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "configure logging: %v\n", err)
		os.Exit(1)
	}
	slog.Info("process starting", "version", config.ProductVersion, "log_path", logRuntime.Path)
	runErr := run(logRuntime)
	if runErr != nil {
		slog.Error("process stopped with error", "error", logging.ErrorText(runErr))
	} else {
		slog.Info("process stopped")
	}
	if closeErr := logRuntime.Close(); closeErr != nil && runErr == nil {
		runErr = closeErr
		_, _ = fmt.Fprintf(os.Stderr, "close logging: %v\n", closeErr)
	}
	if runErr != nil {
		os.Exit(1)
	}
}

func run(logRuntime *logging.Runtime) error {
	if action, ok := argumentValue("--system-service-action"); ok {
		if err := config.RunAction(action, hasArgument("--autostart")); err != nil {
			return err
		}
		return nil
	}
	if hasArgument("--core-service") {
		if dataDir, ok := argumentValue("--core-data-dir"); ok {
			if err := configureTermcpDataDir(dataDir); err != nil {
				return err
			}
		}
		if err := core.Run(); err != nil {
			return err
		}
		return nil
	}
	app := NewApp()
	err := wails.Run(&options.App{
		Title:              config.ProductName,
		Width:              1360,
		Height:             860,
		MinWidth:           980,
		MinHeight:          640,
		DisableResize:      false,
		Frameless:          true,
		StartHidden:        false,
		HideWindowOnClose:  true,
		BackgroundColour:   &options.RGBA{R: 242, G: 244, B: 242, A: 1},
		AssetServer:        &assetserver.Options{Assets: assets},
		Logger:             logging.NewWailsLogger(logRuntime.Logger),
		LogLevel:           wailslogger.DEBUG,
		LogLevelProduction: wailslogger.DEBUG,
		OnStartup:          app.startup,
		OnShutdown:         app.shutdown,
		Bind:               []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "ai.openmcp.termcp.desktop",
			OnSecondInstanceLaunch: func(options.SecondInstanceData) {
				app.showWindow("")
			},
		},
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About:    &mac.AboutInfo{Title: config.ProductName, Message: "本机 termcp Core 管理端与 SSH 工作台"},
		},
	})
	return err
}

func configureTermcpDataDir(explicit string) error {
	_, err := config.ResolveDataDir(explicit)
	return err
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
