package mcp

import (
	"strings"
	"testing"
)

// TestTheGuideNeverPointsAtAnEmptyCapabilityHub.
//
// A first-time caller reported: the loudest instruction in the 4 KB guide was
// "`ls ext` CAPABILITY HUB — start here", and obeying it returned
// "(no extensions installed)". The recommendation has to be true on the instance
// answering, so the section is decided from the same live listing `ls ext` prints.
func TestTheGuideNeverPointsAtAnEmptyCapabilityHub(t *testing.T) {
	// No extensions installed: the empty instance, which is what a fresh install is.
	s := newTestServer(t)
	guide := termText(t, s, map[string]any{})

	if strings.Contains(guide, "CAPABILITY HUB") {
		t.Errorf("the guide still calls an empty extension list a capability hub / start point:\n%s", guide)
	}
	if !strings.Contains(guide, "ls host") {
		t.Errorf("the guide lost its pointer to the saved targets, which is what a caller with no extensions needs:\n%s", guide)
	}
	if !strings.Contains(guide, "None are installed") {
		t.Errorf("the guide does not say the extension list is empty, so `ls ext` still looks like a next step:\n%s", guide)
	}
}
