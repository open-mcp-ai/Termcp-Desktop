import test from 'node:test';
import assert from 'node:assert/strict';

const memory = new Map();
globalThis.localStorage = {
  getItem(key) { return memory.get(key) || null; },
  setItem(key, value) { memory.set(key, String(value)); },
};
globalThis.document = { documentElement: { lang: 'en', dataset: {}, style: { setProperty() {} } } };
globalThis.window = {};

const { state } = await import('../src/state.js');
const { exportConnectionsBody, exportableConnections, importResultBody } = await import('../src/features/connections/transfer.js');

test('host export lists remote hosts only and defaults to all of them', () => {
  state.data.connections = [
    { name: 'internal', kind: 'internal' },
    { name: 'production', kind: 'remote', host: 'prod.example', user: 'deploy' },
    { name: 'staging', kind: 'remote', host: 'stage.example', user: 'ops' },
  ];
  state.dialog = { type: 'export-connections', error: '' };
  assert.deepEqual(exportableConnections().map(item => item.name), ['production', 'staging']);
  const html = exportConnectionsBody();
  assert.doesNotMatch(html, /value="internal"/);
  assert.equal((html.match(/name="connections"/g) || []).length, 2);
  assert.equal((html.match(/name="connections"[^>]*checked/g) || []).length, 2);
  assert.match(html, /data-select-all-connections checked/);
  assert.match(html, /name="password" type="password"/);
  assert.match(html, /request header/);
});

test('host import results keep every success and failure visible', () => {
  const html = importResultBody({
    file_name: 'hosts.enc',
    imported: 2,
    renamed: 1,
    failed: 1,
    hosts: [
      { name: 'production', status: 'imported' },
      { name: 'staging', final_name: 'staging-2', status: 'renamed' },
      { name: 'broken', status: 'failed', error: 'host is required' },
    ],
  });
  assert.match(html, /2 imported · 1 renamed · 1 failed/);
  assert.match(html, /status-imported">imported/);
  assert.match(html, /Created as staging-2/);
  assert.match(html, /status-failed">failed/);
  assert.match(html, /host is required/);
});
