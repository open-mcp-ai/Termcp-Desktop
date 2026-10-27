import { t } from './i18n/index.js';
import { bundledCoreVersion } from './build-info.js';

const bridge = {
  available: () => Boolean(window.go?.main?.App),
  call(name, ...args) {
    const fn = window.go?.main?.App?.[name];
    if (!fn) return Promise.reject(new Error('Wails bridge unavailable'));
    let result;
    try { result = fn(...args); } catch (error) {
      if (name !== 'LogFrontend') reportFrontend('error', 'Wails binding threw', `${name}: ${String(error)}`);
      return Promise.reject(error);
    }
    return Promise.resolve(result).catch(error => {
      if (name !== 'LogFrontend') reportFrontend('error', 'Wails binding rejected', `${name}: ${String(error)}`);
      throw error;
    });
  },
};

function reportFrontend(level, message, details = '') {
  const logger = window.go?.main?.App?.LogFrontend;
  if (logger) Promise.resolve(logger(level, String(message), String(details))).catch(() => {});
  else if (level === 'error') console.error(message, details);
}

const now = new Date().toISOString();
const demoHistoryLines = ['$ npm test\r\n', '✓ all tests passed\r\n', '$ go version\r\n', 'go version go1.25.9\r\n', '$ '];
const demoHistoryLog = demoHistoryLines.join('');
const demoHistoryRows = demoHistoryLines.map((line, index) => ({
  start: new TextEncoder().encode(demoHistoryLines.slice(0, index).join('')).length,
  end: new TextEncoder().encode(demoHistoryLines.slice(0, index + 1).join('')).length,
}));
const demoHistoryMarks = [
  { status: 'i', time: Date.now() - 3000, start: 0, end: 0 },
  { status: 'o', time: Date.now() - 2900, start: 0, end: demoHistoryRows[2].start },
  { status: 'a', time: Date.now() - 2000, start: demoHistoryRows[2].start, end: demoHistoryRows[2].start },
  { status: 'o', time: Date.now() - 1900, start: demoHistoryRows[2].start, end: demoHistoryRows.at(-1).end },
];
const demo = {
  core: { running: true, managed: true, address: 'http://127.0.0.1:18765', version: `v${bundledCoreVersion}`, state: 'running' },
  connections: [
    { name: 'internal', kind: 'internal', description: 'This computer', default_approval: false },
    { name: 'dev-server', kind: 'remote', host: 'dev.example.test', user: 'developer', port: 22, description: 'Development' },
    { name: 'staging', kind: 'remote', host: 'staging.example.test', user: 'operator', port: 22, description: 'Staging' },
  ],
  sessions: [
    { id: 'session-demo-01', name: 'API development', status: 'running', approval_mode: true, approval_need: 1, ssh_endpoint: 'remote', created_at: Date.now(), updated_at: Date.now(), shells: [
      { id: 'shell-demo-01', name: 'build', status: 'running', mode: 'pty' },
      { id: 'shell-demo-02', name: 'logs', status: 'running', mode: 'pty' },
    ] },
    { id: 'session-demo-02', name: 'Local workspace', status: 'running', ssh_endpoint: 'internal', created_at: Date.now(), updated_at: Date.now(), shells: [
      { id: 'shell-demo-03', name: 'zsh', status: 'running', mode: 'pty' },
    ] },
  ],
  history: [
    { id: 'history-demo-01', name: 'Release check', status: 'dead', ssh_endpoint: 'remote', updated_at: Date.now(), shells: [
      { id: 'history-shell-01', name: 'release', status: 'exited', mode: 'pty' },
    ] },
  ],
  extensions: [
    { name: 'docs', kind: 'skill', target: 'internal', description: 'Read-only documentation window', enabled: true, status: 'idle', forwards: [] },
    { name: 'release-check', kind: 'mcp', target: 'dev-server', description: 'Release inspection MCP server', enabled: true, status: 'unavailable', failure: { at: Date.now() - 30000, stage: 'start', reason: 'stderr: command not found: release-mcp' }, next_retry_ms: Date.now() + 12000, forwards: [] },
    { name: 'audit-console', kind: 'prompt', target: 'staging', description: 'Investigative shell with audit notes', enabled: true, status: 'available', session_id: 'ext-session-audit', shell_id: 'ext-shell-audit', forwards: [] },
  ],
  forwards: [],
  fetched_at: now,
  preview: true,
};

const demoService = {
  supported: true,
  platform: 'darwin',
  installed: true,
  running: true,
  autostart: true,
  pid: 2841,
  label: 'ai.openmcp.termcp.desktop.core',
  definition: '~/Library/LaunchAgents/ai.openmcp.termcp.desktop.core.plist',
  log_path: '~/.termcp/gui_log',
  data_dir: '~/.termcp',
  executable: '/Applications/Termcp-Desktop.app/Contents/MacOS/Termcp',
  description: 'macOS LaunchAgent',
};

const demoConnections = new Map([
  ['internal', 'kind = "internal"\n'],
  ['dev-server', 'kind = "remote"\nhost = "dev.example.test"\nuser = "developer"\nport = 22\npassword = ""\nprivate_key = """\n"""\n'],
  ['staging', 'kind = "remote"\nhost = "staging.example.test"\nuser = "operator"\nport = 22\npassword = ""\nprivate_key = """\n"""\n'],
]);

const demoExtensions = new Map(demo.extensions.map(extension => [extension.name, extension]));
const demoExtensionDocuments = new Map([
  ['docs', 'kind = "skill"\ntarget = "internal"\ndescription = "Read-only documentation window"\npaths = ["~/skills/docs/"]\n'],
  ['release-check', 'kind = "mcp"\ntarget = "dev-server"\ndescription = "Release inspection MCP server"\nmode = "pipe"\ncommand = "release-mcp"\n'],
  ['audit-console', 'kind = "prompt"\ntarget = "staging"\ndescription = "Investigative shell with audit notes"\nmode = "pty"\n'],
]);

const demoApprovals = [{ request: { id: 'approval-demo-01', session_id: 'session-demo-01', shell_id: 'shell-demo-01', source: 'mcp', kind: 'shell_input', summary: 'Run npm test', text: 'npm test', keys: ['enter'], state: 'pending', created_at: Date.now(), need: 1 }, need: 1 }];
const demoActivity = [
  { time: new Date(Date.now() - 180000).toISOString(), unix_ms: Date.now() - 180000, session_id: 'session-demo-01', shell_id: 'shell-demo-01', session_name: 'API development', source: 'ai', kind: 'a', verdict: 'input', bytes: demoHistoryRows[2].start },
  { time: new Date(Date.now() - 420000).toISOString(), unix_ms: Date.now() - 420000, session_id: 'history-demo-01', shell_id: 'history-shell-01', session_name: 'Release check', source: 'human', kind: 'i', verdict: 'input', bytes: 0 },
];
const demoClients = {
  webui: [{ id: '1', addr: '127.0.0.1', user_agent: 'Termcp Desktop', watching: 1, since: now, self: true }],
  mcp: [{ session_id: 'mcp-demo-01', name: 'Claude', version: '1.0', namespace: 'ns:desktop-preview', connected_at: now }],
};

function clone(value) {
  return JSON.parse(JSON.stringify(value));
}

function parseBody(body) {
  if (!body) return {};
  try { return JSON.parse(body); } catch { return body; }
}

function response(value, contentType = 'application/json') {
  return { status: 200, content_type: contentType, body: contentType.includes('json') ? JSON.stringify(value) : String(value), headers: {} };
}

function demoTOMLField(raw, key, fallback = '') {
  const match = String(raw).match(new RegExp(`^${key}\\s*=\\s*(.+)$`, 'm'));
  if (!match) return fallback;
  const value = match[1].trim();
  if (/^-?\d+$/.test(value)) return Number(value);
  try { return JSON.parse(value); } catch { return value.replace(/^['"]|['"]$/g, ''); }
}

function demoExtensionDocument(summary) {
  return `kind = "${summary.kind}"\ntarget = "${summary.target}"\n${summary.description ? `description = ${JSON.stringify(summary.description)}\n` : ''}enabled = ${summary.enabled !== false}\n`;
}

function demoExtensionDetail(extension) {
  return {
    extension,
    kind: extension.kind,
    mode: extension.kind === 'skill' ? '' : (extension.kind === 'mcp' ? 'pipe' : 'pty'),
    command: extension.kind === 'mcp' ? 'release-mcp' : '',
    args: [],
    allow: [],
    paths: extension.kind === 'skill' ? ['~/skills/docs/'] : [],
    notes: '',
    failures: extension.failure ? [extension.failure] : [],
    forward_decls: [],
  };
}

function demoAPI(method, rawPath, body) {
  const url = new URL(rawPath, 'http://preview.local');
  const parts = url.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const payload = parseBody(body);
  if (method === 'GET' && url.pathname === '/api/extensions') return response({ extensions: [...demoExtensions.values()] });
  if (method === 'GET' && url.pathname === '/api/extensions/batch') {
    const names = (url.searchParams.get('names') || '').split(',').filter(Boolean);
    const selected = names.length ? names : [...demoExtensionDocuments.keys()];
    return response(selected.filter(name => demoExtensionDocuments.has(name)).map(name => `[[extensions]]\nname = "${name}"\n${demoExtensionDocuments.get(name)}`).join('\n'), 'application/toml');
  }
  if (method === 'POST' && url.pathname === '/api/extensions/batch') {
    const document = String(body || '');
    const entries = document.split(/\[\[extensions\]\]/).slice(1);
    const renamed = [];
    for (const entry of entries) {
      const original = demoTOMLField(entry, 'name');
      if (!original) continue;
      let name = original;
      for (let suffix = 2; demoExtensions.has(name); suffix += 1) name = `${original}-${suffix}`;
      if (name !== original) renamed.push({ from: original, to: name });
      const extension = { name, kind: demoTOMLField(entry, 'kind', 'custom'), target: demoTOMLField(entry, 'target', ''), description: demoTOMLField(entry, 'description'), enabled: demoTOMLField(entry, 'enabled', true) !== false, status: demoTOMLField(entry, 'enabled', true) !== false ? 'idle' : 'disabled', forwards: [] };
      demoExtensions.set(name, extension);
      demoExtensionDocuments.set(name, demoExtensionDocument(extension));
    }
    return response({ imported: entries.length, renamed });
  }
  if (method === 'POST' && url.pathname === '/api/extensions') {
    const name = String(payload.name || url.searchParams.get('name') || '').trim();
    if (!name) return response('name is required', 'text/plain');
    const extension = { name, kind: payload.kind || 'custom', target: payload.target || '', description: payload.description || '', enabled: payload.enabled !== false, status: payload.enabled === false ? 'disabled' : 'idle', forwards: [] };
    demoExtensions.set(name, extension);
    demoExtensionDocuments.set(name, demoExtensionDocument(extension));
    return response({ extension });
  }
  if (parts[1] === 'extensions' && parts[2]) {
    const name = parts[2];
    const extension = demoExtensions.get(name);
    if (!extension) return response({ error: 'extension not found' }, 'application/json');
    if (method === 'GET' && url.searchParams.get('format') === 'toml') return response(demoExtensionDocuments.get(name) || demoExtensionDocument(extension), 'application/toml');
    if (method === 'GET') return response(demoExtensionDetail(extension));
    if (method === 'PUT') {
      if (typeof payload === 'object' && payload) {
        Object.assign(extension, {
          kind: payload.kind ?? extension.kind,
          target: payload.target ?? extension.target,
          description: payload.description ?? extension.description,
          enabled: payload.enabled ?? extension.enabled,
        });
        extension.status = extension.enabled ? (extension.status === 'available' ? extension.status : 'idle') : 'disabled';
        demoExtensionDocuments.set(name, demoExtensionDocument(extension));
      } else {
        extension.description = demoTOMLField(body, 'description', extension.description);
        extension.kind = demoTOMLField(body, 'kind', extension.kind);
        extension.target = demoTOMLField(body, 'target', extension.target);
        demoExtensionDocuments.set(name, String(body || ''));
      }
      return response({ extension });
    }
    if (method === 'DELETE') {
      demoExtensions.delete(name);
      demoExtensionDocuments.delete(name);
      return response('');
    }
    if (method === 'POST' && parts[3] === 'retry') {
      if (!extension.enabled) return response({ ok: false, error: 'extension is disabled; enable it before retrying' });
      extension.status = extension.name === 'release-check' ? 'unavailable' : 'idle';
      return response({ ok: true });
    }
  }
  if (method === 'GET' && url.pathname === '/api/connections') return response({ connections: demo.connections });
  if (parts[1] === 'connections' && parts[2]) {
    const name = parts[2];
    if (method === 'GET') return response(demoConnections.get(name) || '', 'text/plain');
    if (method === 'PUT') {
      const raw = String(body || ''); const from = url.searchParams.get('from');
      if (from && from !== name) { demoConnections.delete(from); demo.connections = demo.connections.filter(connection => connection.name !== from); }
      demoConnections.set(name, raw);
      const summary = { name, kind: demoTOMLField(raw, 'kind', 'remote'), host: demoTOMLField(raw, 'host'), user: demoTOMLField(raw, 'user'), port: demoTOMLField(raw, 'port', 22), description: demoTOMLField(raw, 'description'), default_approval: demoTOMLField(raw, 'default_approval', false) };
      const index = demo.connections.findIndex(connection => connection.name === name);
      if (index >= 0) demo.connections[index] = summary; else demo.connections.push(summary);
      return response('', 'text/plain');
    }
    if (method === 'DELETE') { demoConnections.delete(name); demo.connections = demo.connections.filter(c => c.name !== name); return response(''); }
  }
  if (method === 'POST' && url.pathname === '/api/connections/test') return response({ ok: true, latency_ms: 42 });
  if (method === 'GET' && url.pathname === '/api/approvals') return response({ approvals: demoApprovals, count: demoApprovals.length });
  if (parts[1] === 'approvals' && parts[2] && method === 'POST') {
    const item = demoApprovals.find(value => value.request.id === parts[2]);
    if (item) item.request.state = parts[3] === 'approve' ? 'approved' : 'rejected';
    return response({ request: item?.request || null });
  }
  if (method === 'GET' && url.pathname === '/api/sessions') return response({ sessions: [...demo.sessions, ...demo.history] });
  if (parts[1] === 'sessions' && parts[2] && parts[3] === 'shells') {
    const session = [...demo.sessions, ...demo.history].find(s => s.id === parts[2]);
    if (method === 'GET') return response({ shells: session?.shells || [] });
    if (method === 'POST') {
      const shell = { id: `shell-demo-${Date.now()}`, name: payload.name || 'shell', status: 'running', mode: payload.mode || 'pty' };
      if (session) session.shells.push(shell);
      return response({ session_id: session?.id, shell_id: shell.id, name: shell.name });
    }
  }
  if (parts[1] === 'sessions' && parts[2] && parts[3] === 'forwards') {
    if (method === 'GET') return response({ forwards: demo.forwards.filter(f => f.session_id === parts[2]) });
    if (method === 'POST') {
      const forward = { id: `forward-demo-${Date.now()}`, session_id: parts[2], ...payload };
      demo.forwards.push(forward); return response(forward);
    }
  }
  if (parts[1] === 'sessions' && parts[2] && parts[3] === 'files') {
    if (parts[4] === 'default-directory') return response({ directory: '/home/developer' });
    const requested = url.searchParams.get('path') || '/';
    return response({ name: requested.split('/').pop() || '/', is_dir: true, size: 0, children: [
      { name: 'src', is_dir: true, size: 0, mod_time: now },
      { name: 'README.md', is_dir: false, size: 3480, mod_time: now },
      { name: 'termcp.log', is_dir: false, size: 12480, mod_time: now },
    ] });
  }
  if (parts[1] === 'sessions' && parts[2]) {
    const session = [...demo.sessions, ...demo.history].find(s => s.id === parts[2]);
    if (parts[3] === 'approval') {
      if (method === 'PATCH' && session) session.approval_mode = Boolean(payload.enabled);
      return response({ session_id: session?.id, approval_mode: Boolean(session?.approval_mode), requests: demoApprovals.filter(value => value.request.session_id === parts[2]).map(value => value.request) });
    }
    if (method === 'GET') return response(session || {});
    if (method === 'PATCH' && session) { session.name = payload.name || session.name; return response(session); }
    if (method === 'POST' && ['terminate', 'disconnect'].includes(parts[3]) && session) {
      session.status = 'dead';
      session.updated_at = Date.now();
      session.shells.forEach(shell => { shell.status = 'exited'; });
      demo.sessions = demo.sessions.filter(s => s.id !== session.id);
      demo.history.push(session);
      return response({ ok: true });
    }
    if (method === 'DELETE') { demo.sessions = demo.sessions.filter(s => s.id !== parts[2]); demo.history = demo.history.filter(s => s.id !== parts[2]); return response(''); }
  }
  if (method === 'POST' && url.pathname === '/api/sessions') {
    const session = { id: `session-demo-${Date.now()}`, name: payload.name || payload.ssh_config, status: 'running', ssh_endpoint: payload.ssh_config, created_at: Date.now(), updated_at: Date.now(), shells: [] };
    const shell = { id: `shell-demo-${Date.now()}`, name: 'shell', status: 'running', mode: payload.mode || 'pty' };
    session.shells.push(shell); demo.sessions.push(session); return response({ session_id: session.id, shell_id: shell.id });
  }
  if (method === 'GET' && url.pathname === '/api/forwards') return response({ forwards: demo.forwards });
  if (parts[1] === 'forwards' && parts[2] && method === 'DELETE') { demo.forwards = demo.forwards.filter(f => f.id !== parts[2]); return response(''); }
  if (parts[1] === 'shells' && parts[3] === 'output-range') {
    const output = parts[2] === 'history-shell-01' ? demoHistoryLog : 'Termcp preview\r\n$ go test ./...\r\nok  termcp/gui\r\n$ ';
    const bytes = new TextEncoder().encode(output);
    const start = url.searchParams.has('tail') ? 0 : Number(url.searchParams.get('start') || 0);
    const chunk = bytes.slice(start, start + Number(url.searchParams.get('max') || bytes.length));
    return response({ start, end: start + chunk.length, total: bytes.length, d: new TextDecoder().decode(chunk) });
  }
  if (parts[1] === 'shells' && parts[3] === 'rail') {
    const cols = Number(url.searchParams.get('cols'));
    const top = Number(url.searchParams.get('top')) || 0;
    const count = Number(url.searchParams.get('count')) || 0;
    return response({ cols, top, total_rows: demoHistoryRows.length,
      spans: Array.from({ length: count }, (_, index) => demoHistoryRows[top + index] || null), marks: demoHistoryMarks });
  }
  if (parts[1] === 'shells' && parts[2] && parts[3] === 'archive' && method === 'POST') {
    for (const session of demo.sessions) {
      const shell = (session.shells || []).find(item => item.id === parts[2]);
      if (!shell) continue;
      session.shells = session.shells.filter(item => item.id !== parts[2]);
      shell.status = 'exited';
      shell.archived = true;
      demo.history.push({ ...session, status: 'running', archived_shells_only: true, shells: [shell] });
      break;
    }
    return response('');
  }
  if (parts[1] === 'shells' && method === 'DELETE') return response('');
  return response({});
}

export const core = {
  bridge,
  preview: !bridge.available(),
  log(level, message, details = '') { reportFrontend(level, message, details); },
  async snapshot() {
    return bridge.available() ? bridge.call('GetSnapshot') : clone(demo);
  },
  async conversationIndex(sessionID, shellID) {
    if (bridge.available()) return bridge.call('GetConversationIndex', sessionID, shellID);
    return shellID === 'history-shell-01' ? clone(demoHistoryMarks) : [];
  },
  async activity(sessionID = '', limit = 200) {
    if (bridge.available()) return bridge.call('GetActivity', sessionID, limit);
    return clone(demoActivity.filter(event => !sessionID || event.session_id === sessionID).slice(0, limit));
  },
  async clients() {
    return bridge.available() ? bridge.call('GetClients') : clone(demoClients);
  },
  async saveConversationLog(sessionID, shellID) {
    if (!bridge.available()) throw new Error(t('Local downloads are disabled in browser preview'));
    return bridge.call('SaveConversationLog', sessionID, shellID);
  },
  async serviceStatus() {
    if (bridge.available()) return bridge.call('GetServiceStatus');
    return { ...clone(demoService), core: clone(demo.core) };
  },
  async api(method, path, body = '', contentType = '') {
    const upper = method.toUpperCase();
    const raw = typeof body === 'string' ? body : JSON.stringify(body);
    const result = bridge.available()
      ? await bridge.call('API', { method: upper, path, body: raw, content_type: contentType || (raw ? 'application/json' : '') })
      : demoAPI(upper, path, raw);
    let data = result.body || '';
    if ((result.content_type || '').includes('application/json') && data) {
      try { data = JSON.parse(data); } catch { /* Some legacy Core text endpoints advertise JSON. */ }
    }
    return { ...result, data };
  },
  async websocketURL() {
    return bridge.available() ? bridge.call('CoreWebSocketURL') : '';
  },
  async upload(sessionID, directory) {
    if (!bridge.available()) return { local_name: 'preview.txt', remote_path: `${directory.replace(/\/$/, '')}/preview.txt`, bytes: 128 };
    return bridge.call('ChooseAndUploadFile', sessionID, directory);
  },
  async defaultDirectory(sessionID) {
    const result = await this.api('GET', `/api/sessions/${encodeURIComponent(sessionID)}/files/default-directory`);
    return result.data?.directory || '';
  },
  async save(path, filename) {
    if (!bridge.available()) throw new Error(t('Local downloads are disabled in browser preview'));
    return bridge.call('SaveAPIResource', path, filename);
  },
  async exportConnections(names, password = '') {
    if (bridge.available()) return bridge.call('ExportConnections', names, password);
    return { path: 'termcp-connections.toml', count: names.length, cancelled: false };
  },
  async importConnections(password = '') {
    if (bridge.available()) return bridge.call('ImportConnections', password);
    return {
      file_name: 'preview-connections.toml',
      cancelled: false,
      imported: 2,
      renamed: 1,
      failed: 0,
      hosts: [
        { name: 'dev-server', status: 'imported' },
        { name: 'staging', final_name: 'staging-2', status: 'renamed' },
      ],
    };
  },
  async archiveShell(shellID) {
    if (bridge.available()) return bridge.call('ArchiveShell', shellID);
    return this.api('POST', `/api/shells/${encodeURIComponent(shellID)}/archive`);
  },
  async installService(autostart = true) {
    if (bridge.available()) return bridge.call('InstallCoreService', autostart);
    Object.assign(demoService, { installed: true, running: true, autostart, pid: 2841 });
  },
  async uninstallService() {
    if (bridge.available()) return bridge.call('UninstallCoreService');
    Object.assign(demoService, { installed: false, running: false, autostart: false, pid: 0 });
  },
  async start() {
    if (bridge.available()) return bridge.call('StartLocalCore');
    demoService.running = true; demoService.pid = 2841; demo.core.running = true;
  },
  async stop() {
    if (bridge.available()) return bridge.call('StopLocalCore');
    demoService.running = false; demoService.pid = 0; demo.core.running = false;
  },
  async restart() {
    if (bridge.available()) return bridge.call('RestartLocalCore');
    demoService.running = true; demoService.pid = 2842; demo.core.running = true;
  },
  async setAutostart(enabled) {
    if (bridge.available()) return bridge.call('SetCoreAutostart', enabled);
    demoService.autostart = enabled;
  },
  async setLanguage(language) {
    if (bridge.available()) return bridge.call('SetUILanguage', language);
    return language;
  },
  async requestApprovalNotificationPermission() {
    if (bridge.available()) return bridge.call('RequestApprovalNotificationPermission');
    if (typeof Notification === 'undefined') return false;
    return (Notification.permission === 'granted' ? 'granted' : await Notification.requestPermission()) === 'granted';
  },
  async sendApprovalNotification(requestID, title, body) {
    if (bridge.available()) return bridge.call('SendApprovalNotification', requestID, title, body);
    if (typeof Notification === 'undefined' || Notification.permission !== 'granted') return;
    new Notification(title, { body });
  },
  async systemFonts() {
    if (bridge.available()) return bridge.call('SystemFonts');
    return ['system-ui', 'Arial', 'Helvetica Neue', 'SF Pro Text', 'Segoe UI', 'Noto Sans', 'PingFang SC', 'Microsoft YaHei', 'Georgia', 'Times New Roman', 'Courier New', 'Menlo', 'Monaco'];
  },
  window(action) {
    if (!bridge.available()) return;
    const methods = { min: 'WindowMinimise', max: 'WindowToggleMaximise', close: 'WindowClose' };
    bridge.call(methods[action]);
  },
};
