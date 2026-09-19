import { t } from './i18n.js';

const bridge = {
  available: () => Boolean(window.go?.main?.App),
  call(name, ...args) {
    const fn = window.go?.main?.App?.[name];
    return fn ? fn(...args) : Promise.reject(new Error('Wails bridge unavailable'));
  },
};

const now = new Date().toISOString();
const demo = {
  core: { running: true, managed: true, address: 'http://127.0.0.1:18765', state: 'running' },
  connections: [
    { name: 'internal', kind: 'internal', description: 'Core 所在本机' },
    { name: 'dev-server', kind: 'remote', host: 'dev.example.test', user: 'developer', port: 22, description: '开发环境' },
    { name: 'staging', kind: 'remote', host: 'staging.example.test', user: 'operator', port: 22, description: '预发布环境' },
  ],
  sessions: [
    { id: 'session-demo-01', name: 'API 开发', status: 'running', mode: 'pty', ssh_endpoint: 'remote', created_at: now, updated_at: now, shells: [
      { id: 'shell-demo-01', name: 'build', status: 'running', mode: 'pty' },
      { id: 'shell-demo-02', name: 'logs', status: 'running', mode: 'pty' },
    ] },
    { id: 'session-demo-02', name: '本地工作区', status: 'running', mode: 'pty', ssh_endpoint: 'internal', created_at: now, updated_at: now, shells: [
      { id: 'shell-demo-03', name: 'zsh', status: 'running', mode: 'pty' },
    ] },
  ],
  history: [
    { id: 'history-demo-01', name: '发布检查', status: 'archived', reason: 'explicit', ssh_endpoint: 'remote', updated_at: now, tags: ['release'], notes: '演示历史记录' },
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
  log_path: '~/.termcp/logs/termcp-desktop-core.log',
  data_dir: '~/.termcp',
  executable: '/Applications/Termcp-Desktop.app/Contents/MacOS/Termcp',
  description: 'macOS LaunchAgent',
};

const demoConnections = new Map([
  ['dev-server', 'kind = "remote"\nhost = "dev.example.test"\nuser = "developer"\nport = 22\npassword = ""\nprivate_key = """\n"""\n'],
  ['staging', 'kind = "remote"\nhost = "staging.example.test"\nuser = "operator"\nport = 22\npassword = ""\nprivate_key = """\n"""\n'],
]);

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

function demoAPI(method, rawPath, body) {
  const url = new URL(rawPath, 'http://preview.local');
  const parts = url.pathname.split('/').filter(Boolean).map(decodeURIComponent);
  const payload = parseBody(body);
  if (method === 'GET' && url.pathname === '/api/connections') return response({ connections: demo.connections });
  if (method === 'GET' && url.pathname === '/api/connection-templates') return response({ remote: 'kind = "remote"\nhost = ""\nuser = ""\nport = 22\npassword = ""\nprivate_key = """\n"""', internal: 'kind = "internal"' });
  if (parts[1] === 'connections' && parts[2]) {
    const name = parts[2];
    if (method === 'GET') return response(demoConnections.get(name) || '', 'text/plain');
    if (method === 'PUT') { demoConnections.set(name, String(body || '')); return response('', 'text/plain'); }
    if (method === 'DELETE') { demoConnections.delete(name); demo.connections = demo.connections.filter(c => c.name !== name); return response(''); }
  }
  if (method === 'POST' && url.pathname === '/api/connections/test') return response({ ok: true, latency_ms: 42 });
  if (method === 'GET' && url.pathname === '/api/sessions') return response({ sessions: demo.sessions });
  if (parts[1] === 'sessions' && parts[2] && parts[3] === 'shells') {
    const session = demo.sessions.find(s => s.id === parts[2]);
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
    const requested = url.searchParams.get('path') || '/';
    return response({ name: requested.split('/').pop() || '/', is_dir: true, size: 0, children: [
      { name: 'src', is_dir: true, size: 0, mod_time: now },
      { name: 'README.md', is_dir: false, size: 3480, mod_time: now },
      { name: 'termcp.log', is_dir: false, size: 12480, mod_time: now },
    ] });
  }
  if (parts[1] === 'sessions' && parts[2]) {
    const session = demo.sessions.find(s => s.id === parts[2]);
    if (method === 'GET') return response(session || {});
    if (method === 'PATCH' && session) { session.name = payload.name || session.name; return response(session); }
    if ((method === 'POST' && ['terminate', 'disconnect'].includes(parts[3])) || method === 'DELETE') {
      demo.sessions = demo.sessions.filter(s => s.id !== parts[2]); return response('');
    }
  }
  if (method === 'POST' && url.pathname === '/api/sessions') {
    const session = { id: `session-demo-${Date.now()}`, name: payload.name || payload.ssh_config, status: 'running', mode: payload.mode || 'pty', ssh_endpoint: payload.ssh_config === 'internal' ? 'internal' : 'remote', shells: [] };
    const shell = { id: `shell-demo-${Date.now()}`, name: 'shell', status: 'running', mode: session.mode };
    session.shells.push(shell); demo.sessions.push(session); return response({ session_id: session.id, shell_id: shell.id });
  }
  if (method === 'GET' && url.pathname === '/api/history') return response({ sessions: demo.history });
  if (method === 'GET' && url.pathname === '/api/history/search') return response({ hits: demo.history.filter(h => JSON.stringify(h).toLowerCase().includes((url.searchParams.get('q') || '').toLowerCase())) });
  if (parts[1] === 'history' && parts[2]) {
    const item = demo.history.find(h => h.id === parts[2]);
    if (parts[3] === 'transcript') return response('$ npm test\n✓ all tests passed\n', 'text/plain');
    if (method === 'GET') return response(item || {});
    if (method === 'PATCH' && item) { Object.assign(item, payload); return response(item); }
    if (method === 'DELETE') { demo.history = demo.history.filter(h => h.id !== parts[2]); return response(''); }
  }
  if (method === 'GET' && url.pathname === '/api/forwards') return response({ forwards: demo.forwards });
  if (parts[1] === 'forwards' && parts[2] && method === 'DELETE') { demo.forwards = demo.forwards.filter(f => f.id !== parts[2]); return response(''); }
  if (method === 'GET' && url.pathname === '/api/notifications') return response({ notifications: [{ id: 'notify-demo-01', session_id: 'session-demo-01', shell_id: 'shell-demo-01', event: 'process_exit', created_at: now }] });
  if (parts[1] === 'notifications' && method === 'DELETE') return response({ ok: true });
  if (parts[1] === 'shells' && parts[3] === 'output-range') return response({ start: 0, end: 74, total: 74, d: btoa('Termcp preview\r\n$ go test ./...\r\nok  termcp/gui\r\n$ ') });
  if (parts[1] === 'shells' && method === 'DELETE') return response('');
  return response({});
}

export const core = {
  bridge,
  preview: !bridge.available(),
  async snapshot() {
    return bridge.available() ? bridge.call('GetSnapshot') : clone(demo);
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
  async save(path, filename) {
    if (!bridge.available()) throw new Error(t('浏览器预览不执行本机下载'));
    return bridge.call('SaveAPIResource', path, filename);
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
  window(action) {
    if (!bridge.available()) return;
    const methods = { min: 'WindowMinimise', max: 'WindowToggleMaximise', close: 'WindowClose' };
    bridge.call(methods[action]);
  },
};
