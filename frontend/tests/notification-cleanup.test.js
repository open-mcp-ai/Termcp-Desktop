import test from 'node:test';
import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';

const files = [
  'src/app.js',
  'src/core.js',
  'src/features/inspector/view.js',
  'src/features/settings/view.js',
  'src/i18n/zh-CN.js',
  'src/theme.css',
  'src/styles.css',
];

test('retired shell notification rules are absent without removing approval notifications', async () => {
  const sources = Object.fromEntries(await Promise.all(files.map(async file => [file, await readFile(new URL(`../${file}`, import.meta.url), 'utf8')])));
  for (const [file, source] of Object.entries(sources)) {
    assert.doesNotMatch(source, /\/api\/notifications/, file);
    assert.doesNotMatch(source, /shell_notify/, file);
    assert.doesNotMatch(source, /data-inspector-tab="notifications"/, file);
  }
  assert.match(sources['src/app.js'], /sendApprovalNotification/);
  assert.match(sources['src/features/settings/view.js'] || '', /data-approval-notifications/);
});
