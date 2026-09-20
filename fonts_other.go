//go:build !darwin && !linux && !windows

package main

func systemFontFamilies() []string { return nil }
