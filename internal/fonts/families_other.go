//go:build !darwin && !linux && !windows

package fonts

func families() []string { return nil }
