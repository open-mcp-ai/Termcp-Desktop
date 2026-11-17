package mcp

import (
	"strings"
	"testing"
)

// ONE BANNER LINE MUST NOT BRICK A CHANNEL.
//
// A stdio peer writes its own logging to the same stdout as its JSON-RPC frames, so
// the buffer a recv looks at holds both. The scan for the frame's first byte took the
// first '{' OR '[' in that buffer, and a banner is free to contain a bracket — measured
// on chrome-devtools-mcp 1.12.0, whose banner starts `[chrome-devtools-mcp]` (issue
// #182):
//
//	[chrome-devtools-mcp] The connecting client did not negotiate the MCP roots ...
//	{"result":{"content":[{"type":"text","text":"## Pages\n1: ..."}]},"jsonrpc":"2.0","id":2}
//
// The decoder was handed the banner, the reply behind it was never reached, and because
// the recv cursor advances only by the frame that was RETURNED there was no way past it.
// Every later frame queued up behind a prefix that would never parse, and the 204 said
// "wait", which never helped.
func TestAPeerBannerWithABracketDoesNotHideTheFrame(t *testing.T) {
	banner := "[chrome-devtools-mcp] The connecting client did not negotiate the MCP roots capability. " +
		"Tools that read or write local files will be restricted to the OS temp directory.\n"
	frame := `{"result":{"content":[{"type":"text","text":"## Pages\n1: example"}]},"jsonrpc":"2.0","id":2}`

	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"banner with a bracket in front", banner + frame, frame},
		{"banner with a brace in front", "npm warn [something] {not json\n" + frame, frame},
		// A complete value before the frame wins, by design: the scan returns the first
		// value that decodes, and `{"a":1}` IS one. This case is the other shape — a
		// bracket that cannot start a value, in front of the frame.
		{"several unparseable candidates", "[[{ {not json\n" + frame, frame},
		{"noise between frames", frame + "\n" + banner + frame, frame},
		{"a frame on its own", frame, frame},
		{"a batch array", `[{"jsonrpc":"2.0","id":1}]`, `[{"jsonrpc":"2.0","id":1}]`},
		{"nested brackets in the frame itself", `{"id":1,"result":[[1,2],{"b":[3]}]}`, `{"id":1,"result":[[1,2],{"b":[3]}]}`},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := firstMCPMessage(c.raw)
			if !ok {
				t.Fatalf("firstMCPMessage found no frame in %q", c.raw)
			}
			if got != c.want {
				t.Fatalf("firstMCPMessage:\n got %q\nwant %q", got, c.want)
			}
		})
	}
}

// AN INCOMPLETE FRAME IS STILL INCOMPLETE. Trying every candidate must not turn
// "half a frame has arrived" into a successful read: the caller is told to come back,
// which is the one honest answer while the rest is in flight.
func TestAHalfArrivedFrameIsNotAFrame(t *testing.T) {
	for _, raw := range []string{
		"",
		"no brackets at all",
		`{"jsonrpc":"2.0","id":1`,
		`{"a":`,
		"[1, 2",
		"[chrome-devtools-mcp] banner only, no frame yet\n",
	} {
		if got, ok := firstMCPMessage(raw); ok {
			t.Fatalf("firstMCPMessage(%q) answered %q; nothing complete has arrived", raw, got)
		}
	}
}

// A FRAME IS RETURNED WITH THE NOISE STRIPPED, so the offset arithmetic in mcpRecvFrame
// (which adds strings.Index(body, frame) to the window start) still lands on the frame.
func TestTheFrameIsFoundAfterTheNoise(t *testing.T) {
	raw := "[peer] starting\n" + `{"jsonrpc":"2.0","id":7}` + "\n"
	frame, ok := firstMCPMessage(raw)
	if !ok {
		t.Fatal("no frame found")
	}
	if !strings.Contains(raw, frame) {
		t.Fatalf("the returned frame %q is not a substring of the buffer", frame)
	}
	if want := strings.Index(raw, `{"jsonrpc"`); want != strings.Index(raw, frame) {
		t.Fatalf("the frame starts at %d, want %d", strings.Index(raw, frame), want)
	}
}
