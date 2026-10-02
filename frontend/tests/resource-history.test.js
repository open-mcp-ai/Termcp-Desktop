import test from 'node:test';
import assert from 'node:assert/strict';

const memory = new Map();
globalThis.localStorage = {
  getItem(key) { return memory.get(key) || null; },
  setItem(key, value) { memory.set(key, String(value)); },
};
globalThis.document = { documentElement: { lang: 'en', dataset: {}, style: { setProperty() {} } } };
globalThis.window = {};

const { commandLinePayload } = await import('../src/features/resources/command.js');
const { tagsFor, setTags, knownTags } = await import('../src/features/resources/tags.js');
const { connectionPage, historyPage } = await import('../src/features/resources/pages.js');
const { historyPreviewSpan, readableTerminalText } = await import('../src/features/resources/history.js');
const { state } = await import('../src/state.js');

test('startup command is sent as executable and argv for v0.2.4', () => {
  assert.deepEqual(commandLinePayload('python -m http.server "8080 test"'), { command: 'python', args: ['-m', 'http.server', '8080 test'] });
  assert.deepEqual(commandLinePayload(''), { command: '', args: [] });
  assert.throws(() => commandLinePayload('echo "unfinished'), /quote/);
});

test('connection details do not claim live host status', () => {
  state.data.sessions = [];
  const html = connectionPage({ name: 'host', kind: 'remote', host: 'example.test', user: 'alice', port: 22 });
  assert.doesNotMatch(html, /class="status-dot running"/);
});

test('resource tags are deduplicated, persisted and shared with archived session', () => {
  assert.deepEqual(setTags('session', 's1', 'release, staging, release'), ['release', 'staging']);
  assert.deepEqual(tagsFor('history', 's1'), ['release', 'staging']);
  assert.deepEqual(knownTags([{ type: 'history', id: 's1' }]), ['release', 'staging']);
  assert.match(memory.get('termcp-desktop-resource-tags'), /session:s1/);
  setTags('session', 's1', '');
  assert.deepEqual(tagsFor('history', 's1'), []);
});

const archived = { id: 'history-repro', name: 'History repro', status: 'dead', ssh_endpoint: 'internal', updated_at: Date.now(), shells: [{ id: 'shell-repro', name: 'shell', mode: 'pty' }] };

test('history keeps an input marker separate from the following output range', () => {
  state.historyShellID = 'shell-repro';
  state.historySpans = [
    { status: 'i', time: Date.now(), start: 0, end: 0 },
    { status: 'o', time: Date.now(), start: 0, end: 16 },
  ];
  state.historySpanIndex = 0;
  state.historyText = 'unrelated output';
  state.historyNextOffset = 16;
  state.historyLoading = false;
  state.historyError = '';
  const inputHTML = historyPage(archived);
  assert.match(inputHTML, /Input events are markers only/);
  assert.doesNotMatch(inputHTML, /class="transcript"/);
  state.historySpanIndex = 1;
  const outputHTML = historyPage(archived);
  assert.match(outputHTML, /class="transcript">unrelated output/);
});

test('history output renders terminal control codes as readable text', () => {
  state.historyShellID = 'shell-repro';
  state.historySpans = [{ status: 'o', time: Date.now(), start: 0, end: 24 }];
  state.historySpanIndex = 0;
  state.historyText = '\x1b[0m\x1b[39m\x1b[49m\x1b[24mhello \x1b[32mworld\x1b[0m\r\n';
  state.historyNextOffset = 24;
  state.historyLoading = false;
  state.historyError = '';
  const html = historyPage(archived);
  assert.match(html, /hello world/);
  assert.doesNotMatch(html, /\x1b/);
});

test('input marks never claim output bytes, even at the same offset', () => {
  const spans = [
    { status: 'i', start: 12, end: 12 },
    { status: 'q', start: 12, end: 12 },
    { status: 'o', start: 12, end: 27 },
    { status: 'i', start: 27, end: 27 },
    { status: 'o', start: 28, end: 35 },
  ];
  assert.equal(historyPreviewSpan(spans, 0), null);
  assert.equal(historyPreviewSpan(spans, 2), spans[2]);
  assert.equal(historyPreviewSpan(spans, 3), null);
});

test('history does not invent input text when later output has a different offset', () => {
  state.historySpans = [
    { status: 'i', time: Date.now(), start: 0, end: 0 },
    { status: 'o', time: Date.now(), start: 1, end: 6 },
  ];
  state.historySpanIndex = 0;
  state.historyText = 'later';
  const html = historyPage(archived);
  assert.match(html, /Input events are markers only/);
  assert.doesNotMatch(html, /class="transcript">later/);
});

test('terminal text renderer handles OSC, carriage return and backspace', () => {
  assert.equal(readableTerminalText('\x1b]0;private title\x07progress 1\rprogress 2\nabc\bD'), 'progress 2\nabD');
});
