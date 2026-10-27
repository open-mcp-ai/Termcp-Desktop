import test from 'node:test';
import assert from 'node:assert/strict';

const memory = new Map();
globalThis.localStorage = {
  getItem(key) { return memory.get(key) || null; },
  setItem(key, value) { memory.set(key, String(value)); },
};
globalThis.document = { documentElement: { lang: 'en', dataset: {}, style: { setProperty() {} } } };
globalThis.window = {};

const {
  extensionNamesFromDocument,
  extensionPayload,
  formFromExtensionDetail,
  importFailureReport,
  importReport,
  userSessions,
} = await import('../src/features/extensions/model.js');
const { ExtensionController, readExtensionImport } = await import('../src/features/extensions/controller.js');
const { extensionsPage } = await import('../src/features/extensions/view.js');
const { state } = await import('../src/state.js');

test('extension session filtering keeps only caller-owned sessions', () => {
  const sessions = [
    { id: 'plain', name: 'plain' },
    { id: 'infra', name: 'ext:prod', namespace: 'ext:prod' },
    { id: 'bare-infra', name: 'ext', namespace: 'ext' },
    { id: 'caller', name: 'agent', namespace: 'ns:alice' },
    { id: 'prefixed-but-not-extension', name: 'external', namespace: 'external' },
  ];
  assert.deepEqual(userSessions(sessions).map(item => item.id), ['plain', 'caller', 'prefixed-but-not-extension']);
});

test('field editor reconstructs fields that Core exposes only in the TOML file view', () => {
  const detail = {
    extension: { name: 'docs', kind: 'skill', target: 'internal', enabled: false, description: 'docs tree' },
    mode: '',
    command: '',
    args: ['one', 'two'],
    allow: ['file_read'],
    paths: ['/srv/docs'],
    notes: 'notes',
    forward_decls: [],
  };
  const toml = `kind = "skill"
target = "internal"
idle_timeout = "45s"

[headers]
Authorization = "Bearer token"
`;
  const form = formFromExtensionDetail(detail, toml);
  assert.equal(form.idleTimeout, '45s');
  assert.deepEqual(JSON.parse(form.headers), { Authorization: 'Bearer token' });
  assert.equal(form.args, 'one\ntwo');
  assert.equal(form.enabled, false);
});

test('update payload clears submitted empties while create omits blanks for presets', () => {
  const form = {
    name: 'draft',
    kind: 'mcp',
    target: 'internal',
    description: '',
    enabled: true,
    mode: '',
    command: '',
    args: '',
    url: '',
    headers: '',
    allow: '',
    paths: '',
    idleTimeout: '',
    notes: '',
    forwards: '',
  };
  const create = extensionPayload(form, { create: true });
  assert.deepEqual(create, { kind: 'mcp', target: 'internal', enabled: true, name: 'draft' });
  const update = extensionPayload(form);
  assert.equal(update.description, '');
  assert.equal(update.mode, '');
  assert.deepEqual(update.args, []);
  assert.deepEqual(update.allow, []);
  assert.deepEqual(update.headers, {});
  assert.deepEqual(update.forwards, []);
});

test('import report names every template and reports conflicts', () => {
  const document = `[[extensions]]
name = "one"
kind = "custom"
target = "internal"

[[extensions]]
name = "two"
kind = "custom"
target = "internal"
`;
  assert.deepEqual(extensionNamesFromDocument(document), ['one', 'two']);
  const rows = importReport(document, { imported: 2, renamed: [{ from: 'two', to: 'two-2' }] });
  assert.deepEqual(rows, [
    { name: 'one', importedAs: 'one', status: 'created', error: '' },
    { name: 'two', importedAs: 'two-2', status: 'created', error: '' },
  ]);
  assert.equal(importFailureReport(new Error('parse failed'))[0].status, 'failed');
  assert.match(importFailureReport(new Error('parse failed'))[0].error, /parse failed/);
});

test('extension controller uses the same REST routes for fields and TOML', async () => {
  const calls = [];
  const api = async (...args) => {
    calls.push(args);
    if (args[0] === 'GET' && args[1].includes('format=toml')) return { body: 'kind = "custom"\n' };
    if (args[0] === 'GET' && args[1] === '/api/extensions') return { data: { extensions: [{ name: 'draft' }] } };
    if (args[0] === 'GET') return { data: { extension: { name: 'draft' }, failures: [] } };
    if (args[1].includes('/retry')) return { data: { ok: false, error: 'disabled' } };
    return { data: { imported: 1 } };
  };
  const controller = new ExtensionController(api);
  assert.deepEqual(await controller.list(), [{ name: 'draft' }]);
  const editor = await controller.editor('draft');
  assert.equal(editor.toml, 'kind = "custom"\n');
  await controller.saveForm('', 'draft', { kind: 'custom' });
  await controller.saveTOML('draft', 'draft', 'kind = "custom"\n');
  await controller.saveTOML('', 'draft', 'kind = "custom"\n');
  await assert.rejects(controller.retry('draft'), /disabled/);
  assert.ok(calls.some(call => call[0] === 'POST' && call[1] === '/api/extensions'));
  assert.ok(calls.some(call => call[0] === 'PUT' && call[1] === '/api/extensions/draft' && call[3].includes('toml')));
  assert.ok(calls.some(call => call[0] === 'POST' && call[1] === '/api/extensions?name=draft' && call[3].includes('toml')));
});

test('import file reading rejects oversized and empty inputs without writing', async () => {
  assert.deepEqual(await readExtensionImport(null), { cancelled: true });
  await assert.rejects(readExtensionImport({ size: (16 << 20) + 1, text: async () => 'x' }), /larger/);
  await assert.rejects(readExtensionImport({ size: 2, text: async () => '  ' }), /empty/);
  assert.deepEqual(await readExtensionImport({ size: 2, text: async () => 'a = 1' }), { cancelled: false, document: 'a = 1' });
});

test('extensions page renders only Core status and failure facts', () => {
  state.data.core.running = true;
  state.extensionsLoading = false;
  state.extensionError = '';
  state.extensionImport = null;
  state.extensions = [{
    name: 'docs',
    kind: 'skill',
    target: 'internal',
    enabled: true,
    status: 'unavailable',
    failure: { stage: 'target', reason: 'profile missing' },
    next_retry_ms: Date.now() + 5000,
    forwards: [],
  }];
  const html = extensionsPage();
  assert.match(html, /profile missing/);
  assert.match(html, /Unavailable/);
  assert.match(html, /Next retry/);
  assert.doesNotMatch(html, /extension-status status-available/);
});

test('extensions page shows forward addresses and explains broken or unusable templates', () => {
  state.data.core.running = true;
  state.extensionsLoading = false;
  state.extensionError = '';
  state.extensions = [
    {
      name: 'tunnel',
      kind: 'custom',
      target: 'ops',
      enabled: true,
      status: 'available',
      forwards: [{ kind: 'local', listen_addr: '127.0.0.1:8080', target_addr: 'db.internal:5432' }],
    },
    { name: 'broken', enabled: false, status: 'unavailable' },
  ];
  const html = extensionsPage();
  assert.match(html, /local · 127.0.0.1:8080 · db.internal:5432/);
  assert.match(html, /No terminal handle reported/);
  assert.match(html, /template unavailable/);
  assert.match(html, /The stored template could not be read/);
});
