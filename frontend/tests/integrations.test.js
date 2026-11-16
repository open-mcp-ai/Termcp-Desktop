import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

const memory = new Map();
globalThis.localStorage = {
  getItem(key) { return memory.get(key) || null; },
  setItem(key, value) { memory.set(key, String(value)); },
};
globalThis.document = { documentElement: { lang: 'en', dataset: {}, style: { setProperty() {} } } };
globalThis.window = {};

const { integrationPage } = await import('../src/features/integrations/view.js');
const { state } = await import('../src/state.js');

test('integration page uses live Core docs and exports the Desktop HTTP skill', async () => {
  state.data.core.address = 'http://127.0.0.1:18765';
  const html = integrationPage();
  assert.match(html, /http:\/\/127\.0\.0\.1:18765\/api\.md/);
  assert.match(html, /data-action="export-termcp-skill"/);
  assert.match(html, /learn-api/);
  assert.doesNotMatch(html, /\/skills\.md/);

  const skillPath = fileURLToPath(new URL('../../skills/termcp/SKILL.md', import.meta.url));
  const skill = await readFile(skillPath, 'utf8');
  assert.match(skill, /GET \/api\.md/);
  assert.match(skill, /POST \/api\/term/);
  assert.match(skill, /\/sse/);
  assert.match(skill, /\/stream/);
});
