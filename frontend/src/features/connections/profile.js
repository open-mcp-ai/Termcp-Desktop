function parseTOMLValue(raw) {
  const value = String(raw || '').trim();
  if (value === 'true') return true;
  if (value === 'false') return false;
  if (/^-?\d+$/.test(value)) return Number(value);
  if (value.startsWith('"') && value.endsWith('"')) {
    try { return JSON.parse(value); } catch { return value.slice(1, -1).replace(/\\"/g, '"').replace(/\\\\/g, '\\'); }
  }
  if (value.startsWith("'") && value.endsWith("'")) return value.slice(1, -1);
  return value;
}

function parseConnectionTOML(raw = '') {
  const root = {}; const sections = {}; let current = root;
  const lines = String(raw).replace(/\r/g, '').split('\n');
  for (let index = 0; index < lines.length;) {
    const line = lines[index++].trim();
    if (!line || line.startsWith('#')) continue;
    const section = line.match(/^\[([^\]]+)]$/);
    if (section) { const name = section[1].trim(); current = sections[name] || (sections[name] = {}); continue; }
    const pair = line.match(/^([A-Za-z0-9_]+)\s*=\s*(.*)$/);
    if (!pair) continue;
    const [, key, source] = pair;
    if (source.trim().startsWith('"""')) {
      let value = source.trim().slice(3); const parts = [];
      const inlineEnd = value.indexOf('"""');
      if (inlineEnd >= 0) parts.push(value.slice(0, inlineEnd));
      else {
        if (value) parts.push(value);
        while (index < lines.length) {
          value = lines[index++]; const end = value.indexOf('"""');
          if (end >= 0) { parts.push(value.slice(0, end)); break; }
          parts.push(value);
        }
      }
      current[key] = parts.join('\n').replace(/^\n|\n$/g, '');
    } else current[key] = parseTOMLValue(source);
  }
  return { root, sections };
}

function nestedJumpConfig(raw = '') {
  const lines = String(raw).replace(/\r/g, '').split('\n');
  const start = lines.findIndex(line => /^\s*\[jump\.jump(?:\.jump)*]\s*(?:#.*)?$/.test(line));
  if (start < 0) return { raw: '', count: 0 };
  const nested = lines.slice(start).join('\n').trim();
  return { raw: nested, count: (nested.match(/^\s*\[jump\.jump(?:\.jump)*]/gm) || []).length };
}

export function emptyConnectionProfile(name = '') {
  return {
    name, description: '', host: '', port: 22, user: '', auth: 'password', password: '', privateKey: '', keyPassphrase: '',
    trustUnknownHost: true, knownHosts: '', dialTimeout: 30, proxy: '', defaultShell: '', defaultMode: 'pty',
    jumpEnabled: false, jumpHost: '', jumpPort: 22, jumpUser: '', jumpAuth: 'password', jumpPassword: '', jumpPrivateKey: '',
    jumpKeyPassphrase: '', jumpTrustUnknownHost: true, jumpKnownHosts: '', jumpDialTimeout: 30, jumpProxy: '', nestedJumpConfig: '', nestedJumpCount: 0,
  };
}

export function connectionProfileFromTOML(name, raw) {
  const parsed = parseConnectionTOML(raw); const root = parsed.root; const jump = parsed.sections.jump || {};
  const nested = nestedJumpConfig(raw);
  return {
    ...emptyConnectionProfile(name),
    description: root.description || '', host: root.host || '', port: root.port || 22, user: root.user || '',
    auth: root.private_key ? 'key' : 'password', password: root.password || '', privateKey: root.private_key || '', keyPassphrase: root.key_passphrase || '',
    trustUnknownHost: root.trust_unknown_host !== false, knownHosts: root.known_hosts || '', dialTimeout: root.dial_timeout_seconds || 30,
    proxy: root.proxy || '', defaultShell: root.default_shell || '', defaultMode: root.default_mode || 'pty',
    jumpEnabled: Boolean(jump.host), jumpHost: jump.host || '', jumpPort: jump.port || 22, jumpUser: jump.user || '',
    jumpAuth: jump.private_key ? 'key' : 'password', jumpPassword: jump.password || '', jumpPrivateKey: jump.private_key || '',
    jumpKeyPassphrase: jump.key_passphrase || '', jumpTrustUnknownHost: jump.trust_unknown_host !== false,
    jumpKnownHosts: jump.known_hosts || '', jumpDialTimeout: jump.dial_timeout_seconds || 30, jumpProxy: jump.proxy || '', nestedJumpConfig: nested.raw, nestedJumpCount: nested.count,
  };
}

const tomlString = value => JSON.stringify(String(value ?? ''));

export function connectionFormToTOML(form) {
  const data = new FormData(form); const lines = ['kind = "remote"'];
  const addString = (key, value, required = false) => { const text = String(value || ''); if (required || text.trim()) lines.push(`${key} = ${tomlString(text)}`); };
  addString('description', data.get('description'));
  addString('host', data.get('host'), true);
  lines.push(`port = ${Number(data.get('port') || 22)}`);
  addString('user', data.get('user'), true);
  if (data.get('auth') === 'key') {
    addString('private_key', data.get('private_key'), true);
    addString('key_passphrase', data.get('key_passphrase'));
  } else addString('password', data.get('password'), true);
  lines.push(`trust_unknown_host = ${data.get('trust_unknown_host') === 'true'}`);
  addString('known_hosts', data.get('known_hosts'));
  lines.push(`dial_timeout_seconds = ${Number(data.get('dial_timeout_seconds') || 30)}`);
  addString('proxy', data.get('proxy'));
  addString('default_shell', data.get('default_shell'));
  addString('default_mode', data.get('default_mode'));
  if (data.get('jump_enabled') === 'true') {
    lines.push('', '[jump]');
    addString('host', data.get('jump_host'), true);
    lines.push(`port = ${Number(data.get('jump_port') || 22)}`);
    addString('user', data.get('jump_user'), true);
    if (data.get('jump_auth') === 'key') {
      addString('private_key', data.get('jump_private_key'), true);
      addString('key_passphrase', data.get('jump_key_passphrase'));
    } else addString('password', data.get('jump_password'), true);
    lines.push(`trust_unknown_host = ${data.get('jump_trust_unknown_host') === 'true'}`);
    addString('known_hosts', data.get('jump_known_hosts'));
    lines.push(`dial_timeout_seconds = ${Number(data.get('jump_dial_timeout_seconds') || 30)}`);
    addString('proxy', data.get('jump_proxy'));
    const nested = String(data.get('nested_jump_config') || '').trim();
    if (nested) lines.push('', nested);
  }
  return `${lines.join('\n')}\n`;
}

