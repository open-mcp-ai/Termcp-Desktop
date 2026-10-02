import test from 'node:test';
import assert from 'node:assert/strict';

import { SessionCreationCoordinator } from '../src/features/resources/session-create.js';

const tick = () => new Promise(resolve => setImmediate(resolve));
const created = { data: { session_id: 'session-new', shell_id: 'shell-new' } };

test('session creation waits for the matching Core sessions notification', async () => {
  const coordinator = new SessionCreationCoordinator({
    create: async () => created,
    snapshot: async () => ({ sessions: [] }),
    connected: () => true,
    timeoutMs: 50,
  });
  let settled = false;
  const attempt = coordinator.create('host', { ssh_config: 'host' }).then(result => { settled = true; return result; });
  await tick();
  assert.equal(settled, false, 'HTTP 200 alone must not report a connected session');
  coordinator.observe({ type: 'sessions', sessions: [{ id: 'another-session' }] });
  await tick();
  assert.equal(settled, false, 'another session must not confirm this connection');
  coordinator.observe({ type: 'sessions', sessions: [{ id: 'session-new' }] });
  assert.deepEqual(await attempt, created.data);
});

test('notification that arrives before the HTTP response still confirms the new session', async () => {
  let finishCreate;
  const coordinator = new SessionCreationCoordinator({
    create: () => new Promise(resolve => { finishCreate = resolve; }),
    snapshot: async () => ({ sessions: [] }),
    connected: () => true,
    timeoutMs: 50,
  });
  const attempt = coordinator.create('host', { ssh_config: 'host' });
  coordinator.observe({ type: 'sessions', sessions: [{ id: 'session-new' }] });
  finishCreate(created);
  assert.deepEqual(await attempt, created.data);
});

test('duplicate connection attempts make one request and a real dial failure stays an error', async () => {
  let calls = 0;
  let fail;
  const coordinator = new SessionCreationCoordinator({
    create: () => { calls += 1; return new Promise((_, reject) => { fail = reject; }); },
    snapshot: async () => { throw new Error('snapshot must not decide a failed dial'); },
    connected: () => true,
    timeoutMs: 50,
  });
  const attempt = coordinator.create('host', { ssh_config: 'host' });
  assert.equal(coordinator.pending('host'), true);
  assert.equal(await coordinator.create('host', { ssh_config: 'host' }), null);
  assert.equal(calls, 1);
  coordinator.observe({ type: 'sessions', sessions: [{ id: 'another-session' }] });
  fail(new Error('no route to host'));
  await assert.rejects(attempt, /no route to host/);
  assert.equal(coordinator.pending('host'), false);
});

test('when the event channel is unavailable, the current Core snapshot confirms the returned id', async () => {
  const coordinator = new SessionCreationCoordinator({
    create: async () => created,
    snapshot: async () => ({ sessions: [{ id: 'session-new' }] }),
    connected: () => false,
  });
  assert.deepEqual(await coordinator.create('host', { ssh_config: 'host' }), created.data);
});

test('missing notification falls back to Core state without inventing success', async () => {
  const present = new SessionCreationCoordinator({
    create: async () => created,
    snapshot: async () => ({ sessions: [{ id: 'session-new' }] }),
    connected: () => true,
    timeoutMs: 1,
  });
  assert.deepEqual(await present.create('host', { ssh_config: 'host' }), created.data);

  const absent = new SessionCreationCoordinator({
    create: async () => created,
    snapshot: async () => ({ sessions: [] }),
    connected: () => true,
    timeoutMs: 1,
  });
  await assert.rejects(absent.create('host', { ssh_config: 'host' }), /did not confirm session/);
});
