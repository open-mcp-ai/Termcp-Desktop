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
const { historyRowCells } = await import('../src/features/resources/history.js');
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

test('history presents a continuous terminal with source colours instead of offset excerpts', () => {
  state.historyShellID = 'shell-repro';
  state.historyLoading = false;
  state.historyError = '';
  const html = historyPage(archived);
  assert.match(html, /data-history-terminal="shell-repro"/);
  for (const kind of ['input', 'agent', 'output']) assert.match(html, new RegExp(`history-colour-${kind}`));
  assert.doesNotMatch(html, /history-timeline|Input events are markers only|class="transcript"/);
});

test('row colours follow zero-length input marks and preserve the output cause across rows', () => {
  const marks = [
    { status: 'i', start: 0, end: 0, time: 100 },
    { status: 'o', start: 0, end: 20, time: 101 },
    { status: 'a', start: 20, end: 20, time: 200 },
    { status: 'o', start: 20, end: 50, time: 201 },
  ];
  const rows = [0, 10, 20, 30, 40].map(start => ({ start, end: start + 10 }));
  const cells = historyRowCells(marks, rows);
  assert.deepEqual(cells.map(cell => cell.status), ['i', 'o', 'a', 'o', 'o']);
  assert.deepEqual(cells.map(cell => cell.cause), ['', 'i', '', 'a', 'a']);
  assert.deepEqual(cells.map(cell => cell.start), [0, 10, 20, 30, 40]);
});

test('blank terminal rows stay blank and approval marks never replace content colours', () => {
  const cells = historyRowCells([
    { status: 'q', start: 0, end: 0 },
    { status: 'A', start: 0, end: 0 },
    { status: 'a', start: 0, end: 0 },
    { status: 'i', start: 0, end: 0 },
    { status: 'o', start: 0, end: 12 },
  ], [null, { start: 0, end: 12 }, null]);
  assert.equal(cells[0], undefined);
  assert.equal(cells[1].status, 'a');
  assert.equal(cells[1].review, true);
  assert.equal(cells[2], undefined);
});

test('the visible window can begin midway through an output span', () => {
  const cells = historyRowCells([
    { status: 'i', start: 0, end: 0 },
    { status: 'o', start: 0, end: 1000, time: 10 },
  ], [{ start: 720, end: 800 }, { start: 800, end: 900 }]);
  assert.deepEqual(cells.map(cell => [cell.status, cell.cause]), [['o', 'i'], ['o', 'i']]);
});

const { historyChunk } = await import('../src/features/resources/history-terminal.js');
test('UTF-8 split across byte windows is re-read without shifting byte offsets', () => {
  assert.deepEqual(historyChunk({ start: 0, end: 3, d: 'a\ufffd\ufffd' }), { text: 'a', next: 1 });
  assert.deepEqual(historyChunk({ start: 1, end: 4, d: '中' }), { text: '中', next: 4 });
  assert.deepEqual(historyChunk({ start: 7, end: 10, d: '\ufffd' }), { text: '\ufffd', next: 10 }, 'A literal replacement character is valid UTF-8');
});
