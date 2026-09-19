package main

import corepkg "github.com/open-mcp-ai/termcp/gui/internal/core"

func runCoreService() error {
	core := corepkg.New("127.0.0.1", 18765)
	return runPlatformCoreService(core.Start, core.Stop)
}
