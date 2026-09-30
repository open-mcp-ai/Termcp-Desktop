import test from 'node:test';
import assert from 'node:assert/strict';

import { ShellActivityTracker } from '../src/features/workspace/activity.js';

test('tracks human and AI input from Core without treating terminal echo as a submit', () => {
  const changes = [];
  const activity = new ShellActivityTracker((shellID, status) => changes.push([shellID, status]));

  activity.input({ shell_id: 'human-shell', src: 'api', submit: false });
  activity.output('human-shell', 'l');
  assert.equal(activity.status('human-shell'), 'human');

  activity.input({ shell_id: 'agent-shell', src: 'ai', submit: false });
  activity.output('agent-shell', 'partial echo\r\n');
  assert.equal(activity.status('agent-shell'), 'ai');

  activity.input({ shell_id: 'agent-shell', src: 'api', submit: false });
  assert.equal(activity.status('agent-shell'), 'ai', 'human keystrokes do not relabel an AI line');
  activity.input({ shell_id: 'agent-shell', src: 'ai', submit: true });
  assert.equal(activity.status('agent-shell'), '');
  assert.equal(activity.status('human-shell'), 'human');

  activity.end('human-shell');
  assert.deepEqual(changes, [
    ['human-shell', 'human'],
    ['agent-shell', 'ai'],
    ['agent-shell', ''],
    ['human-shell', ''],
  ]);
  activity.clear();
});

test('clears an inactive typing indicator and accepts a later input event', async () => {
  const activity = new ShellActivityTracker(() => {}, 10);
  activity.input({ shell_id: 'shell-1', src: 'ai', submit: false });
  await new Promise(resolve => setTimeout(resolve, 30));
  assert.equal(activity.status('shell-1'), '');
  activity.input({ shell_id: 'shell-1', src: 'api', submit: false });
  assert.equal(activity.status('shell-1'), 'human');
  activity.clear();
});
