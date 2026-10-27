import test from 'node:test';
import assert from 'node:assert/strict';

import {
  activityLabel,
  activityMatches,
  clientsFromFrame,
  normalizeActivity,
  normalizeClients,
} from '../src/features/observability/model.js';
import { ObservabilityController } from '../src/features/observability/controller.js';
import { observabilityPage } from '../src/features/observability/view.js';

function emptyState() {
  return {
    section: 'observability',
    wsStatus: 'connected',
    data: {
      sessions: [{ id: 'session-live', name: 'Live session', status: 'running' }],
      history: [{ id: 'session-archived', name: 'Archived session', status: 'exited' }],
    },
    observability: {
      loading: false,
      error: '',
      sessionID: '',
      activity: [],
      clients: { webui: [], mcp: [] },
    },
  };
}

test('normalizes Core activity and attached client frames', () => {
  const activity = normalizeActivity([
    { session_id: 's1', shell_id: 'h1', source: 'ai', verdict: 'input', bytes: 12 },
    { session_id: '', shell_id: 'ignored' },
  ]);
  assert.equal(activity.length, 1);
  assert.equal(activityMatches(activity[0], 's1'), true);
  assert.equal(activityMatches(activity[0], 's2'), false);
  assert.equal(activityLabel(activity[0]), 'AI submitted input');

  const clients = clientsFromFrame({
    clients: [{ id: '1', self: true }],
    mcp_clients: [{ session_id: 'm1', name: 'Agent' }],
  });
  assert.equal(clients.webui.length, 1);
  assert.equal(clients.mcp.length, 1);
  assert.deepEqual(normalizeClients(null), { webui: [], mcp: [] });
  assert.equal(clientsFromFrame({ type: 'terminal' }), null);
});

test('loads activity and clients, then applies live connection frames without polling', async () => {
  const state = emptyState();
  let renders = 0;
  let activityRequests = 0;
  const core = {
    async activity(sessionID, limit) {
      activityRequests += 1;
      assert.equal(sessionID, '');
      assert.equal(limit, 200);
      return [{ session_id: 's1', shell_id: 'h1', source: 'human', verdict: 'input', bytes: 5 }];
    },
    async clients() {
      return { webui: [{ id: '1' }], mcp: [] };
    },
  };
  const controller = new ObservabilityController(core, state, { render: () => { renders += 1; } });
  await controller.load();
  assert.equal(activityRequests, 1);
  assert.equal(state.observability.activity.length, 1);
  assert.equal(state.observability.clients.webui.length, 1);
  assert.ok(renders >= 2);

  assert.equal(controller.applySessionFrame({
    type: 'sessions',
    clients: [{ id: '1', self: true }, { id: '2' }],
    mcp_clients: [{ session_id: 'm2', name: 'agent' }],
  }), true);
  assert.equal(state.observability.clients.webui.length, 2);
  assert.equal(state.observability.clients.mcp[0].session_id, 'm2');
  controller.dispose();
});

test('renders filtered activity with a byte target and separate client kinds', () => {
  const state = emptyState();
  state.observability.activity = [{
    time: '2026-10-27T08:00:00Z',
    session_id: 'session-live',
    shell_id: 'shell-live',
    session_name: 'Live session',
    source: 'ai',
    verdict: 'input',
    bytes: 42,
  }];
  state.observability.clients = {
    webui: [{ id: '1', addr: '127.0.0.1', user_agent: 'Termcp Desktop', watching: 1, since: '2026-10-27T08:00:00Z', self: true }],
    mcp: [{ session_id: 'm1', name: 'Agent', version: '1.0', namespace: 'ns:test', connected_at: '2026-10-27T08:00:00Z' }],
  };
  const html = observabilityPage(state);
  assert.match(html, /data-activity-open/);
  assert.match(html, /data-session="session-live"/);
  assert.match(html, /data-shell="shell-live"/);
  assert.match(html, /data-byte="42"/);
  assert.match(html, /Termcp Desktop · This window/);
  assert.match(html, /Agent 1\.0/);
  assert.match(html, /ns:test/);
});
