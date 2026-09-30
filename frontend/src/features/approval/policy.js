const KEY = 'termcp-desktop-approval-settings';

function load() {
  try {
    const value = JSON.parse(localStorage.getItem(KEY) || '{}');
    if (value && typeof value === 'object') return {
      defaultEnabled: value.defaultEnabled === true,
      desktopNotifications: value.desktopNotifications === true,
      policies: value.policies && typeof value.policies === 'object' ? value.policies : {},
    };
  } catch { /* Damaged local settings fall back to approval off. */ }
  return { defaultEnabled: false, desktopNotifications: false, policies: {} };
}

export const approvalSettings = load();

function save() { localStorage.setItem(KEY, JSON.stringify(approvalSettings)); }

export function policyFor(name, connection) {
  const policy = approvalSettings.policies[name];
  return ['inherit', 'on', 'off'].includes(policy) ? policy : connection?.default_approval ? 'on' : 'inherit';
}

export function rememberPolicies(connections) {
  let changed = false;
  for (const connection of connections) {
    if (!Object.hasOwn(approvalSettings.policies, connection.name)) {
      approvalSettings.policies[connection.name] = connection.default_approval ? 'on' : 'inherit';
      changed = true;
    }
  }
  if (changed) save();
}

export function effectiveApproval(name, connection) {
  const policy = policyFor(name, connection);
  return policy === 'on' || (policy === 'inherit' && approvalSettings.defaultEnabled);
}

export function setGlobalApproval(enabled) {
  approvalSettings.defaultEnabled = Boolean(enabled);
  save();
}

export function setConnectionPolicy(name, policy) {
  if (!['inherit', 'on', 'off'].includes(policy)) throw new Error('Invalid approval policy');
  approvalSettings.policies[name] = policy;
  save();
}

export function moveConnectionPolicy(oldName, newName) {
  if (oldName === newName) return;
  if (Object.hasOwn(approvalSettings.policies, oldName)) approvalSettings.policies[newName] = approvalSettings.policies[oldName];
  delete approvalSettings.policies[oldName];
  save();
}

export function removeConnectionPolicy(name) {
  delete approvalSettings.policies[name];
  save();
}

export function setDesktopNotifications(enabled) {
  approvalSettings.desktopNotifications = Boolean(enabled);
  save();
}

// Preserve the rest of the Core profile, including jump-host sections and secrets.
export function withDefaultApproval(raw, enabled) {
  const lines = String(raw).replace(/\r\n/g, '\n').split('\n');
  const section = lines.findIndex(line => /^\s*\[[^\]]+\]/.test(line));
  const end = section < 0 ? lines.length : section;
  const existing = lines.findIndex((line, index) => index < end && /^\s*default_approval\s*=/.test(line));
  if (existing >= 0) lines[existing] = `default_approval = ${Boolean(enabled)}`;
  else lines.splice(end, 0, `default_approval = ${Boolean(enabled)}`);
  return `${lines.join('\n').replace(/\n*$/, '')}\n`;
}
