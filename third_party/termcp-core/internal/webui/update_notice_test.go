package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestUpdateNoticeComparesOnlyRealVersions runs the update-notice comparison
// under node.
//
// The page asks GitHub for the latest release and tells the person when theirs
// is older. The whole risk of that feature is the comparison: a build that is
// not a release (`dev-1a2b3c4`, `dev-1a2b3c4-dirty`, a bare `dev`) has no
// meaningful position in a release order, so comparing it numerically either
// announces an update on every local build or, worse, hides one. The rule under
// test is the one the issue asked for: only a real `vX.Y.Z` gets compared, and a
// local build says nothing at all.
func TestUpdateNoticeComparesOnlyRealVersions(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; cannot exercise the version comparison")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "update.js")
	if err := os.WriteFile(script, []byte(updateNoticeHarness), 0o600); err != nil {
		t.Fatal(err)
	}
	views := readAssetLF(t, "static/js/update-notice.js")
	if err := os.WriteFile(filepath.Join(dir, "update-notice.js"), []byte(views), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, filepath.Join(dir, "update-notice.js")).CombinedOutput()
	if err != nil {
		t.Fatalf("the update comparison failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "UPDATE OK") {
		t.Fatalf("the update harness did not report success:\n%s", out)
	}
}

const updateNoticeHarness = `
const assert = require('assert');
const fs = require('fs');
const vm = require('vm');

/* The module ends with a load-time call, so the globals it touches have to
   exist. Nothing is fetched here: without a version, checkForUpdate resolves
   immediately without a request. */
const context = {
  console, Promise, JSON, String, Number, RegExp, Array, Object, Math, isNaN, parseInt,
  document: { querySelector: () => null, getElementById: () => null },
  localStorage: { getItem: () => null, setItem: () => {} },
  fetch: () => { throw new Error('no network in this harness'); }
};
context.window = context;
vm.createContext(context);
vm.runInContext(fs.readFileSync(process.argv[2], 'utf8'), context);

const cmp = context.compareVersions;

/* --- the version shape decides whether a comparison is allowed at all ----- */

/* A release is vX.Y.Z. Everything else in this build (dev-<sha>, dev-<sha>-dirty,
   plain dev, an empty string, a random tag) is not comparable. */
const notReleases = ['dev', 'dev-1a2b3c4', 'dev-1a2b3c4-dirty', '', 'nightly', 'v1.2', 'v1.2.3.4-rc.1-x'];
for (const v of notReleases) {
  assert.strictEqual(cmp(v, 'v0.2.9'), null, v + ' must not be compared as a release');
  assert.strictEqual(cmp('v0.2.9', v), null, 'a local build as the candidate must not be compared');
}

/* --- real versions compare numerically, not lexically ------------------- */

assert.strictEqual(cmp('v0.2.7', 'v0.2.8'), -1, '0.2.7 is older than 0.2.8');
assert.strictEqual(cmp('v0.2.8', 'v0.2.7'), 1, '0.2.8 is newer than 0.2.7');
assert.strictEqual(cmp('v0.2.8', 'v0.2.8'), 0, 'the same version is not an update');
/* The lexical trap: "0.2.10" < "0.2.9" as strings. */
assert.strictEqual(cmp('v0.2.10', 'v0.2.9'), 1, '0.2.10 is newer than 0.2.9');
assert.strictEqual(cmp('v0.9.0', 'v0.10.0'), -1, '0.9.0 is older than 0.10.0');
assert.strictEqual(cmp('v1.0.0', 'v0.99.99'), 1, 'the major number dominates');
/* A leading v is optional on the release side, and surrounding space is ignored. */
assert.strictEqual(cmp('0.2.7', 'v0.2.8'), -1, 'the v prefix is optional');
assert.strictEqual(cmp(' v0.2.7 ', 'v0.2.8'), -1, 'stray whitespace is ignored');

/* --- what the page decides from a comparison --------------------------- */

/* The decision is a separate function so the comparison can stay pure: it says
   whether to show the notice, given what the server reports and what GitHub
   says the latest release is. */
const decide = context.updateNoticeFor;
// Returns the bare version for display: the notice reads "v0.2.8 is available",
// so the v is added by the copy, not carried in the value.
assert.strictEqual(decide('v0.2.7', 'v0.2.8'), '0.2.8', 'an older release is told about the newer one');
assert.strictEqual(decide('v0.2.8', 'v0.2.8'), null, 'the current release says nothing');
assert.strictEqual(decide('v0.2.8', 'v0.2.7'), null, 'a newer local release says nothing');
assert.strictEqual(decide('dev-1a2b3c4', 'v0.2.8'), null, 'a dev build says nothing');
assert.strictEqual(decide('dev-1a2b3c4-dirty', 'v0.2.8'), null, 'a dirty dev build says nothing');
assert.strictEqual(decide('v0.2.7', ''), null, 'a failed lookup says nothing');
assert.strictEqual(decide('v0.2.7', null), null, 'no lookup result says nothing');

/*
 * A pre-release is not a release for comparison purposes. Someone running
 * v0.2.8-rc.1 must not be told to upgrade to v0.2.7, which would be a downgrade
 * dressed as an update; and the final v0.2.8 should still reach them afterwards.
 */
assert.strictEqual(decide('v0.2.8-rc.1', 'v0.2.7'), null, 'an rc is never told to downgrade');
assert.strictEqual(decide('v0.2.8-rc.1', 'v0.2.8'), '0.2.8', 'an rc is told when the final lands');

console.log('UPDATE OK');
`
