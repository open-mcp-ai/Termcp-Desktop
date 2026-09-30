import test from 'node:test';
import assert from 'node:assert/strict';

import {
  activateInspectorShell,
  followShellDirectory,
  performFileUpload,
  rememberInspectorPath,
} from '../src/features/inspector/controller.js';
import { joinPath, parentPath } from '../src/ui/render.js';

function inspectorState() {
  return { path: '', shellID: '', paths: {}, shellDirectories: {} };
}

test('cancelled file selection does not report success or refresh the directory', async () => {
  const events = [];
  const result = await performFileUpload({
    upload: async () => ({ cancelled: true }),
    sessionID: 'session-1',
    directory: '/srv/app',
    onSuccess: () => events.push('success'),
    refresh: async () => events.push('refresh'),
  });

  assert.equal(result.cancelled, true);
  assert.deepEqual(events, []);
});

test('completed upload reports success and refreshes the directory', async () => {
  const events = [];
  const result = await performFileUpload({
    upload: async (sessionID, directory) => ({
      cancelled: false,
      local_name: 'build.log',
      remote_path: `${directory}/build.log`,
      sessionID,
    }),
    sessionID: 'session-1',
    directory: '/srv/app',
    onSuccess: value => events.push(`success:${value.remote_path}`),
    refresh: async () => events.push('refresh'),
  });

  assert.equal(result.cancelled, false);
  assert.deepEqual(events, ['success:/srv/app/build.log', 'refresh']);
});

test('file paths are retained per shell and follow cwd until manually changed', () => {
  const inspector = inspectorState();

  activateInspectorShell(inspector, 'shell-a');
  assert.equal(inspector.path, '');
  assert.equal(followShellDirectory(inspector, 'shell-a', '/home/alice'), true);
  assert.equal(inspector.path, '/home/alice');

  rememberInspectorPath(inspector, 'shell-a', '/var/log');
  assert.equal(followShellDirectory(inspector, 'shell-a', '/srv/app'), false);
  assert.equal(inspector.path, '/var/log', 'manual browsing must not be overwritten');

  activateInspectorShell(inspector, 'shell-b', '/home/bob');
  assert.equal(inspector.path, '/home/bob');
  activateInspectorShell(inspector, 'shell-a');
  assert.equal(inspector.path, '/var/log');
});

test('parent paths preserve Windows drive and network share roots', () => {
  assert.equal(parentPath('C:\\Users\\alice'), 'C:/Users');
  assert.equal(parentPath('C:/Users'), 'C:/');
  assert.equal(parentPath('C:/'), 'C:/');
  assert.equal(parentPath('//server/share/folder'), '//server/share');
  assert.equal(parentPath('//server/share'), '//server/share');
  assert.equal(joinPath('//server/share', 'folder'), '//server/share/folder');
});
