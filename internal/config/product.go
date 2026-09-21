package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	ProductName = "Termcp"
	ProjectName = "Termcp-Desktop"
)

// ProductVersion may be replaced at release build time with Go's -X linker flag.
var ProductVersion = "0.1.1"

// DataDir returns the resolved termcp data directory for this process.
func DataDir() string {
	if dir := strings.TrimSpace(os.Getenv("TERMCP_DATA_DIR")); dir != "" {
		return filepath.Clean(dir)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", ".termcp")
	}
	return filepath.Join(home, ".termcp")
}

// ResolveDataDir resolves the data directory (falling back to the user home)
// and exports it as TERMCP_DATA_DIR for the current process.
func ResolveDataDir(explicit string) (string, error) {
	dir := strings.TrimSpace(explicit)
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve user home for termcp data: %w", err)
		}
		dir = filepath.Join(home, ".termcp")
	}
	dir = filepath.Clean(dir)
	return dir, os.Setenv("TERMCP_DATA_DIR", dir)
}
