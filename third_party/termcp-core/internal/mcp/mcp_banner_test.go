package mcp

import (
	"strings"
	"testing"
)

// A STDIO PEER WRITES ITS OWN NOISE ONTO THE SAME STREAM, and that noise can start
// with the byte a frame starts with. Both of these banners were measured on real
// servers, and both begin with '[' — the exact byte the frame scan used to trust.
//
// The failure was not "the frame is late". It was "the parse started in the wrong
// place", reported to the caller as "not yet" — for ever.
func TestRecvFindsTheFrameBehindABannerStartingWithABracket(t *testing.T) {
	banners := []struct {
		name   string
		banner string
	}{
		{
			// chrome-devtools-mcp 1.12.0, after notifications/initialized.
			name:   "chrome-devtools-mcp",
			banner: `[chrome-devtools-mcp] The connecting client did not negotiate the MCP roots capability.`,
		},
		{
			// @modelcontextprotocol/server-filesystem 2.0.0, same place.
			name:   "server-filesystem",
			banner: `Client does not support MCP Roots, using allowed directories set from server args: [ 'C:\tc' ]`,
		},
	}
	for _, b := range banners {
		t.Run(b.name, func(t *testing.T) {
			s := newTestServer(t)
			h := openPipe(t, s, `sh -c 'printf `+b.banner+`\n; `+
				`printf "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"ok\":true}}\n"'`)
			waitForShellToExit(t, s, shellIDFromHandle(t, s, h))
			defer termText(t, s, map[string]any{"cmd": "close " + h})

			got := termText(t, s, map[string]any{"cmd": "read " + h + " --frame"})
			if strings.Contains(got, "no complete frame") {
				t.Fatalf("the banner stopped the scan:\n%s", got)
			}
			if !strings.Contains(got, `"ok":true`) {
				t.Errorf("the frame behind the banner was not delivered:\n%s", got)
			}
			// And the banner itself must not be handed back as if it were the frame.
			if strings.Contains(got, "chrome-devtools-mcp]") || strings.Contains(got, "server args:") {
				t.Errorf("the banner was returned as the frame:\n%s", got)
			}
		})
	}
}

// THE CURSOR MUST ADVANCE OVER THE NOISE, not stall on it. A recv that returned the
// frame but left the position inside the banner would deliver the same frame again.
func TestRecvAdvancesPastTheBannerToTheNextFrame(t *testing.T) {
	s := newTestServer(t)
	h := openPipe(t, s, `sh -c 'printf "[peer] starting\n"; `+
		`printf "{\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"n\":1}}\n"; `+
		`printf "{\"jsonrpc\":\"2.0\",\"id\":2,\"result\":{\"n\":2}}\n"'`)
	waitForShellToExit(t, s, shellIDFromHandle(t, s, h))
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	first := termText(t, s, map[string]any{"cmd": "read " + h + " --frame"})
	if !strings.Contains(first, `"n":1`) {
		t.Fatalf("the first frame was not delivered:\n%s", first)
	}
	second := termText(t, s, map[string]any{"cmd": "read " + h + " --frame"})
	if !strings.Contains(second, `"n":2`) {
		t.Errorf("the second frame was not delivered after the first:\n%s", second)
	}
}

// THE MESSAGE HAS TO SAY WHICH PROBLEM IT IS. "no complete frame yet" is the same
// sentence for a peer that is slow and a peer whose bytes will never parse, and only
// one of them is worth waiting on.
func TestRecvSaysWhenTheBytesInFrontAreNotAFrame(t *testing.T) {
	s := newTestServer(t)
	// A banner, then nothing: the buffer holds bytes that will never become a frame.
	h := openPipe(t, s, `sh -c 'printf "just a banner, no frame\n"; sleep 5'`)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	got := termText(t, s, map[string]any{"cmd": "read " + h + " --frame"})
	if !strings.Contains(got, "no complete frame") {
		t.Fatalf("expected the not-yet answer, got:\n%s", got)
	}
	if !strings.Contains(got, "not a frame") {
		t.Errorf("the reply did not name the unparseable prefix:\n%s", got)
	}
	if !strings.Contains(got, "just a banner") {
		t.Errorf("the reply did not quote what it found:\n%s", got)
	}
}

// THE SCAN TRIES EVERY CANDIDATE, not just the first bracket. A banner can hold
// several, and only one of them is the frame.
//
// THE FIXTURES ARE MESSAGES because that is what a frame is (#184): a value counts
// only when it carries `id` or `method`. This test used to feed it `{"a":1}` — legal
// JSON, not a message — and that is exactly the looseness the issue measured, so the
// fixtures moved rather than the assertion.
func TestFirstMCPMessageTriesEveryCandidate(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"banner then frame", `[warn] x [y] {"id":1}`, `{"id":1}`},
		{"an array frame is a frame too", `[warn] [{"id":1},{"method":"x"}]`, `[{"id":1},{"method":"x"}]`},
		{"noise with braces that never close", `{oops {"id":1}`, `{"id":1}`},
		{"nothing but noise", `[warn] nothing here`, ""},
		// A HALF-ARRIVED VALUE IS STILL "NOT YET", and saying anything else would be
		// wrong in the other direction: the caller should wait, and there is nothing
		// to explain. notAFrame is what decides that, so it is pinned here rather
		// than through a two-second wait on the door.
		{"a partial frame at the front", `{"id":`, ""},
		{"a partial frame at the end", `[warn] {"id":`, ""},
		{"the value comes first", `{"id":1} trailing`, `{"id":1}`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := firstMCPMessage(tc.in)
			if tc.want == "" {
				if ok {
					t.Errorf("firstMCPMessage(%q) = %q, want no value", tc.in, got)
				}
				return
			}
			if !ok {
				t.Fatalf("firstMCPMessage(%q) found nothing, want %q", tc.in, tc.want)
			}
			if got != tc.want {
				t.Errorf("firstMCPMessage(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// WHAT COUNTS AS A FRAME (#184). The criterion is the SHAPE of a JSON-RPC message,
// because the caller asked for a frame and "it parses as JSON" has an everyday wrong
// answer: PowerShell writes `InputObject[0]: some text`, and `[0]` is legal JSON.
//
// The line to hold: a value that is only JSON is SKIPPED, so the scan keeps looking
// rather than answering with something the caller will read as an answer.
func TestOnlyAMessageIsAFrame(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		// A reply carries id, a notification carries method, a batch is a list.
		{"a reply", `{"jsonrpc":"2.0","id":2,"result":{}}`, `{"jsonrpc":"2.0","id":2,"result":{}}`},
		{"a notification", `{"jsonrpc":"2.0","method":"Page.loadEventFired"}`, `{"jsonrpc":"2.0","method":"Page.loadEventFired"}`},
		{"a batch", `[{"id":1},{"method":"x"}]`, `[{"id":1},{"method":"x"}]`},
		// 1.0 has no jsonrpc field, and refusing a peer over a missing field would turn
		// a working channel into an unreadable one.
		{"a 1.0 reply", `{"id":1,"result":1}`, `{"id":1,"result":1}`},
		{"the reporter's line", `InputObject[0]: some text`, ""},
		{"a bare zero in brackets", `[0]`, ""},
		{"an object with no id and no method", `{"a":1}`, ""},
		// A CONTAINER THAT IS NOT A BATCH IS NOT SKIPPED WHOLESALE, and the answer is
		// the message inside it. That is not a loophole: the cursor advances only past
		// the bytes RETURNED, so `[0]` stays in the buffer and the next read sees it
		// again — nothing is lost, and a real message is not withheld because it arrived
		// inside a malformed wrapper.
		{"a batch holding a non-message", `[{"id":1},[0]]`, `{"id":1}`},
		{"json null", `null`, ""},
		{"a string", `"hello"`, ""},
		{"a number", `42`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := firstMCPMessage(tc.in)
			if tc.want == "" {
				if ok {
					t.Fatalf("firstMCPMessage(%q) = %q; that is not a JSON-RPC message", tc.in, got)
				}
				return
			}
			if !ok || got != tc.want {
				t.Fatalf("firstMCPMessage(%q) = %q, %v; want %q", tc.in, got, ok, tc.want)
			}
		})
	}
}

// AND THE SCAN KEEPS LOOKING PAST IT. Skipping the candidate is what makes the
// criterion usable: the real reply is usually BEHIND the line that looked like JSON,
// and answering "not yet" while it sits in the buffer is the other half of the bug.
func TestAFrameBehindANonMessageIsStillFound(t *testing.T) {
	raw := "InputObject[0]: some text\n" + `{"jsonrpc":"2.0","id":7,"result":{"ok":true}}` + "\n"
	got, ok := firstMCPMessage(raw)
	if !ok {
		t.Fatal("the reply behind the bracketed noise was not found")
	}
	if want := `{"jsonrpc":"2.0","id":7,"result":{"ok":true}}`; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

// WHAT THE 204 SAYS. "no complete frame yet" is the same sentence for a peer that is
// slow and a peer whose bytes will never parse, and only one of them is worth waiting
// on — a caller that retries a banner retries for ever.
func TestNotAFrameNamesTheUnparseablePrefix(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"a banner", "just a banner, no frame\n", "just a banner, no frame"},
		{"only the first line is quoted", "banner line\nmore noise\n", "banner line"},
		{"a value that is merely unfinished", `{"jsonrpc":`, ""},
		{"an array that is merely unfinished", `[{"a":1},`, ""},
		{"nothing at all", "", ""},
		{"whitespace only", "   \n  ", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := notAFrame(tc.in)
			if tc.want == "" {
				if got != "" {
					t.Errorf("notAFrame(%q) = %q, want nothing", tc.in, got)
				}
				return
			}
			if !strings.Contains(got, tc.want) {
				t.Errorf("notAFrame(%q) = %q, want it to name %q", tc.in, got, tc.want)
			}
			if !strings.Contains(got, "not a frame") {
				t.Errorf("notAFrame(%q) = %q, which does not say what the problem is", tc.in, got)
			}
		})
	}
}
