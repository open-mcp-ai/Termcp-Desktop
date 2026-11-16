package mcp

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/open-mcp-ai/termcp/internal/forward"
)

// TestSoakChannelAndForwardChurn drives rapid create/close across channels, forwards
// and readers under -race, looking for leaks and lifecycle races.
//
// The regression it guards against is `dropRecvCursor`: closing a channel leaked one
// map entry every time, and no single-shot test saw it because the leak is one entry
// per channel over the session's life.
func TestSoakChannelAndForwardChurn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak in short mode")
	}
	s := newTestServer(t)
	s.forwardMgr = forward.NewForwardManager()
	s.sessMgr.AddOnDeadHook(s.forwardMgr.CloseBySession)

	// Step 1: Rapid channel create & close (50 channels in a single session)
	primary := openPipe(t, s, "sleep 60")
	sid := strings.SplitN(primary, ":", 2)[0]
	defer termText(t, s, map[string]any{"cmd": "close " + primary})

	for i := 0; i < 40; i++ {
		// The channel has to OUTLIVE the reader registration: a command that finishes
		// in milliseconds makes `reader` a 409 by design (see
		// TestReaderRefusesAFinishedChannel), and a churn test that raced that would be
		// measuring the scheduler instead of the lifecycle.
		//
		// `--argv sleep 60`, NOT `--argv 'sleep 5'`: quoting groups words, it does not
		// form a command line, so the quoted form names one program called "sleep 5"
		// and the channel dies with `executable file not found` before the first
		// assertion. That version passed on a fast machine (the reader won the race
		// against a process that was already dead) and failed on all three CI runners.
		// 60s is the primary's lifetime: longer than the test can run, so it cannot
		// expire mid-loop.
		cmd := fmt.Sprintf("open %s --argv sleep 60", sid)
		out := termText(t, s, map[string]any{"cmd": cmd})
		h := firstHandle(out)
		if h == "" {
			t.Fatalf("channel %d opened no handle:\n%s", i, out)
		}
		// Register a reader on it, then drop it. Both are asserted: a churn test that
		// ignores the replies is measuring how fast the server refuses.
		reg := termText(t, s, map[string]any{"cmd": "reader " + h})
		if !strings.HasPrefix(reg, "TERM/1 200") {
			t.Fatalf("channel %d did not register a reader:\n%s", i, reg)
		}
		// The reader id is reported as "reader <id>". It must travel to --drop, or the
		// call is refused: `reader <handle> --drop <id>`. Ignoring the reply was what
		// let `--drop` with no value pass unnoticed in the previous draft.
		rid := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(reg), "reader"))
		for _, line := range strings.Split(reg, "\n") {
			if strings.HasPrefix(line, "reader ") {
				rid = strings.TrimSpace(strings.TrimPrefix(line, "reader "))
			}
		}
		if drop := termText(t, s, map[string]any{"cmd": "reader " + h + " --drop " + rid}); strings.HasPrefix(drop, "TERM/1 4") {
			t.Fatalf("channel %d refused --drop %s:\n%s", i, rid, drop)
		}
		if closed := termText(t, s, map[string]any{"cmd": "close " + h}); strings.HasPrefix(closed, "TERM/1 4") {
			t.Fatalf("channel %d was not closed:\n%s", i, closed)
		}
	}

	// Step 2: Concurrent forwards churn (10 concurrent forward create-then-close)
	var wg sync.WaitGroup
	errCh := make(chan error, 20)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			cmd := fmt.Sprintf("open %s forward --local-port 0 --remote-host 127.0.0.1 --remote-port 22", sid)
			out := termText(t, s, map[string]any{"cmd": cmd})
			if !strings.HasPrefix(out, "TERM/1 2") {
				errCh <- fmt.Errorf("open forward %d: %s", idx, out)
				return
			}
			fwdID := ""
			for _, line := range strings.Split(out, "\n") {
				if v, ok := strings.CutPrefix(line, "forward_id:"); ok {
					fwdID = strings.TrimSpace(v)
				}
			}
			if fwdID == "" {
				errCh <- fmt.Errorf("no forward_id in %s", out)
				return
			}
			// Close it immediately
			closeCmd := fmt.Sprintf("close %s forward %s", sid, fwdID)
			closeOut := termText(t, s, map[string]any{"cmd": closeCmd})
			if !strings.HasPrefix(closeOut, "TERM/1 2") {
				errCh <- fmt.Errorf("close forward %s: %s", fwdID, closeOut)
			}
		}(i)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Errorf("concurrent forward error: %v", err)
	}

	// Step 3: All forwards should be gone
	list := termText(t, s, map[string]any{"cmd": "ls " + sid + " forward"})
	if !strings.Contains(list, "forwards: []") {
		t.Errorf("forwards leaked after churn:\n%s", list)
	}
}

// TestHighVolumePtyReadAndGrep pumps high-throughput writes through a pty and reads
// with grep and limits, ensuring no deadlock between writer and buffer.
func TestHighVolumePtyReadAndGrep(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping soak in short mode")
	}
	s := newTestServer(t)
	h := openPty(t, s)
	defer termText(t, s, map[string]any{"cmd": "close " + h})

	// Run 100 iterations of fast output
	termText(t, s, map[string]any{"cmd": "run " + h + " $1", "args": []any{
		// POSIX while, not seq: this test runs on the Windows CI too.
		"i=0; while [ $i -lt 100 ]; do i=$((i+1)); echo burst-line-$i; done",
	}})

	// Give the shell a moment to produce
	time.Sleep(300 * time.Millisecond)

	// Read with grep-limit
	got := termText(t, s, map[string]any{"cmd": "read " + h + " --grep burst-line- --grep-limit 5"})
	if !strings.HasPrefix(got, "TERM/1 2") {
		t.Fatalf("grep read failed:\n%s", got)
	}
}

// TestReaderRefusesAFinishedChannel pins the 409.
//
// A channel whose `--argv` command has finished still holds its transcript, so reading
// it works — but attaching a reader to a closed buffer cannot. That used to answer 500
// "buffer: closed", which reads as a failure inside the server and hides the one thing
// the caller can still do.
func TestReaderRefusesAFinishedChannel(t *testing.T) {
	s := newTestServer(t)
	primary := openPipe(t, s, "sleep 30")
	sid := strings.SplitN(primary, ":", 2)[0]
	defer termText(t, s, map[string]any{"cmd": "close " + primary})

	out := termText(t, s, map[string]any{"cmd": "open " + sid + " --argv echo done"})
	h := firstHandle(out)
	if h == "" {
		t.Fatalf("no channel was opened:\n%s", out)
	}

	// Wait for the command to finish rather than sleeping a fixed time.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(termText(t, s, map[string]any{"cmd": "read " + h + " --from-line 1"}), "done") {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	got := termText(t, s, map[string]any{"cmd": "reader " + h})
	if !strings.HasPrefix(got, "TERM/1 409") {
		t.Fatalf("a finished channel did not answer 409:\n%s", got)
	}
	if !strings.Contains(got, "read") {
		t.Errorf("the refusal does not say that reading still works:\n%s", got)
	}
}
