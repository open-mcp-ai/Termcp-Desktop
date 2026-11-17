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
  parseValuesDocument,
  userSessions,
  valuesFromHoles,
} = await import('../src/features/extensions/model.js');
const { ExtensionController, readExtensionImport } = await import('../src/features/extensions/controller.js');
const { extensionEditorBody, extensionEditorOpen, extensionsPage } = await import('../src/features/extensions/view.js');
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
    extension: { name: 'docs', target: '', enabled: false, description: 'docs tree', url: '' },
    target: '',
    targets: [{ host: 'node-a' }, { host: 'node-b' }],
    mode: '',
    command: '',
    args: ['one', 'two'],
    allow: ['file_read'],
    paths: ['/srv/docs'],
    notes: 'notes',
    forward_decls: [],
  };
  const toml = `target = "internal"
resident = true
termcp_tool = "termcp_term"
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
  assert.equal(form.resident, true);
  assert.equal(form.termcpTool, 'termcp_term');
  assert.deepEqual(JSON.parse(form.targets), [{ host: 'node-a' }, { host: 'node-b' }]);
  assert.equal('kind' in form, false);
});

test('update payload writes real capability fields and never sends a classification label', () => {
  const form = {
    name: 'draft',
    target: 'internal',
    targets: '',
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
    resident: false,
    termcpTool: '',
    notes: '',
    forwards: '',
  };
  const create = extensionPayload(form, { create: true });
  assert.deepEqual(create, { target: 'internal', enabled: true, resident: false, name: 'draft' });
  const update = extensionPayload(form);
  assert.equal('kind' in update, false);
  assert.equal(update.description, '');
  assert.equal(update.mode, '');
  assert.deepEqual(update.args, []);
  assert.deepEqual(update.allow, []);
  assert.deepEqual(update.headers, {});
  assert.deepEqual(update.forwards, []);
});

test('typed values are displayed separately and parsed without dropping false or zero', () => {
  const holes = [
    { name: 'enabled', type: 'boolean', set: true, value: false },
    { name: 'port', type: 'integer', set: true, value: 0 },
    { name: 'root', type: 'string', set: false },
  ];
  const document = valuesFromHoles(holes);
  assert.deepEqual(JSON.parse(document), { enabled: false, port: 0 });
  assert.deepEqual(parseValuesDocument(document), { enabled: false, port: 0 });
  assert.throws(() => parseValuesDocument('[]'), /JSON object/);
});

test('existing templates use lossless TOML and a separate values editor', () => {
  const dialog = extensionEditorOpen('existing');
  dialog.form = { ...dialog.form, name: 'existing', target: 'internal' };
  dialog.toml = 'target = "{{target}}"\n\n[params.target]\ntype = "string"\nhelp = "host"\n';
  dialog.values = '{"target":"internal"}';
  dialog.holes = [{ name: 'target', type: 'string', help: 'host', set: true, value: 'internal' }];
  const html = extensionEditorBody(dialog);
  assert.match(html, /Template TOML/);
  assert.match(html, />Values</);
  assert.match(html, /name="toml"/);
  assert.match(html, /name="values"/);
  assert.match(html, /Existing templates are edited as lossless TOML/);
  assert.match(html, /name="target"[^>]*disabled/);
});

test('multi-host values, resident, zero idle timeout and termcp_tool survive form serialization', () => {
  const payload = extensionPayload({
    name: 'multi',
    target: '',
    targets: '[{"host":"a"},{"host":"b"}]',
    description: '',
    enabled: true,
    mode: 'pipe',
    command: 'cat',
    args: '',
    url: '',
    headers: '',
    allow: '',
    paths: '',
    idleTimeout: '0',
    resident: true,
    termcpTool: 'termcp_term',
    notes: '',
    forwards: '',
  }, { create: true });
  assert.deepEqual(payload.targets, [{ host: 'a' }, { host: 'b' }]);
  assert.equal(payload.idle_timeout, '0');
  assert.equal(payload.resident, true);
  assert.equal(payload.termcp_tool, 'termcp_term');
  assert.equal('kind' in payload, false);
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
  await controller.saveForm('', 'draft', { target: 'internal' });
    await controller.saveTOML('draft', 'draft', 'target = "internal"\n', { command: 'cat' });
    await controller.saveTOML('', 'draft', 'target = "{{target}}"\n', { target: 'internal' });
    await controller.saveValues('draft', { target: 'internal' });
    await controller.preview('draft', 'target = "{{target}}"\n');
  await assert.rejects(controller.retry('draft'), /disabled/);
  assert.ok(calls.some(call => call[0] === 'POST' && call[1] === '/api/extensions'));
  const payload = call => typeof call[2] === 'string' ? JSON.parse(call[2]) : call[2];
  assert.ok(calls.some(call => call[0] === 'PUT' && call[1] === '/api/extensions/draft' && call[3] === 'application/json' && payload(call).values.command === 'cat'));
  assert.ok(calls.some(call => call[0] === 'POST' && call[1] === '/api/extensions?name=draft' && call[3] === 'application/json'));
  assert.ok(calls.some(call => call[0] === 'PUT' && call[1] === '/api/extensions/draft' && payload(call).values.target === 'internal'));
  assert.ok(calls.some(call => call[0] === 'POST' && call[1] === '/api/extensions/draft/preview' && call[3].includes('toml')));
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
      target: 'ops',
      enabled: true,
      status: 'available',
      forwards: [{ kind: 'local', listen_addr: '127.0.0.1:8080', target_addr: 'db.internal:5432' }],
    },
    { name: 'broken', enabled: false, status: 'unavailable' },
  ];
  const html = extensionsPage();
  assert.match(html, /local · 127.0.0.1:8080 · db.internal:5432/);
  assert.match(html, /Available without a terminal shell/);
  assert.match(html, /Template unavailable/);
});

test('URL peers and multi-host templates are not rejected for missing terminal handles', () => {
  state.data.core.running = true;
  state.extensionsLoading = false;
  state.extensionError = '';
  state.extensions = [{
    name: 'remote-tools',
    url: 'https://mcp.example.test/stream',
    enabled: true,
    status: 'available',
    host_count: 1,
    forward_count: 0,
  }, {
    name: 'cluster',
    target: '',
    enabled: true,
    status: 'unavailable',
    host_count: 2,
    forward_count: 0,
    targets: [
      { target: 'node-a', status: 'available' },
      { target: 'node-b', status: 'unavailable', next_retry: Date.now() + 5000 },
    ],
  }, {
    name: 'waiting',
    enabled: true,
    status: 'idle',
    needs: ['root'],
    holes: [{ name: 'root', type: 'string', help: 'root path', set: false }],
  }];
  const html = extensionsPage();
  assert.match(html, /URL MCP/);
  assert.match(html, /MCP peer remote-tools · no terminal shell/);
  assert.match(html, /Multi-host/);
  assert.match(html, /node-a/);
  assert.match(html, /Needs root/);
  assert.doesNotMatch(html, /Template unavailable/);
});
