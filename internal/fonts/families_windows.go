//go:build windows

package fonts

import (
	"encoding/json"
	"os/exec"
	"sort"
	"strings"
)

func families() []string {
	command := `[System.Reflection.Assembly]::LoadWithPartialName('System.Drawing') | Out-Null; $f = New-Object System.Drawing.Text.InstalledFontCollection; @($f.Families | ForEach-Object { $_.Name } | Sort-Object -Unique) | ConvertTo-Json -Compress`
	output, err := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command).Output()
	if err != nil {
		return nil
	}
	var fonts []string
	if json.Unmarshal(output, &fonts) != nil {
		var single string
		if json.Unmarshal(output, &single) == nil && strings.TrimSpace(single) != "" {
			fonts = []string{single}
		}
	}
	sort.Slice(fonts, func(i, j int) bool { return strings.ToLower(fonts[i]) < strings.ToLower(fonts[j]) })
	return fonts
}
