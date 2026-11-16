package mcp

import (
	"strings"
	"testing"
)

// TestLogNamesTheFailingStatement pins issues P1-2 and P1-3 from the rc3 report:
// a failed call was logged as "is_error=true error=TERM/1 200 OK", naming the
// statement that SUCCEEDED, and 2xx statuses appeared in error lines at all.
func TestLogNamesTheFailingStatement(t *testing.T) {
	cases := []struct {
		name     string
		preview  string
		wantCode string
		wantPart string
	}{
		{
			name: "second statement failed",
			preview: "TERM/1 200 OK\nPath: run a:1 echo hi\n\nhi\n\n" +
				"TERM/1 404 Not Found\nPath: read a:2\nError-Code: shell_not_found\n\nno such shell",
			wantCode: "shell_not_found",
			wantPart: "404 Not Found",
		},
		{
			name:     "first statement failed",
			preview:  "TERM/1 500 Internal Server Error\nError-Code: command_failed\n\nboom",
			wantCode: "command_failed",
			wantPart: "500",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sec := failingSection(tc.preview)
			if sec == "" {
				t.Fatalf("no failing section found in:\n%s", tc.preview)
			}
			if !strings.Contains(sec, tc.wantPart) {
				t.Errorf("failingSection = %q, want it to name %q", sec, tc.wantPart)
			}
			if !strings.Contains(sec, tc.wantCode) {
				t.Errorf("failingSection = %q, want it to carry %q", sec, tc.wantCode)
			}
		})
	}

	// All-2xx: no failing section, so the log must not invent one.
	ok := "TERM/1 200 OK\nPath: run a:1 echo hi\n\nhi\n\nTERM/1 204 No Content\nPath: close a:1"
	if sec := failingSection(ok); sec != "" {
		t.Errorf("a successful reply produced a failing section: %q", sec)
	}
}
