//go:build darwin

package main

import (
	"context"
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
	"time"
)

func systemFontFamilies() []string {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "system_profiler", "SPFontsDataType", "-json", "-detailLevel", "mini").Output()
	if err != nil {
		return nil
	}
	var payload struct {
		Fonts []struct {
			Enabled   string `json:"enabled"`
			Typefaces []struct {
				Family  string `json:"family"`
				Enabled string `json:"enabled"`
			} `json:"typefaces"`
		} `json:"SPFontsDataType"`
	}
	if json.Unmarshal(output, &payload) != nil {
		return nil
	}
	set := map[string]struct{}{}
	for _, font := range payload.Fonts {
		if font.Enabled == "no" {
			continue
		}
		for _, face := range font.Typefaces {
			family := strings.TrimSpace(face.Family)
			if family != "" && face.Enabled != "no" {
				set[family] = struct{}{}
			}
		}
	}
	return sortedFontNames(set)
}

func sortedFontNames(set map[string]struct{}) []string {
	fonts := make([]string, 0, len(set))
	for font := range set {
		fonts = append(fonts, font)
	}
	sort.Slice(fonts, func(i, j int) bool { return strings.ToLower(fonts[i]) < strings.ToLower(fonts[j]) })
	return fonts
}
