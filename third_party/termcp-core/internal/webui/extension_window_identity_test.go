package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestStatusCardsAreNotSessionWindows runs the real window lookups from
// shell-windows.js and the leave guard from dialogs.js under node.
//
// An extension's status card is a .shell-window with the terminal taken out, and
// it carries the extension's NAME in _sid so the tab bar can label it. That is
// the whole hazard: a name is the author's to choose under the same
// [A-Za-z0-9_-]{1,64} rule a 12-hex session id satisfies, so _sid alone cannot
// tell the two kinds of window apart. Every session path that resolves a window —
// raise, notify, feed output, close — goes through getShellWindowBySid, and
// without the guard an extension named after a copied session id is what those
// paths would find.
//
// The leave guard is the second half: its claim is that sessions keep running on
// the server, and a status card has no session behind it. Counting one would warn
// the reader about a session they never started.
func TestStatusCardsAreNotSessionWindows(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; cannot exercise the window lookups")
	}
	shell := readAssetLF(t, "static/js/shell-windows.js")
	dialogs := readAssetLF(t, "static/js/dialogs.js")

	script := `
const fs = require('fs');
const vm = require('vm');

const shell = fs.readFileSync(process.argv[2], 'utf8');
const dialogs = fs.readFileSync(process.argv[3], 'utf8');

/* The two functions under test, sliced out of the real modules, plus the one
   helper they lean on. A missing function fails loudly: a rename must not turn
   this into a test of nothing. */
function slice(src, name) {
  const at = src.indexOf('function ' + name + '(');
  if (at < 0) throw new Error(name + ' is gone from the module');
  const end = src.indexOf('\n}\n', at);
  if (end < 0) throw new Error('cannot delimit ' + name);
  return src.slice(at, end + 3);
}

const src = [
  slice(shell, 'getShellWindowBySid'),
  slice(dialogs, 'pageHasLiveState'),
  'function tCount(one, other, p) { return (p.count === 1 ? one : other) + \":\" + p.count; }',
  'function allShellWins() { return WINS; }',
  'function t(k) { return k; }'
].join('\n');

const sandbox = { WINS: [] };
vm.createContext(sandbox);
vm.runInContext(src, sandbox);

let bad = 0;
function check(name, got, want) {
  if (got !== want) {
    console.log('FAIL ' + name + ': got ' + JSON.stringify(got) + ' want ' + JSON.stringify(want));
    bad++;
  } else {
    console.log('PASS ' + name);
  }
}

/* A session window and a status card whose NAME equals that session's id. This is
   the collision the guard exists for, and it is legal input: names allow
   [A-Za-z0-9_-] and a session id is 12 lowercase hex characters. */
const SID = 'a1b2c3d4e5f6';
/* _parentSid is what a real session window carries (terminal-window.js sets it
   from the session id); the card deliberately has none. */
const sessWin = { _sid: SID, _parentSid: SID, _connName: 'host-a' };
const cardWin = { _sid: SID, _extName: SID };
sandbox.WINS = [cardWin, sessWin];
vm.runInContext('var found = getShellWindowBySid(' + JSON.stringify(SID) + ');', sandbox);
check('a session lookup skips the card and finds the session', sandbox.found === sessWin, true);

/* The card is still findable on its own identity — that is what the panel uses. */
vm.runInContext('var cardLookup = (function () { var w = allShellWins(); for (var i=0;i<w.length;i++) { if (w[i]._extName === ' + JSON.stringify(SID) + ') return w[i]; } return null; })();', sandbox);
check('the card is found by _extName', sandbox.cardLookup === cardWin, true);

/* A card alone, asked for by its name: no session window may be returned. */
sandbox.WINS = [cardWin];
vm.runInContext('var alone = getShellWindowBySid(' + JSON.stringify(SID) + ');', sandbox);
check('a card alone is not the session lookup answer', sandbox.alone, null);

/* The leave guard: only windows with a session or a dial behind them count. */
function live(wins) {
  sandbox.WINS = wins;
  return vm.runInContext('pageHasLiveState()', sandbox);
}
check('no windows, no warning', live([]), '');
check('one session window warns', live([sessWin]) !== '', true);
check('a card alone does not warn', live([cardWin]), '');
check('a dial in flight warns', live([{ _placeholder: true, _pendingConnName: 'host-a' }]) !== '', true);
check('two sessions warn about two', live([sessWin, { _sid: 'ffffffffffff', _parentSid: 'ffffffffffff', _connName: 'b' }]).indexOf(':2') > 0, true);
check('a card beside a session still counts one', live([cardWin, sessWin]).indexOf(':1') > 0, true);

console.log(bad === 0 ? 'IDENTITY OK' : bad + ' identity failure(s)');
process.exit(bad === 0 ? 0 : 1);
`
	tmp := t.TempDir()
	shellPath := filepath.Join(tmp, "shell-windows.js")
	if err := os.WriteFile(shellPath, []byte(shell), 0o600); err != nil {
		t.Fatal(err)
	}
	dialogsPath := filepath.Join(tmp, "dialogs.js")
	if err := os.WriteFile(dialogsPath, []byte(dialogs), 0o600); err != nil {
		t.Fatal(err)
	}
	scriptPath := filepath.Join(tmp, "identity_test.js")
	if err := os.WriteFile(scriptPath, []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, scriptPath, shellPath, dialogsPath).CombinedOutput()
	got := string(out)
	if err != nil {
		t.Fatalf("identity check failed: %v\n%s", err, got)
	}
	if !strings.Contains(got, "IDENTITY OK") {
		t.Fatalf("identity check did not report success:\n%s", got)
	}
	t.Logf("%s", strings.TrimSpace(got))
}
