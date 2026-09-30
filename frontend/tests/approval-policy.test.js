import test from 'node:test';
import assert from 'node:assert/strict';

const saved = new Map();
globalThis.localStorage = {
  getItem(key) { return saved.get(key) || null; },
  setItem(key, value) { saved.set(key, String(value)); },
};

const policy = await import('../src/features/approval/policy.js');

test('approval defaults off and connection overrides win over the global default', () => {
  const connections = [{ name: 'internal', default_approval: false }, { name: 'production', default_approval: true }];
  policy.rememberPolicies(connections);
  assert.equal(policy.approvalSettings.defaultEnabled, false);
  assert.equal(policy.effectiveApproval('internal', connections[0]), false);
  assert.equal(policy.effectiveApproval('production', connections[1]), true);

  policy.setGlobalApproval(true);
  assert.equal(policy.effectiveApproval('internal', connections[0]), true);
  policy.setConnectionPolicy('production', 'off');
  assert.equal(policy.effectiveApproval('production', connections[1]), false);
  policy.setGlobalApproval(false);
  assert.equal(policy.effectiveApproval('internal', connections[0]), false);
});

test('profile approval update leaves jump-host settings intact', () => {
  const source = 'kind = "remote"\nhost = "example.test"\n\n[jump]\nhost = "bastion.test"\n';
  const enabled = policy.withDefaultApproval(source, true);
  assert.match(enabled, /^default_approval = true$/m);
  assert.match(enabled, /\[jump\]\nhost = "bastion.test"/);
  assert.equal(enabled.match(/default_approval/g)?.length, 1);
  assert.equal(policy.withDefaultApproval(enabled, false).match(/default_approval = false/g)?.length, 1);
});
