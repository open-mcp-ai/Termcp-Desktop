package webui

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestNotificationSoundAndPermissionNeedAGesture runs util.js under node.
//
// The bug it encodes is an ordering one, not a missing call: both the
// notification permission prompt and the AudioContext used for the sound only
// work inside a user gesture, and the page only ever learns about a
// notification from a WebSocket frame — which is not one. Asking at arrival
// time therefore did nothing at all (permission stayed 'default' forever, so
// every notification was screen-only), and audio stayed silent.
//
// The test drives the real module: it loads all of util.js against a small DOM
// stub, then asserts the ask has been armed at load time, that it happens on
// the first gesture and only once, and that the sound is produced by a real
// (stubbed) AudioContext rather than merely not throwing.
func TestNotificationSoundAndPermissionNeedAGesture(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not on PATH; cannot exercise the notification sound")
	}
	dir := t.TempDir()
	script := filepath.Join(dir, "sound.js")
	if err := os.WriteFile(script, []byte(notificationSoundHarness), 0o600); err != nil {
		t.Fatal(err)
	}
	util := filepath.Join(dir, "util.js")
	if err := os.WriteFile(util, []byte(readAssetLF(t, "static/js/util.js")), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := exec.Command(node, script, util).CombinedOutput()
	if err != nil {
		t.Fatalf("the notification sound check failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "SOUND OK") {
		t.Fatalf("the sound harness did not report success:\n%s", out)
	}
}

// notificationSoundHarness drives util.js: a page stub for the module-level
// calls, and an AudioContext stub that records what was played.
const notificationSoundHarness = `
const assert = require('assert');
const fs = require('fs');
const vm = require('vm');

/* --- page stub ---------------------------------------------------------- */
const listeners = {};
function el() {
  const node = { children: [], textContent: '', title: '', className: '', id: '',
    classList: { add() {}, remove() {}, toggle() {} }, style: { setProperty() {}, removeProperty() {} },
    appendChild(c) { node.children.push(c); return c; },
    setAttribute() {}, getAttribute() { return null; }, addEventListener() {} };
  return node;
}
const document = {
  documentElement: el(),
  body: el(),
  addEventListener: (name, fn) => { (listeners[name] = listeners[name] || []).push(fn); },
  removeEventListener: (name, fn) => {
    listeners[name] = (listeners[name] || []).filter(f => f !== fn);
  },
  createElement: el,
  getElementById: () => null,
  querySelectorAll: () => []
};
const context = {
  console, Promise, Math, Date, JSON, String, Number, Array, Object, isNaN, isFinite, parseFloat, parseInt,
  document,
  window: { matchMedia: () => null, _lastSessionsSnapshot: [], addEventListener() {} },
  setTimeout, clearTimeout,
  getShellWindowBySid: () => null,
  findSessTile: () => null,
  getComputedStyle: () => ({ getPropertyValue: () => '' }),
  t: k => k
};
Object.assign(context.window, context);

/* --- notification + audio stubs ---------------------------------------- */
let permissionAsked = 0;
context.Notification = function (title, opts) { context._lastSystemNotification = { title, opts }; };context.Notification.permission = 'default';
context.Notification.requestPermission = () => {
  permissionAsked++;
  context.Notification.permission = 'granted';   // the browser records the answer too
  return Promise.resolve('granted');
};
// browserSystemNotify reads permission off window, which is the same object.
context.window.Notification = context.Notification;

const played = [];
/* A browser only starts an AudioContext that was created inside a user gesture.
   One created outside it is constructed 'suspended' and resume() from a non
   gesture cannot lift it, which is exactly why the page opens the context on the
   first click rather than when the first notification arrives. */
let gestureActive = false;
class AudioContextStub {
  constructor() {
    this.state = gestureActive ? 'running' : 'suspended';
    this.currentTime = 0; this.destination = {}; this.resumed = 0;
  }
  resume() {
    this.resumed++;
    if (gestureActive) this.state = 'running';
    return Promise.resolve();
  }
  createOscillator() {
    const osc = { type: '', frequency: { value: 0 }, connect() {}, start(t) { osc.startedAt = t; }, stop(t) { osc.stoppedAt = t; } };
    return osc;
  }
  createGain() {
    // Record each scheduled ramp so the envelope, not just the frequency, is checked.
    const gain = { gain: { setValueAtTime() {}, linearRampToValueAtTime(v, t) { played.push({ v, t }); } }, connect() {} };
    return gain;
  }
}
context.AudioContext = AudioContextStub;

vm.createContext(context);
vm.runInContext(fs.readFileSync(process.argv[2], 'utf8'), context);

const fail = [];
function eq(name, got, want) {
  if (got !== want) fail.push(name + ': got ' + JSON.stringify(got) + ', want ' + JSON.stringify(want));
}
const fire = (name) => (listeners[name] || []).slice().forEach(fn => fn({ type: name }));

/* Arming happens at load, before any notification can arrive — otherwise the
   first notification (the one a person actually needs) would be silent. */
eq('the gesture hook is armed at load', (listeners['pointerdown'] || []).length, 1);
eq('a keypress arms it too', (listeners['keydown'] || []).length, 1);
eq('no permission prompt has been shown yet', permissionAsked, 0);

/* Nothing a notification can do on its own: no gesture, no sound, no prompt. */
eq('a notification with no gesture cannot play', context.playNotifySound('info'), false);
eq('a notification with no gesture cannot prompt', permissionAsked, 0);

/* A real gesture. It must ask for permission and open the audio context, and
   it must unhook itself so a page full of clicks asks once. */
gestureActive = true;
fire('pointerdown');
gestureActive = false;
eq('the gesture asks for notification permission', permissionAsked, 1);
eq('the gesture unhooks the click listener', (listeners['pointerdown'] || []).length, 0);
eq('the gesture unhooks the key listener', (listeners['keydown'] || []).length, 0);

/* Now the sound works. */
played.length = 0;
eq('a notification after a gesture plays', context.playNotifySound('info'), true);
// Two notes; each has a ramp up and a ramp down, so the gain never clicks on.
eq('the sound is two notes with an envelope', played.length, 4);
const rising = played.filter(p => p.v > 0).map(p => p.v);
eq('both notes are audible', rising.length, 2);
assert.ok(rising.every(v => v > 0 && v < 0.5), 'the volume must stay modest');

/* A repeat click must not ask again. */
fire('keydown');
eq('a second gesture does not ask again', permissionAsked, 1);

/* The sound is attached to the notification the page actually renders —
   showUiNotify, the one function every ui_notify frame and approval event goes
   through. Asserting playNotifySound on its own would pass with the call site
   deleted, which is the bug this is meant to catch. */
played.length = 0;
context.showUiNotify({ title: 'build', message: 'done', level: 'success', duration_seconds: 0 });
eq('a rendered notification plays the sound', played.filter(p => p.v > 0).length, 2);

/* Permission is never requested from a notification: a WebSocket frame is not a
   gesture, so the prompt would be ignored and the permission refused silently.
   The ask belongs to the gesture path asserted above. */
context.Notification.permission = 'default';
const askedBefore = permissionAsked;
context.showUiNotify({ title: 'x', message: 'y', level: 'info', duration_seconds: 0 });
eq('a notification never opens a permission prompt', permissionAsked, askedBefore);

/* A granted permission delivers the system notification; a denied one is
   silent rather than an error. */
context.Notification.permission = 'granted';
context.browserSystemNotify({ title: 'x', message: 'y' });
eq('a granted notification reaches the system', context._lastSystemNotification.title, 'x');
context.Notification.permission = 'denied';
context._lastSystemNotification = null;
context.browserSystemNotify({ title: 'z', message: 'w' });
eq('a denied notification is skipped quietly', context._lastSystemNotification, null);

/* Level picks the pitch, and the exploratory read must not throw for a level
   the caller made up. */
played.length = 0;
context.playNotifySound('error');
const errorPitch = played.filter(p => p.v > 0).length;
eq('the error sound still has two notes', errorPitch, 2);
assert.doesNotThrow(() => context.playNotifySound('nonsense'));

if (fail.length) { console.log(fail.join('\n')); process.exit(1); }
console.log('SOUND OK');
`
