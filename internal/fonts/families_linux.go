//go:build linux

package fonts

import (
	"os/exec"
	"sort"
	"strings"
)

func families() []string {
	output, err := exec.Command("fc-list", "--format", "%{family}\n").Output()
	if err != nil {
		return nil
	}
	set := map[string]struct{}{}
	for _, line := range strings.Split(string(output), "\n") {
		for _, value := range strings.Split(line, ",") {
			if family := strings.TrimSpace(value); family != "" {
				set[family] = struct{}{}
			}
		}
	}
	fonts := make([]string, 0, len(set))
	for font := range set {
		fonts = append(fonts, font)
	}
	sort.Slice(fonts, func(i, j int) bool { return strings.ToLower(fonts[i]) < strings.ToLower(fonts[j]) })
	return fonts
}
