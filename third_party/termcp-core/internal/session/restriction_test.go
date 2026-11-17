package session

import (
	"strings"
	"testing"
)

// TestRestrictionAllowsTool pins the whitelist semantics an extension's trimmed
// session depends on: nothing listed = everything allowed, and the list is
// matched by exact tool name only.
func TestRestrictionAllowsTool(t *testing.T) {
	tests := []struct {
		name  string
		allow []string
		tool  string
		want  bool
	}{
		{"empty allow list is unrestricted", nil, "file_write", true},
		{"empty allow list is unrestricted for any tool", nil, "session_delete", true},
		{"listed tool is allowed", []string{"file_read", "file_stat"}, "file_read", true},
		{"unlisted tool is denied", []string{"file_read", "file_stat"}, "file_write", false},
		{"a prefix of a listed name is not the name", []string{"file_read"}, "file_reader_register", false},
		{"no fuzzy match on a near miss", []string{"shell_input"}, "shell_inputs", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRestriction(tt.allow, nil)
			if err != nil {
				t.Fatalf("NewRestriction: %v", err)
			}
			if got := r.AllowsTool(tt.tool); got != tt.want {
				t.Errorf("AllowsTool(%q) = %v, want %v", tt.tool, got, tt.want)
			}
		})
	}
}

// TestRestrictionAllowsPath pins the three matchers a path pattern can compile
// into, and the one boundary that a naive string prefix would get wrong.
func TestRestrictionAllowsPath(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		path    string
		want    bool
	}{
		// A tree entry grants its own path and its whole subtree.
		{"tree grants the entry itself", "~/skills/foo/", "~/skills/foo", true},
		{"tree grants a direct child", "~/skills/foo/", "~/skills/foo/SKILL.md", true},
		{"tree grants a nested child", "~/skills/foo/", "~/skills/foo/scripts/run.sh", true},
		{"tree keeps the trailing-slash form too", "~/skills/foo", "~/skills/foo/SKILL.md", true},
		{"tree denies a sibling directory", "~/skills/foo/", "~/skills/bar/SKILL.md", false},
		{"tree denies a path outside it", "~/skills/foo/", "/etc/passwd", false},
		// The boundary a plain strings.HasPrefix would miss: a granted directory
		// must not leak a directory whose name merely extends it.
		{"tree denies an adjacent longer directory name", "/srv/data", "/srv/database/secret", false},
		{"tree grants an exact match on the boundary", "/srv/data", "/srv/data", true},

		// Globs match the whole cleaned path, slash-based, and have no **.
		{"glob matches a bare filename", "*.md", "SKILL.md", true},
		{"glob does not cross a directory it does not name", "*.md", "~/skills/foo/SKILL.md", false},
		{"glob matches an explicitly named directory", "skills/*/*.md", "skills/foo/SKILL.md", true},
		{"glob with a character class", "skills/foo/[SB]KILL.md", "skills/foo/SKILL.md", true},

		// Regex is unanchored unless the author anchors it.
		{"regex matches when unanchored", "re:SKILL\\.md$", "~/skills/foo/SKILL.md", true},
		{"regex anchor excludes a nonconforming path", "re:^/srv/.*\\.md$", "~/skills/foo/SKILL.md", false},
		{"regex matches an anchored path", "re:^/srv/.*\\.md$", "/srv/data/x.md", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, err := NewRestriction(nil, []string{tt.pattern})
			if err != nil {
				t.Fatalf("NewRestriction(%q): %v", tt.pattern, err)
			}
			if got := r.AllowsPath(tt.path); got != tt.want {
				t.Errorf("AllowsPath(%q) = %v, want %v", tt.path, got, tt.want)
			}
		})
	}
}

// TestRestrictionMultiplePathRules pins that rules are alternatives: the first
// match grants, and entries that do not match still let a later one do the job.
func TestRestrictionMultiplePathRules(t *testing.T) {
	r, err := NewRestriction(nil, []string{"~/skills/foo/", "re:^/srv/logs/.*$"})
	if err != nil {
		t.Fatalf("NewRestriction: %v", err)
	}
	if !r.AllowsPath("~/skills/foo/SKILL.md") {
		t.Error("first rule should grant a skill path")
	}
	if !r.AllowsPath("/srv/logs/app.log") {
		t.Error("second rule should grant a log path")
	}
	if r.AllowsPath("/etc/passwd") {
		t.Error("a path under no rule should be denied")
	}
}

// TestRestrictionEmptyPathsUnrestricted pins the default an ordinary session
// relies on: a restriction built with no path rules grants everything, and
// blank-only entries do not turn that into a deny-all.
func TestRestrictionEmptyPathsUnrestricted(t *testing.T) {
	r, err := NewRestriction([]string{"file_read"}, []string{"  ", ""})
	if err != nil {
		t.Fatalf("NewRestriction: %v", err)
	}
	if !r.AllowsPath("/etc/shadow") {
		t.Error("blank patterns must not restrict paths")
	}
	if r.AllowsTool("file_write") {
		t.Error("the tool whitelist still applies")
	}
}

// TestNewRestrictionRejectsBadPatterns pins that a malformed pattern fails at
// build time and the error names the offending pattern, so a save is rejected
// with something actionable rather than misbehaving later.
func TestNewRestrictionRejectsBadPatterns(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		wantIn  string
	}{
		{"invalid regex", "re:^foo([", "invalid regex"},
		{"invalid glob", "skills/[a-", "invalid glob"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewRestriction(nil, []string{tt.pattern}); err == nil {
				t.Fatalf("pattern %q should have been rejected", tt.pattern)
			} else if !strings.Contains(err.Error(), tt.wantIn) {
				t.Errorf("error %q should mention %q", err, tt.wantIn)
			}
		})
	}
}

// TestRestrictionZeroValueUnrestricted pins the contract the session package
// itself depends on: a session that never asked for a restriction is fully
// privileged, so the field can stay zero-valued on every existing call site.
func TestRestrictionZeroValueUnrestricted(t *testing.T) {
	var r Restriction
	if !r.AllowsTool("file_delete") {
		t.Error("zero value should allow every tool")
	}
	if !r.AllowsPath("/etc/passwd") {
		t.Error("zero value should allow every path")
	}
}
