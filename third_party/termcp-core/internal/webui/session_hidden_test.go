package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestHiddenSessionsStayOffThePlates runs the real list composition and the real
// node attribution from the shipped assets under node.
//
// An extension's session is infrastructure: the extension layer holds one
// connection per host and serves its extensions through channels in it, so the
// session is reported in the extension column and nowhere else. What makes that
// work is a property of the record — the namespace — read in the one place the
// plates compose their list, and in the node readout that counts sessions per
// profile. Both are exercised here rather than pinned as strings, because the
// failure mode is a card that appears (or a count that lies), not a missing line.
func TestHiddenSessionsStayOffThePlates(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; cannot exercise the plate composition")
	}
	sess := readAssetLF(t, "static/js/sessions.js")
	hidden := between(t, sess, "/** Sessions a subsystem keeps for itself", "\n/* ---- sessions still being dialled")
	pending := between(t, sess, "/** The list every plate reads", "\n/** Stop a dial in progress")
	dialogs := readAssetLF(t, "static/js/dialogs.js")
	// sessionBelongsToNode and runningSessionsForNode are contiguous; the repaint
	// helper follows and needs the card layer, so the slice stops before it.
	nodeFn := between(t, dialogs, "function sessionBelongsToNode(", "\n/** Repaint the node cards")

	script := `
const fs = require('fs');
const vm = require('vm');

const src = fs.readFileSync(process.argv[2], 'utf8');
const sandbox = {};
vm.createContext(sandbox);
vm.runInContext(
  'var window = {};\n' +
  'function allShellWins() { return window.wins || []; }\n' +
  'function pendingSessionRecords() { return [{ id: "pending:box", _pendingConn: "box" }]; }\n' +
  src, sandbox);

let bad = 0;
function check(name, got, want) {
  if (JSON.stringify(got) !== JSON.stringify(want)) {
    console.log('FAIL ' + name + ': got ' + JSON.stringify(got) + ' want ' + JSON.stringify(want));
    bad++;
  } else console.log('PASS ' + name);
}

const ordinary = { id: 's1', name: 'box', status: 'running', ssh_config: 'prod' };
const extension = { id: 's2', name: 'ext:prod', status: 'running', ssh_config: 'prod', namespace: 'ext:prod' };
const archived = { id: 's3', name: 'ext:prod', status: 'exited', namespace: 'ext:prod' };

sandbox.snapshot = [ordinary, extension, archived];
vm.runInContext('window._lastSessionsSnapshot = snapshot;', sandbox);

// The plates read the composed list: the extension's session is absent from it,
// which is what keeps it off the session card AND off the archive card, and the
// pending dial is still there beside the ordinary session.
check('plates', vm.runInContext('sessionsWithPending().map(function (s) { return s.id; })', sandbox),
  ['s1', 'pending:box']);

// The node readout counts what the user can act on: a hidden session is not a
// session on that profile's card.
check('node sessions', vm.runInContext(
  'runningSessionsForNode("prod").map(function (s) { return s.id; })', sandbox), ['s1']);

// A session with no namespace is untouched by the rule: every session that
// existed before namespaces did keeps being listed, in the plates and on the
// node's card alike.
sandbox.snapshot = [{ id: 's9', name: 'plain', status: 'running', ssh_config: 'prod' }];
vm.runInContext('window._lastSessionsSnapshot = snapshot;', sandbox);
check('no namespace is not hidden', vm.runInContext(
  'sessionsWithPending().map(function (s) { return s.id; }).concat(runningSessionsForNode("prod").map(function (s) { return s.id; }))',
  sandbox), ['s9', 'pending:box', 's9']);

console.log(bad === 0 ? 'HIDDEN SESSIONS OK' : bad + ' hidden-session failure(s)');
process.exit(bad === 0 ? 0 : 1);
`
	dir := t.TempDir()
	predicatePath := filepath.Join(dir, "hidden_predicate.js")
	if err := os.WriteFile(predicatePath, []byte(hidden+"\n"+pending+"\n"+nodeFn), 0o600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(dir, "hidden_predicate_test.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, scriptPath, predicatePath).CombinedOutput()
	if err != nil {
		t.Fatalf("hidden-session composition failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "HIDDEN SESSIONS OK") {
		t.Fatalf("hidden-session probe did not pass:\n%s", out)
	}
}
