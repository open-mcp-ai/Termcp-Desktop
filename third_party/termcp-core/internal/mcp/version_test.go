package mcp

import (
	"encoding/json"
	"os"
	"regexp"
	"strings"
	"testing"
)

// TestServerInfoVersion verifies the build version passed to New is reported in
// the initialize response (serverInfo.version). main.go derives that value from
// the same metadata `termcp --version` prints, so MCP clients see the release
// tag instead of a hard-coded placeholder.
func TestServerInfoVersion(t *testing.T) {
	s := New(nil, nil, nil, nil, "1.2.3-test")
	res := callMCP(t, s, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1"},
	})
	var init struct {
		ServerInfo struct {
			Name    string `json:"name"`
			Version string `json:"version"`
		} `json:"serverInfo"`
	}
	if err := json.Unmarshal(res, &init); err != nil {
		t.Fatal(err)
	}
	if init.ServerInfo.Name != "termcp" {
		t.Errorf("serverInfo.name = %q, want termcp", init.ServerInfo.Name)
	}
	if init.ServerInfo.Version != "1.2.3-test" {
		t.Errorf("serverInfo.version = %q, want 1.2.3-test", init.ServerInfo.Version)
	}

	// An empty version must fall back to a non-empty default, never "".
	s2 := New(nil, nil, nil, nil, "")
	res2 := callMCP(t, s2, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1"},
	})
	if err := json.Unmarshal(res2, &init); err != nil {
		t.Fatal(err)
	}
	if init.ServerInfo.Version == "" {
		t.Fatal("serverInfo.version must not be empty when New is called with \"\"")
	}
}

// TestInstructionsDoNotMentionRemovedTools guards the initialize instructions
// against resurrecting tool names that no longer exist (the removed `history`
// tool regressed this once already).
func TestInstructionsDoNotMentionRemovedTools(t *testing.T) {
	s := New(nil, nil, nil, nil, "test")
	res := callMCP(t, s, "initialize", map[string]any{
		"protocolVersion": "2025-06-18",
		"capabilities":    map[string]any{},
		"clientInfo":      map[string]any{"name": "test", "version": "1"},
	})
	var init struct {
		Instructions string `json:"instructions"`
	}
	if err := json.Unmarshal(res, &init); err != nil {
		t.Fatal(err)
	}
	if init.Instructions == "" {
		t.Fatal("initialize must carry server instructions")
	}
	for _, removed := range []string{"history(", "local_forward", "message_list", "file_chmod"} {
		if strings.Contains(init.Instructions, removed) {
			t.Errorf("instructions mention removed tool %q", removed)
		}
	}
}

// releaseHeading matches a final release heading in CHANGELOG.md. The date is
// part of the pattern on purpose: without it, `\b` after the patch number
// matches inside `## v0.2.8-rc.1 — ...` and the rc heading is read as 0.2.8.
var releaseHeading = regexp.MustCompile(`^## v(\d+\.\d+\.\d+) — \d{4}-\d{2}-\d{2}\s*$`)

// newestFinalRelease returns the first final release heading, skipping
// pre-releases (`## vX.Y.Z-rc.N`), which never move server.json.
func newestFinalRelease(changelog string) string {
	for _, line := range strings.Split(changelog, "\n") {
		if m := releaseHeading.FindStringSubmatch(line); m != nil {
			return m[1]
		}
	}
	return ""
}

// TestNewestFinalReleaseSkipsPreReleases pins the rc rule itself. Reverting the
// heading pattern to a bare `\d+\.\d+\.\d+\b` makes this test fail on the
// first case, which is how the disagreement with release_metadata.py came back
// the last time.
func TestNewestFinalReleaseSkipsPreReleases(t *testing.T) {
	const rcOnTop = "# Changelog\n\n## Unreleased\n\n## v0.2.8-rc.1 — 2026-10-15\n\n- note\n\n## v0.2.7 — 2026-10-09\n\n- note\n"
	tests := []struct {
		name      string
		changelog string
		want      string
	}{
		{"an rc on top does not become the manifest version", rcOnTop, "0.2.7"},
		{"a final release on top is the manifest version", strings.Replace(rcOnTop, "## v0.2.8-rc.1", "## v0.2.8", 1), "0.2.8"},
		{"an rc with no date is not a heading at all", "## v0.2.8-rc.1\n", ""},
		{"a heading with no date is not a release", "## v0.2.8\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := newestFinalRelease(tt.changelog); got != tt.want {
				t.Errorf("newestFinalRelease() = %q, want %q", got, tt.want)
			}
		})
	}
}

// TestServerJSONMatchesChangelog keeps the MCP Registry manifest in step with
// the release the changelog announces. Every other trace of the version is
// derived (ldflags, the module version, /api/version), but server.json is a
// checked-in file that only the publish workflow rewrites - from the tag, after
// the checkout, never back into the repo. So a stale one is caught by nobody:
// it sat at 0.2.0 through four releases. This is the cheap local check that
// closes that gap; it lives here because `go test ./internal/...` is what CI
// actually runs.
//
// The heading compared against is the newest FINAL release in the changelog,
// which is the version server.json must name. It is not necessarily the build
// under development: bumping the heading is part of cutting a release here, so
// the first `## vX.Y.Z` is the release being prepared or just shipped, and a
// commit after it (an unreleased fix) does not change what the manifest must
// say.
//
// An `## vX.Y.Z-rc.N` heading is an internal test build and must NOT move
// server.json - a pre-release is published to nobody (no registry entry, no
// `latest` move), so the manifest keeps naming the newest final release while
// an rc is being cut. Matching the heading with `\b` after the patch number
// would make this test read `v0.2.8-rc.1` as `0.2.8` and demand exactly the
// bump an rc is forbidden to make, while scripts/release_metadata.py (which
// pins the whole heading) would call the same file correct - two guards,
// opposite verdicts on one changelog. The rc suffix is therefore matched
// explicitly and skipped.
func TestServerJSONMatchesChangelog(t *testing.T) {
	changelog, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		// A test run outside the repository root (or a source tarball without the
		// docs) must not fail the suite: the manifest check is repo hygiene, not a
		// property of the package under test. TestSyncedDocsMatchSource skips for
		// the same reason.
		t.Skipf("read CHANGELOG.md: %v", err)
	}
	// The newest final release heading is the version server.json must name.
	version := newestFinalRelease(string(changelog))
	if version == "" {
		t.Fatal("CHANGELOG.md has no '## vX.Y.Z — YYYY-MM-DD' release heading")
	}

	raw, err := os.ReadFile("../../server.json")
	if err != nil {
		t.Skipf("read server.json: %v", err)
	}
	var manifest struct {
		Version  string `json:"version"`
		Packages []struct {
			Identifier string `json:"identifier"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatalf("parse server.json: %v", err)
	}
	if manifest.Version != version {
		t.Errorf("server.json version = %q, CHANGELOG announces %q (bump both when releasing)", manifest.Version, version)
	}
	// The OCI identifier must carry the same version: the publish workflow
	// rewrites it together with `version`, so one drifting alone means a
	// manifest pointing at an image tag that was never built.
	if len(manifest.Packages) == 0 {
		t.Fatal("server.json declares no packages")
	}
	want := ":" + version
	if got := manifest.Packages[0].Identifier; !strings.HasSuffix(got, want) {
		t.Errorf("server.json packages[0].identifier = %q, want it to end in %q", got, want)
	}
}

// TestUnreleasedDoesNotRepeatAPublishedEntry pins the mechanical half of the release
// process: an entry that already appears in a dated section must not sit in
// `## Unreleased` as well.
//
// This is the failure that happens when `make prepare-release` runs on the release
// branch and the commit never comes back to dev — dev keeps the notes it wrote and
// the dated section lives only on main, so the next release ships them twice. It has
// now happened twice (v0.2.7, then v0.2.8-rc.3), which is why it is a test and not
// another paragraph in docs/releasing.md. The doc already said it.
//
// Comparison is on the entry's opening text rather than the whole line: the dated
// section is a copy, and a copy can drift by a trailing character without becoming a
// different entry.
func TestUnreleasedDoesNotRepeatAPublishedEntry(t *testing.T) {
	changelog, err := os.ReadFile("../../CHANGELOG.md")
	if err != nil {
		t.Skipf("read CHANGELOG.md: %v", err)
	}
	text := string(changelog)

	const head = "## Unreleased"
	i := strings.Index(text, head)
	if i < 0 {
		t.Fatalf("CHANGELOG.md has no %q section", head)
	}
	rest := text[i+len(head):]
	end := strings.Index(rest, "\n## ")
	if end < 0 {
		t.Fatalf("the Unreleased section is never closed by another heading")
	}
	unreleased, published := rest[:end], rest[end:]

	fingerprint := func(line string) string {
		line = strings.TrimSpace(strings.TrimPrefix(line, "- "))
		if r := []rune(line); len(r) > 60 {
			return string(r[:60])
		}
		return line
	}

	seen := map[string]bool{}
	for _, line := range strings.Split(published, "\n") {
		if strings.HasPrefix(line, "- ") {
			seen[fingerprint(line)] = true
		}
	}
	for _, line := range strings.Split(unreleased, "\n") {
		if !strings.HasPrefix(line, "- ") {
			continue
		}
		if fp := fingerprint(line); seen[fp] {
			t.Errorf("this entry is already in a dated section, so the release commit never came back to dev:\n  %s", fp)
		}
	}
}
