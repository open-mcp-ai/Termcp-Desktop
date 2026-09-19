import './styles.css';
import { core } from './core.js';
import { TerminalController } from './terminal.js';

const icons = {
  resources: '<path d="M4 5h6l2 2h8v12H4z"/><path d="M4 9h16"/>',
  sessions: '<rect x="4" y="5" width="16" height="14" rx="2"/><path d="m8 10 2 2-2 2m5 0h3"/>',
  history: '<circle cx="12" cy="12" r="8"/><path d="M12 8v5l3 2M4 5v4h4"/>',
  terminal: '<path d="m5 7 4 5-4 5m7 0h7"/>',
  core: '<path d="M8 3h8v4h4v10h-4v4H8v-4H4V7h4z"/><circle cx="12" cy="12" r="3"/>',
  service: '<rect x="4" y="4" width="16" height="6" rx="2"/><rect x="4" y="14" width="16" height="6" rx="2"/><path d="M8 7h.01M8 17h.01M12 7h5M12 17h5"/>',
  settings: '<circle cx="12" cy="12" r="3"/><path d="M12 3v3m0 12v3M3 12h3m12 0h3m-2.6-6.4-2.1 2.1M7.7 16.3l-2.1 2.1m12.8 0-2.1-2.1M7.7 7.7 5.6 5.6"/>',
  refresh: '<path d="M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-2l2 2M18 17a7 7 0 0 1-12 2l-2-2"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  search: '<circle cx="11" cy="11" r="6"/><path d="m16 16 4 4"/>',
  chevron: '<path d="m9 6 6 6-6 6"/>',
  edit: '<path d="m4 20 4-1 11-11-3-3L5 16z"/><path d="m14 6 3 3"/>',
  trash: '<path d="M5 7h14M9 7V4h6v3m-8 0 1 13h8l1-13"/>',
  split: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/>',
  folder: '<path d="M3 6h7l2 2h9v11H3z"/>',
  forward: '<path d="M5 7h11m0 0-3-3m3 3-3 3M19 17H8m0 0 3-3m-3 3 3 3"/>',
  bell: '<path d="M6 16h12l-2-3V9a4 4 0 0 0-8 0v4z"/><path d="M10 19h4"/>',
  download: '<path d="M12 3v12m0 0 4-4m-4 4-4-4M5 20h14"/>',
};

const icon = (name, size = 17) => `<svg viewBox="0 0 24 24" width="${size}" height="${size}" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${icons[name] || ''}</svg>`;
const esc = (value = '') => String(value).replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
const dot = (status = 'running') => `<i class="status-dot ${esc(status)}"></i>`;
const encode = value => encodeURIComponent(String(value));
const fmtSize = bytes => {
  let size = Number(bytes || 0); const units = ['B', 'KB', 'MB', 'GB', 'TB']; let index = 0;
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1; }
  return `${index ? size.toFixed(1) : size} ${units[index]}`;
};
const parentPath = value => {
  const clean = String(value || '/').replace(/\/+$/, '') || '/';
  if (clean === '/') return '/';
  const parts = clean.split('/'); parts.pop(); return parts.join('/') || '/';
};
const joinPath = (base, name) => `${String(base || '/').replace(/\/+$/, '')}/${name}`.replace(/^\/+/, '/');

function loadWorkspaces() {
  try {
    const parsed = JSON.parse(localStorage.getItem('termcp-gui-workspaces') || '[]');
    if (Array.isArray(parsed) && parsed.length) return parsed;
  } catch {}
  return [{ id: 'workspace-main', name: 'SSH 工作台', panes: [], maximized: '' }];
}

const state = {
  data: { core: { running: false, managed: false, address: 'http://127.0.0.1:18765', state: 'starting' }, connections: [], sessions: [], history: [], forwards: [] },
  loading: true,
  error: '',
  section: 'resources',
  filter: '',
  selected: { type: 'core', id: 'core' },
  expanded: new Set(['connection:internal']),
  dialog: null,
  workspaces: loadWorkspaces(),
  activeWorkspace: localStorage.getItem('termcp-gui-active-workspace') || 'workspace-main',
  inspector: { tab: 'files', path: '/', data: null, loading: false, error: '', sessionID: '' },
  historyQuery: '',
  historyTranscript: '',
  wsStatus: core.preview ? 'preview' : 'connecting',
  service: { supported: true, platform: '', installed: false, running: false, autostart: false, pid: 0, label: '', definition: '', log_path: '', executable: '', description: '' },
};

let refreshTimer;
const terminals = new TerminalController(core, {
  status(status) { state.wsStatus = status; updateConnectionBadge(); },
  sessions() { clearTimeout(refreshTimer); refreshTimer = setTimeout(() => refresh({ quiet: true }), 180); },
  notify(message) { toast(message.message || message.text || 'Core 通知'); },
});

function saveWorkspaces() {
  localStorage.setItem('termcp-gui-workspaces', JSON.stringify(state.workspaces));
  localStorage.setItem('termcp-gui-active-workspace', state.activeWorkspace);
}

function shellCount() {
  return state.data.sessions.reduce((count, session) => count + (session.shells || []).filter(shell => shell.status === 'running').length, 0);
}

function sessionByID(id) { return state.data.sessions.find(session => session.id === id); }
function shellByID(id) {
  for (const session of state.data.sessions) {
    const shell = (session.shells || []).find(item => item.id === id);
    if (shell) return { ...shell, session };
  }
  return null;
}
function connectionByName(name) { return state.data.connections.find(item => item.name === name); }
function historyByID(id) { return state.data.history.find(item => item.id === id); }
function activeWorkspace() {
  let workspace = state.workspaces.find(item => item.id === state.activeWorkspace);
  if (!workspace) {
    workspace = state.workspaces[0] || { id: `workspace-${Date.now()}`, name: 'SSH 工作台', panes: [], maximized: '' };
    if (!state.workspaces.length) state.workspaces.push(workspace);
    state.activeWorkspace = workspace.id;
  }
  return workspace;
}
function activePane() {
  const workspace = activeWorkspace();
  const shellID = workspace.activeShell || workspace.panes[0]?.shellID;
  return workspace.panes.find(pane => pane.shellID === shellID) || workspace.panes[0];
}

async function refresh({ quiet = false } = {}) {
  if (!quiet) { state.loading = true; state.error = ''; render(); }
  try {
    const [snapshot, service] = await Promise.all([core.snapshot(), core.serviceStatus()]);
    state.data = { ...snapshot, preview: core.preview, connections: snapshot.connections || [], sessions: snapshot.sessions || [], history: snapshot.history || [], forwards: snapshot.forwards || [] };
    state.service = { ...state.service, ...service };
    reconcileWorkspaces();
  } catch (error) {
    state.error = String(error);
  } finally {
    state.loading = false;
    render();
  }
}

function reconcileWorkspaces() {
  for (const workspace of state.workspaces) {
    workspace.panes = (workspace.panes || []).filter(pane => shellByID(pane.shellID));
    if (!workspace.panes.some(pane => pane.shellID === workspace.activeShell)) workspace.activeShell = workspace.panes[0]?.shellID || '';
    if (workspace.maximized && !workspace.panes.some(pane => pane.shellID === workspace.maximized)) workspace.maximized = '';
  }
  saveWorkspaces();
}

function selectedResource() {
  const { type, id } = state.selected;
  if (type === 'connection') return connectionByName(id);
  if (type === 'session') return sessionByID(id);
  if (type === 'shell') return shellByID(id);
  if (type === 'history') return historyByID(id);
  return state.data.core;
}

function rail() {
  const items = [['resources', '资源', 'resources'], ['workspace', '工作台', 'terminal'], ['sessions', '会话', 'sessions'], ['history', '历史', 'history'], ['service', '系统服务', 'service']];
  return `<aside class="rail"><div class="rail-logo">t_</div><nav aria-label="主导航">${items.map(([key, label, glyph]) => `<button data-section="${key}" class="${state.section === key ? 'active' : ''}" title="${label}" aria-label="${label}">${icon(glyph)}</button>`).join('')}</nav><div class="rail-bottom"><button data-select="core:core" title="Core 管理">${icon('core')}</button><button data-section="settings" class="${state.section === 'settings' ? 'active' : ''}" title="设置">${icon('settings')}</button></div></aside>`;
}

function explorer() {
  return `<aside class="explorer"><div class="explorer-head"><div><span>资源管理器</span><small>LOCAL TERMCP</small></div><div class="mini-actions"><button class="icon-btn" data-action="new-connection" title="新建连接">${icon('plus', 14)}</button><button class="icon-btn" data-action="new-session" title="新建会话">${icon('terminal', 14)}</button></div></div><label class="search">${icon('search', 14)}<input type="search" placeholder="筛选资源" value="${esc(state.filter)}" data-filter></label><div class="tree-scroll">${tree(state.filter.trim().toLowerCase())}</div><button class="core-card ${state.selected.type === 'core' ? 'selected' : ''}" data-select="core:core"><span>${dot(state.data.core.running ? 'running' : 'error')}<b>Local Core</b><small>${coreMode()}</small></span><span class="core-counts"><b>${state.data.sessions.length}</b> 会话 · <b>${shellCount()}</b> Shell</span></button></aside>`;
}

function coreMode() { return state.service.installed ? '系统服务' : '应用内运行'; }

const group = (name, content) => `<section class="tree-group"><header><span>⌄</span>${name}</header>${content || '<p>没有资源</p>'}</section>`;
const matches = (value, query) => !query || String(value).toLowerCase().includes(query);

function tree(query) {
  if (state.section === 'history') {
    const items = state.data.history.filter(item => matches(`${item.name} ${(item.tags || []).join(' ')}`, query));
    return group('历史记录', items.map(historyNode).join(''));
  }
  if (state.section === 'sessions') {
    return group('运行中的会话', state.data.sessions.filter(item => matches(`${item.name} ${item.id}`, query)).map(sessionNode).join(''));
  }
  if (state.section === 'workspace') {
    return group('工作区', state.workspaces.map(workspaceNode).join('')) + group('可用 Shell', state.data.sessions.flatMap(session => (session.shells || []).map(shell => shellNode(shell, session))).join(''));
  }
  if (state.section === 'service') {
    const service = state.service;
    return group('本机服务', `<button class="tree-row leaf selected" data-section="service">${icon('service', 13)}<span><b>Core 系统服务</b><small>${service.installed ? (service.running ? '已注册 · 运行中' : '已注册 · 已停止') : '未注册'}</small></span>${dot(service.running ? 'running' : 'error')}</button>`);
  }
  if (state.section === 'settings') return '<div class="tree-empty">应用、Core 与 API</div>';
  const connections = state.data.connections.filter(item => matches(`${item.name} ${item.host || ''} ${item.user || ''}`, query));
  const sessions = state.data.sessions.filter(item => matches(`${item.name} ${item.id}`, query));
  const history = state.data.history.filter(item => matches(`${item.name} ${item.reason || ''}`, query)).slice(0, 5);
  return group('连接配置', connections.map(connectionNode).join('')) + group('运行中的会话', sessions.map(sessionNode).join('')) + group('最近历史', history.map(historyNode).join(''));
}

function connectionNode(connection) {
  const key = `connection:${connection.name}`;
  const open = state.expanded.has(key);
  const sessions = state.data.sessions.filter(session => session.ssh_endpoint === connection.name || (connection.kind === 'internal' && session.ssh_endpoint === 'internal'));
  return `<div class="tree-node"><div class="tree-row-wrap"><button class="disclosure-button ${open ? 'open' : ''}" data-toggle="${esc(key)}" aria-label="${open ? '收起' : '展开'} ${esc(connection.name)}">${icon('chevron', 12)}</button><button class="tree-row ${state.selected.type === 'connection' && state.selected.id === connection.name ? 'selected' : ''}" data-select="connection:${esc(connection.name)}"><span class="resource-icon ${esc(connection.kind)}">${connection.kind === 'internal' ? '›_' : '⌁'}</span><span><b>${esc(connection.name)}</b><small>${esc(connection.kind === 'internal' ? '本机' : connection.host || 'SSH')}</small></span>${dot('running')}</button></div>${open ? `<div class="tree-children">${sessions.map(sessionNode).join('') || '<p>没有可归属的会话</p>'}</div>` : ''}</div>`;
}

function sessionNode(session) {
  const key = `session:${session.id}`;
  const open = state.expanded.has(key);
  return `<div class="tree-node"><div class="tree-row-wrap"><button class="disclosure-button ${open ? 'open' : ''}" data-toggle="${esc(key)}" aria-label="${open ? '收起' : '展开'} ${esc(session.name)}">${icon('chevron', 11)}</button><button class="tree-row sub ${state.selected.type === 'session' && state.selected.id === session.id ? 'selected' : ''}" data-select="session:${esc(session.id)}">${dot(session.status)}<span><b>${esc(session.name)}</b><small>${esc(session.mode)} · ${(session.shells || []).length} Shell</small></span></button></div>${open ? `<div class="tree-children shells">${(session.shells || []).map(shell => shellNode(shell, session)).join('') || '<p>没有 Shell</p>'}</div>` : ''}</div>`;
}

function shellNode(shell, session) {
  return `<button class="tree-row leaf ${state.selected.type === 'shell' && state.selected.id === shell.id ? 'selected' : ''}" data-open-shell="${esc(shell.id)}" data-session="${esc(session.id)}"><span class="terminal-glyph">›_</span><span><b>${esc(shell.name || 'shell')}</b><small>${esc(session.name)} · ${esc(shell.status)}</small></span></button>`;
}

const historyNode = item => `<button class="tree-row leaf ${state.selected.type === 'history' && state.selected.id === item.id ? 'selected' : ''}" data-select="history:${esc(item.id)}">${icon('history', 13)}<span><b>${esc(item.name)}</b><small>${esc(item.reason || item.status || 'archived')}</small></span></button>`;
const workspaceNode = workspace => `<button class="tree-row leaf ${state.activeWorkspace === workspace.id ? 'selected' : ''}" data-workspace="${esc(workspace.id)}">${icon('terminal', 13)}<span><b>${esc(workspace.name)}</b><small>${workspace.panes.length} 个窗格</small></span></button>`;

const header = (title, eyebrow, actions = '') => `<header class="content-head"><div><div class="eyebrow">${eyebrow}</div><h1>${esc(title)}</h1></div><div class="head-actions">${actions}</div></header>`;
const button = (label, action, kind = '', extra = '') => `<button class="button ${kind}" data-action="${action}" ${extra}>${label}</button>`;

function content() {
  if (state.section === 'workspace') return workspacePage();
  if (state.section === 'service') return servicePage();
  if (state.section === 'settings') return settingsPage();
  if (state.section === 'history' && state.selected.type !== 'history') return historyIndexPage();
  const resource = selectedResource();
  const pages = { core: corePage, connection: connectionPage, session: sessionPage, shell: shellPage, history: historyPage };
  return (pages[state.selected.type] || emptyPage)(resource);
}

function corePage(status) {
  const restart = status.running ? button(`${icon('refresh', 14)}重启 Core`, 'restart-core') : button('启动 Core', 'start-core', 'primary');
  return `${header('Local Core', 'CORE INSTANCE', `${button(`${icon('plus', 14)}新建连接`, 'new-connection')}${restart}`)}<div class="metrics"><article><span>运行状态</span><b>${dot(status.running ? 'running' : 'error')}${status.running ? '运行中' : '已停止'}</b><small>${coreMode()}</small></article><article><span>活跃会话</span><strong>${state.data.sessions.length}</strong><small>${shellCount()} 个运行中 Shell</small></article><article><span>端口转发</span><strong>${state.data.forwards.length}</strong><small>跟随会话生命周期</small></article></div><section class="panel core-overview"><div><span>本机服务地址</span><code>${esc(status.address)}</code></div><div><span>运行模式</span><b>${coreMode()}</b></div><div><span>事件通道</span><b>${esc(state.wsStatus)}</b></div></section>${resourceGrid()}`;
}

function servicePage() {
  const service = state.service;
  const registered = service.installed;
  const actions = !service.supported ? '' : registered
    ? `${service.running ? button('停止', 'stop-core') : button('启动', 'start-core', 'primary')}${button(`${icon('refresh', 14)}重启`, 'restart-core')}${button('卸载服务', 'uninstall-service', 'danger')}`
    : `${button('注册服务', 'install-service')}${button(`${icon('plus', 14)}注册并自启`, 'install-service-autostart', 'primary')}`;
  const platformNames = { darwin: 'macOS · LaunchAgent', linux: 'Linux · systemd --user', windows: 'Windows · Service Control Manager' };
  const platform = platformNames[service.platform] || service.platform || '当前平台';
  const supportNotice = service.supported ? '' : '<div class="error-banner">当前平台不支持系统服务管理。</div>';
  const autostartScope = service.platform === 'windows' ? '跟随系统启动' : '跟随当前用户登录启动';
  return `${header('Core 系统服务', 'LOCAL SERVICE', actions)}${supportNotice}<div class="service-metrics"><article><span>注册状态</span><b>${registered ? '已注册' : '未注册'}</b><small>${esc(platform)}</small></article><article><span>进程状态</span><b>${dot(service.running ? 'running' : 'error')}${service.running ? '运行中' : '已停止'}</b><small>${service.pid ? `PID ${service.pid}` : '没有服务进程'}</small></article><article><span>开机自启</span><b>${service.autostart ? '已开启' : '已关闭'}</b><small>${autostartScope}</small></article></div><section class="panel service-control"><div class="service-control-main"><span class="service-symbol">${icon('service', 25)}</span><div><b>本机 termcp Core</b><small>固定监听 127.0.0.1:18765，由 termcp-gui 的同一可执行文件提供后台服务。</small></div>${registered ? `<label class="switch"><input type="checkbox" data-service-autostart ${service.autostart ? 'checked' : ''}><span></span><em>开机自启</em></label>` : ''}</div><dl><div><dt>服务标识</dt><dd><code>${esc(service.label || '—')}</code></dd></div><div><dt>管理方式</dt><dd>${esc(service.description || platform)}</dd></div><div><dt>服务定义</dt><dd><code>${esc(service.definition || '—')}</code></dd></div><div><dt>可执行文件</dt><dd><code>${esc(service.executable || '—')}</code></dd></div><div><dt>日志</dt><dd><code>${esc(service.log_path || (service.platform === 'windows' ? 'Windows Event Log' : '—'))}</code></dd></div></dl></section><div class="service-note"><b>本机管理边界</b><p>GUI 只管理本机 Core。注册服务后，关闭桌面窗口不会停止 Core；卸载服务会自动切回应用内运行。SSH 主机仍作为连接资源由本机 Core 管理。</p></div>`;
}

function resourceGrid() {
  const rows = [['⌁', state.data.connections.length, '连接配置'], ['▣', state.data.sessions.length, '活跃会话'], ['›_', shellCount(), '运行中 Shell'], ['◷', state.data.history.length, '历史记录']];
  return `<div class="section-title"><h2>资源总览</h2><button class="text-btn" data-section="resources">在树中查看</button></div><div class="resource-grid">${rows.map(row => `<article><span class="resource-big">${row[0]}</span><div><b>${row[1]}</b><small>${row[2]}</small></div></article>`).join('')}</div>`;
}

function connectionPage(connection) {
  if (!connection) return emptyPage();
  const sessions = state.data.sessions.filter(session => session.ssh_endpoint === connection.name || (connection.kind === 'internal' && session.ssh_endpoint === 'internal'));
  const manage = connection.kind === 'internal' ? '' : `${button(`${icon('edit', 14)}编辑`, 'edit-connection', '', `data-name="${esc(connection.name)}"`)}${button('测试', 'test-saved-connection', '', `data-name="${esc(connection.name)}"`)}${button(`${icon('trash', 14)}删除`, 'delete-connection', 'danger', `data-name="${esc(connection.name)}"`)}`;
  return `${header(connection.name, 'SSH CONNECTION', `${manage}${button(`${icon('terminal', 14)}连接`, 'connect', 'primary', `data-connection="${esc(connection.name)}"`)}`)}<section class="connection-hero"><div class="connection-symbol">${connection.kind === 'internal' ? '›_' : '⌁'}</div><div><span>${connection.kind === 'internal' ? '本机连接' : 'SSH 主机'}</span><h2>${esc(connection.kind === 'internal' ? 'Core 所在本机' : `${connection.user || ''}@${connection.host || ''}:${connection.port || 22}`)}</h2><p>${esc(connection.description || '连接凭据由 termcp Core 管理。只有主动编辑时才读取配置正文。')}</p></div>${dot('running')}</section><div class="section-title"><h2>关联会话</h2><span>${sessions.length}</span></div><div class="session-cards">${sessions.map(sessionCard).join('') || '<div class="empty-state">Core 未返回可归属到此配置的会话；所有运行中会话仍在资源树中。</div>'}</div>`;
}

function sessionPage(session) {
  if (!session) return emptyPage();
  const running = session.status === 'running';
  const actions = `${running ? button(`${icon('plus', 14)}新建 Shell`, 'new-shell', '', `data-id="${esc(session.id)}"`) : ''}${button(`${icon('edit', 14)}重命名`, 'rename-session', '', `data-id="${esc(session.id)}"`)}${running ? button('结束并归档', 'terminate-session', 'danger', `data-id="${esc(session.id)}"`) : ''}${button(`${icon('trash', 14)}永久删除`, 'purge-session', 'danger', `data-id="${esc(session.id)}"`)}`;
  return `${header(session.name, running ? 'ACTIVE SESSION' : 'DEAD SESSION', actions)}<div class="session-summary"><div><span>${dot(session.status)}${esc(session.status)}</span><code>termcp://#${esc(session.id)}</code></div><dl><div><dt>连接类型</dt><dd>${esc(session.ssh_endpoint || 'remote')}</dd></div><div><dt>终端模式</dt><dd>${esc(session.mode)}</dd></div><div><dt>Shell</dt><dd>${(session.shells || []).length}</dd></div></dl></div><div class="section-title"><h2>Shell 资源</h2><span>共享一条连接</span></div><div class="shell-grid">${(session.shells || []).map((shell, index) => shellCard(shell, session, index)).join('') || '<div class="empty-state">会话还没有 Shell。</div>'}</div>`;
}

function shellPage(item) {
  if (!item) return emptyPage();
  const close = item.status === 'running' ? button(`${icon('trash', 14)}关闭 Shell`, 'close-shell', 'danger', `data-shell="${esc(item.id)}"`) : '';
  return `${header(item.name || 'shell', 'TERMINAL SHELL', `${button(`${icon('terminal', 14)}在工作台打开`, 'open-selected-shell', 'primary', `data-shell="${esc(item.id)}" data-session="${esc(item.session.id)}"`)}${close}`)}<section class="panel shell-detail"><dl><div><dt>所属会话</dt><dd>${esc(item.session.name)}</dd></div><div><dt>状态</dt><dd>${dot(item.status)} ${esc(item.status)}</dd></div><div><dt>模式</dt><dd>${esc(item.mode)}</dd></div><div><dt>资源地址</dt><dd><code>termcp://#${esc(item.session.id)}:${esc(item.id)}</code></dd></div></dl></section>`;
}

const sessionCard = session => `<button class="session-card" data-select="session:${esc(session.id)}"><span class="session-icon">${icon('terminal', 18)}</span><span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint)} · ${(session.shells || []).length} Shell</small></span>${dot(session.status)}</button>`;
const shellCard = (shell, session, index) => `<button class="shell-card" data-open-shell="${esc(shell.id)}" data-session="${esc(session.id)}"><span>›_</span><div><b>${esc(shell.name || `shell-${index + 1}`)}</b><small>${esc(shell.mode)} · ${esc(shell.status)}</small><code>termcp://#${esc(session.id)}:${index + 1}</code></div>${dot(shell.status)}</button>`;

function historyIndexPage() {
  return `${header('历史记录', 'SESSION ARCHIVE', button(`${icon('search', 14)}搜索`, 'history-search'))}<label class="history-search"><input value="${esc(state.historyQuery)}" placeholder="搜索名称、命令、标签和输出" data-history-query><button class="button" data-action="history-search">搜索</button></label><div class="history-list">${state.data.history.map(item => `<button data-select="history:${esc(item.id)}"><span>${icon('history', 16)}</span><span><b>${esc(item.name)}</b><small>${esc(item.updated_at || item.reason || '')}</small></span><span>${(item.tags || []).map(tag => `<em>${esc(tag)}</em>`).join('')}</span></button>`).join('') || '<div class="empty-state">没有历史记录。</div>'}</div>`;
}

function historyPage(item) {
  if (!item) return historyIndexPage();
  const transcript = state.historyTranscript ? `<pre class="transcript">${esc(state.historyTranscript)}</pre>` : '<div class="empty-state">点击“查看正文”加载完整历史输出。</div>';
  return `${header(item.name, 'SESSION HISTORY', `${button(`${icon('edit', 14)}编辑`, 'edit-history', '', `data-id="${esc(item.id)}"`)}${button('查看正文', 'load-transcript', '', `data-id="${esc(item.id)}"`)}${button(`${icon('download', 14)}导出正文`, 'export-transcript', '', `data-id="${esc(item.id)}"`)}${button('导出截图', 'export-screenshot', '', `data-id="${esc(item.id)}"`)}${button(`${icon('trash', 14)}删除`, 'delete-history', 'danger', `data-id="${esc(item.id)}"`)}`)}<section class="panel history-detail"><span class="archive-badge">只读归档</span><dl><div><dt>连接</dt><dd>${esc(item.ssh_endpoint || 'remote')}</dd></div><div><dt>原因</dt><dd>${esc(item.reason || item.status || 'archived')}</dd></div><div><dt>标签</dt><dd>${(item.tags || []).map(tag => `<em class="tag">${esc(tag)}</em>`).join(' ') || '—'}</dd></div><div><dt>备注</dt><dd>${esc(item.notes || '—')}</dd></div></dl></section><div class="section-title"><h2>历史正文</h2><span>只读</span></div>${transcript}`;
}

function settingsPage() {
  const endpoints = [
    ['连接配置', 'GET / PUT / DELETE', '/api/connections/{name}'], ['连接测试', 'POST', '/api/connections/test'], ['会话', 'GET / POST / PATCH / DELETE', '/api/sessions'], ['终端', 'WebSocket', '/api/ui/ws'], ['Shell', 'GET / POST / DELETE', '/api/sessions/{id}/shells'], ['历史', 'GET / PATCH / DELETE', '/api/history'], ['文件', 'GET / POST / PUT / DELETE', '/api/sessions/{id}/files'], ['转发', 'GET / POST / DELETE', '/api/forwards'], ['通知', 'GET / DELETE', '/api/notifications'],
  ];
  const base = state.data.core.address || 'http://127.0.0.1:18765';
  const mcp = JSON.stringify({ mcpServers: { termcp: { url: `${base}/stream` } } }, null, 2);
  return `${header('应用与 API', 'TERMCP GUI', button(`${icon('refresh', 14)}刷新`, 'refresh'))}<section class="panel settings-list"><div><span><b>Core 模式</b><small>本机 Core 可在应用内运行，或注册为独立系统服务。</small></span><em>${coreMode()}</em></div><div><span><b>终端事件通道</b><small>会话列表、终端输出、输入、resize 与 notify_user 共用 WebSocket。</small></span><em>${esc(state.wsStatus)}</em></div><div><span><b>渲染引擎</b><small>Wails 系统 WebView，不打包 Electron 或 Chromium。</small></span><em>Native WebView</em></div></section><div class="section-title"><h2>MCP 接入</h2><span>Streamable HTTP</span></div><section class="panel mcp-config"><div><span>本机服务地址</span><code>${esc(base)}/stream</code><button data-copy="${esc(`${base}/stream`)}">复制地址</button></div><pre>${esc(mcp)}</pre><button class="button" data-copy="${esc(mcp)}">复制 MCP 配置</button></section><div class="section-title"><h2>Core 接口覆盖</h2><span>${endpoints.length} 组</span></div><div class="api-table">${endpoints.map(row => `<div><b>${row[0]}</b><span>${row[1]}</span><code>${row[2]}</code></div>`).join('')}</div>`;
}
function emptyPage() { return `${header('选择资源', 'RESOURCE EXPLORER')}<div class="empty-state large">从左侧选择连接、会话、Shell 或历史记录。</div>`; }

function workspacePage() {
  const workspace = activeWorkspace();
  workspace.layout ||= 'grid'; workspace.ratio ||= 50;
  const panes = workspace.maximized ? workspace.panes.filter(pane => pane.shellID === workspace.maximized) : workspace.panes;
  const tabs = state.workspaces.map(item => `<button class="workspace-tab ${item.id === workspace.id ? 'active' : ''}" data-workspace="${esc(item.id)}"><span>${esc(item.name)}</span><small>${item.panes.length}</small>${state.workspaces.length > 1 ? `<i data-close-workspace="${esc(item.id)}">×</i>` : ''}</button>`).join('');
  const ratio = panes.length === 2 && !workspace.maximized ? `<label class="split-ratio" title="调整分屏比例"><input type="range" min="20" max="80" value="${workspace.ratio}" data-split-ratio><span>${workspace.ratio}%</span></label>` : '';
  return `<div class="workspace-page"><div class="workspace-tabs">${tabs}<button data-action="new-workspace" title="新建工作区">＋</button></div><div class="workspace-toolbar"><span>${dot(state.wsStatus === 'connected' || core.preview ? 'running' : 'error')} ${core.preview ? '浏览器预览' : `终端通道 ${state.wsStatus}`}</span><div>${ratio}${button(`${icon('plus', 13)}添加窗格`, 'add-pane')}${button('左右', 'layout-columns')}${button('上下', 'layout-rows')}${button(`${icon('split', 13)}平铺`, 'tile-panes')}${button(`${icon('edit', 13)}重命名`, 'rename-workspace')}</div></div>${panes.length ? `<div class="terminal-workspace layout-${esc(workspace.layout)} ${workspace.maximized ? 'maximized' : ''}" style="--pane-count:${panes.length};--split-ratio:${workspace.ratio}%">${panes.map(pane => terminalPane(pane, workspace)).join('')}</div>` : `<div class="workspace-empty"><span>›_</span><h2>空工作区</h2><p>从资源树选择 Shell，或在现有会话中创建一个。</p>${button('添加 Shell', 'add-pane', 'primary')}</div>`}<aside class="workspace-inspector">${inspectorShell()}</aside></div>`;
}

function terminalPane(pane, workspace) {
  const item = shellByID(pane.shellID);
  if (!item) return '';
  const active = workspace.activeShell === pane.shellID;
  return `<section class="terminal-pane ${active ? 'active' : ''}" data-pane-shell="${esc(pane.shellID)}"><header><button class="pane-title" data-active-pane="${esc(pane.shellID)}">${dot(item.status)}<span><b>${esc(item.session.name)}</b><small>${esc(item.name || 'shell')}</small></span></button><div><button data-action="new-shell" data-id="${esc(item.session.id)}" title="同连接新建 Shell">＋</button><button data-max-pane="${esc(pane.shellID)}" title="${workspace.maximized ? '恢复平铺' : '最大化'}">${workspace.maximized ? '❐' : '□'}</button><button data-remove-pane="${esc(pane.shellID)}" title="移除窗格">×</button></div></header><div class="terminal-host" data-terminal-shell="${esc(pane.shellID)}"></div></section>`;
}

function inspectorShell() {
  const pane = activePane();
  const item = pane && shellByID(pane.shellID);
  if (!item) return '<div class="inspector-empty">选择终端窗格后可管理文件、转发和通知。</div>';
  const tabs = [['files', '文件', 'folder'], ['forwards', '转发', 'forward'], ['notifications', '通知', 'bell']];
  return `<header><div><span>会话工具</span><b>${esc(item.session.name)}</b></div><code>${esc(item.name)}</code></header><nav>${tabs.map(([key, label, glyph]) => `<button data-inspector-tab="${key}" class="${state.inspector.tab === key ? 'active' : ''}">${icon(glyph, 13)}${label}</button>`).join('')}</nav><div class="inspector-body">${state.inspector.loading ? '<div class="inspector-empty">加载中…</div>' : inspectorBody(item.session)}</div>`;
}

function inspectorBody(session) {
  if (state.inspector.error) return `<div class="inspector-error">${esc(state.inspector.error)}</div>`;
  if (state.inspector.tab === 'files') return fileInspector(session);
  if (state.inspector.tab === 'forwards') return forwardInspector(session);
  return notificationInspector(session);
}

function fileInspector(session) {
  const data = state.inspector.data;
  const rows = data?.is_dir ? (data.children || []).map(file => {
    const full = joinPath(state.inspector.path, file.name);
    return `<div class="file-item"><button data-file-open="${esc(full)}" data-is-dir="${file.is_dir ? '1' : '0'}"><span>${file.is_dir ? '▰' : '▤'}</span><span><b>${esc(file.name)}</b><small>${file.is_dir ? '目录' : fmtSize(file.size)}</small></span></button><div>${!file.is_dir ? `<button data-file-download="${esc(full)}" title="下载">${icon('download', 12)}</button>` : ''}<button data-file-rename="${esc(full)}" title="重命名">${icon('edit', 12)}</button><button data-file-delete="${esc(full)}" title="删除">${icon('trash', 12)}</button></div></div>`;
  }).join('') : '';
  return `<div class="file-path"><button data-file-up title="上一级">↑</button><input value="${esc(state.inspector.path)}" data-file-path><button data-file-browse>前往</button></div><div class="inspector-actions"><button data-file-upload>${icon('plus', 12)}上传</button><button data-action="new-directory">${icon('folder', 12)}新建目录</button></div><div class="file-list">${data ? (data.is_dir ? rows || '<div class="inspector-empty">空目录</div>' : `<div class="file-preview"><b>${esc(data.name)}</b><span>${fmtSize(data.size)}</span><button data-file-download="${esc(state.inspector.path)}">下载文件</button></div>`) : '<div class="inspector-empty">输入绝对路径并前往。</div>'}</div><small class="inspector-foot">${esc(session.ssh_endpoint)} · SFTP / 本地文件 API</small>`;
}

function forwardInspector(session) {
  const forwards = state.inspector.data?.forwards || [];
  return `<div class="inspector-actions"><button data-action="new-forward">${icon('plus', 12)}新建转发</button></div><div class="forward-list">${forwards.map(forward => `<article><div><b>${esc(forward.direction || forward.kind)}</b><small>${esc(formatForward(forward))}</small></div><button data-forward-delete="${esc(forward.forward_id || forward.id)}">${icon('trash', 12)}</button></article>`).join('') || '<div class="inspector-empty">当前会话没有端口转发。</div>'}</div><small class="inspector-foot">会话 ${esc(session.name)} 结束时自动关闭</small>`;
}

function notificationInspector() {
  const notifications = state.inspector.data?.notifications || [];
  return `<div class="notification-list">${notifications.map(rule => `<article><div><b>${esc(rule.event || rule.kind || '通知规则')}</b><small>${esc(rule.shell_id || rule.session_id || '')}</small></div><button data-notification-delete="${esc(rule.rule_id || rule.id)}">${icon('trash', 12)}</button></article>`).join('') || '<div class="inspector-empty">没有活跃的通知规则。规则由 MCP shell_notify 注册。</div>'}</div>`;
}

function formatForward(forward) {
  if (forward.listen_addr || forward.target_addr) return `${forward.listen_addr || '—'}${forward.target_addr ? ` → ${forward.target_addr}` : ''}`;
  if (forward.direction === 'dynamic') return `127.0.0.1:${forward.local_port || 0}`;
  return `${forward.local_host || '127.0.0.1'}:${forward.local_port || 0} → ${forward.remote_host || ''}:${forward.remote_port || 0}`;
}

function modal() {
  const dialog = state.dialog;
  if (!dialog) return '';
  let body = '';
  let title = '';
  let eyebrow = '';
  let footer = '';
  if (dialog.type === 'session') {
    title = '连接主机'; eyebrow = 'NEW SESSION';
    body = `<label>连接配置<select name="connection">${state.data.connections.map(item => `<option value="${esc(item.name)}" ${dialog.connection === item.name ? 'selected' : ''}>${esc(item.name)} · ${esc(item.kind === 'internal' ? '本机' : item.host)}</option>`).join('')}</select></label><label>会话名称<input name="name" placeholder="默认使用连接配置名称"></label><label>启动命令<input name="command" placeholder="留空打开默认 Shell"></label><label>终端模式<select name="mode"><option value="pty">PTY 交互终端</option><option value="pipe">Pipe 非交互</option></select></label>`;
    footer = `<button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">连接并打开</button>`;
  } else if (dialog.type === 'shell') {
    title = '新建 Shell'; eyebrow = 'SESSION CHANNEL';
    body = `<input type="hidden" name="session" value="${esc(dialog.session)}"><label>Shell 名称<input name="name" value="shell" required></label><label>启动命令<input name="command" placeholder="留空使用默认 Shell"></label><label>终端模式<select name="mode"><option value="pty">PTY</option><option value="pipe">Pipe</option></select></label>`;
    footer = `<button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">创建</button>`;
  } else if (dialog.type === 'connection') {
    title = dialog.original ? `编辑 ${dialog.original}` : '新建 SSH 连接'; eyebrow = 'SSH PROFILE';
    body = `<label>配置名称<input name="name" value="${esc(dialog.name || '')}" pattern="[A-Za-z0-9_-]+" required ${dialog.original ? '' : 'autofocus'}></label><label>配置内容（TOML）<textarea name="config" rows="16" spellcheck="false" required>${esc(dialog.raw || '')}</textarea></label><div class="modal-result">${esc(dialog.result || '密码和私钥只写入 Core 配置；列表接口不会返回凭据。')}</div>`;
    footer = `<button type="button" class="button" data-action="test-editor-connection">测试连接</button><span class="modal-spacer"></span><button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">保存</button>`;
  } else if (dialog.type === 'forward') {
    title = '新建端口转发'; eyebrow = 'SSH FORWARD';
    body = `<input type="hidden" name="session" value="${esc(dialog.session)}"><label>方向<select name="direction"><option value="local">Local · -L</option><option value="remote">Remote · -R</option><option value="dynamic">Dynamic · -D</option></select></label><div class="form-grid"><label>本地地址<input name="local_host" value="127.0.0.1"></label><label>本地端口<input name="local_port" type="number" min="0" max="65535" value="0"></label><label>远端地址<input name="remote_host" value="127.0.0.1"></label><label>远端端口<input name="remote_port" type="number" min="0" max="65535" value="80"></label></div>`;
    footer = `<button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">创建</button>`;
  } else if (dialog.type === 'history') {
    title = '编辑历史记录'; eyebrow = 'SESSION ARCHIVE';
    body = `<input type="hidden" name="id" value="${esc(dialog.item.id)}"><label>名称<input name="name" value="${esc(dialog.item.name)}" required></label><label>标签<input name="tags" value="${esc((dialog.item.tags || []).join(', '))}" placeholder="release, production"></label><label>备注<textarea name="notes" rows="5">${esc(dialog.item.notes || '')}</textarea></label>`;
    footer = `<button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">保存</button>`;
  } else if (dialog.type === 'rename') {
    title = dialog.title || '重命名'; eyebrow = 'RENAME';
    body = `<input type="hidden" name="target" value="${esc(dialog.target || '')}"><input type="hidden" name="id" value="${esc(dialog.id || '')}"><label>新名称<input name="name" value="${esc(dialog.value || '')}" required autofocus></label>`;
    footer = `<button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">保存</button>`;
  } else if (dialog.type === 'directory') {
    title = '新建目录'; eyebrow = 'FILE MANAGER';
    body = `<label>目录路径<input name="path" value="${esc(joinPath(state.inspector.path, 'new-directory'))}" required autofocus></label>`;
    footer = `<button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">创建</button>`;
  } else if (dialog.type === 'add-pane') {
    title = '添加终端窗格'; eyebrow = 'WORKSPACE';
    const options = state.data.sessions.flatMap(session => (session.shells || []).map(shell => `<option value="${esc(shell.id)}" data-session="${esc(session.id)}">${esc(session.name)} / ${esc(shell.name || 'shell')}</option>`)).join('');
    body = `<label>Shell<select name="shell" required>${options}</select></label>`;
    footer = `<button type="button" class="button" data-close>取消</button><button class="button primary" type="submit">添加</button>`;
  } else if (dialog.type === 'confirm') {
    title = dialog.title; eyebrow = 'CONFIRM'; body = `<p>${esc(dialog.message)}</p>`;
    footer = `<button type="button" class="button" data-close>取消</button><button type="button" class="button danger" data-confirm="${esc(dialog.action)}" data-payload="${esc(dialog.payload || '')}">${esc(dialog.confirm || '确认')}</button>`;
  }
  return `<div class="modal-backdrop"><form class="modal ${dialog.type === 'connection' ? 'wide' : ''}" data-form="${esc(dialog.type)}"><header><div><span>${eyebrow}</span><h2>${esc(title)}</h2></div><button type="button" class="icon-btn" data-close>×</button></header>${body}<footer>${footer}</footer></form></div>`;
}

function render() {
  terminals.clear();
  document.querySelector('#app').innerHTML = `<div class="app-shell"><header class="titlebar" style="--wails-draggable:drag"><div class="title-brand"><span>t_</span><b>termcp gui</b></div><div class="title-status" id="connection-badge">${dot(state.data.core.running ? 'running' : 'error')}<span>${state.data.core.running ? 'Core 运行中' : 'Core 已停止'}</span><small>${coreMode()}</small></div><div class="window-controls" style="--wails-draggable:no-drag"><button data-window="min" aria-label="最小化">—</button><button data-window="max" aria-label="最大化">□</button><button data-window="close" aria-label="关闭">×</button></div></header><div class="body">${rail()}${explorer()}<main class="content ${state.section === 'workspace' ? 'workspace-content' : ''}">${state.error ? `<div class="error-banner">${esc(state.error)}<button data-action="refresh">重试</button></div>` : ''}<div class="content-toolbar"><span>${core.preview ? '浏览器预览 · 脱敏演示资源' : '本机 Core 数据'}</span><button class="icon-btn ${state.loading ? 'spinning' : ''}" data-action="refresh" title="刷新资源">${icon('refresh', 15)}</button></div>${content()}</main></div></div>${modal()}`;
  if (state.section === 'workspace') mountWorkspace();
}

function updateConnectionBadge() {
  const element = document.querySelector('#connection-badge small');
  if (element && state.section === 'workspace') element.textContent = state.wsStatus;
}

async function mountWorkspace() {
  const workspace = activeWorkspace();
  for (const pane of workspace.panes) {
    const shell = shellByID(pane.shellID);
    const element = document.querySelector(`[data-terminal-shell="${CSS.escape(pane.shellID)}"]`);
    if (shell && element) terminals.mount(pane.shellID, element, shell.status !== 'running');
  }
  await loadInspector();
}

async function loadInspector() {
  if (state.section !== 'workspace') return;
  const pane = activePane(); const shell = pane && shellByID(pane.shellID);
  if (!shell) return;
  state.inspector.sessionID = shell.session.id;
  state.inspector.loading = true; state.inspector.error = '';
  const body = document.querySelector('.inspector-body'); if (body) body.innerHTML = '<div class="inspector-empty">加载中…</div>';
  try {
    if (state.inspector.tab === 'files') {
      const result = await core.api('GET', `/api/sessions/${encode(shell.session.id)}/files?path=${encode(state.inspector.path)}`);
      state.inspector.data = result.data;
    } else if (state.inspector.tab === 'forwards') {
      const result = await core.api('GET', `/api/sessions/${encode(shell.session.id)}/forwards`);
      state.inspector.data = result.data;
    } else {
      const result = await core.api('GET', `/api/notifications?session_id=${encode(shell.session.id)}`);
      state.inspector.data = result.data;
    }
  } catch (error) { state.inspector.error = String(error); state.inspector.data = null; }
  state.inspector.loading = false;
  const next = document.querySelector('.inspector-body'); if (next) next.innerHTML = inspectorBody(shell.session);
}

function openShell(shellID, sessionID) {
  let workspace = activeWorkspace();
  if (workspace.panes.length >= 4 && !workspace.panes.some(pane => pane.shellID === shellID)) {
    workspace = { id: `workspace-${Date.now()}`, name: sessionByID(sessionID)?.name || 'SSH 工作台', panes: [], maximized: '' };
    state.workspaces.push(workspace); state.activeWorkspace = workspace.id;
  }
  if (!workspace.panes.some(pane => pane.shellID === shellID)) workspace.panes.push({ shellID, sessionID });
  workspace.activeShell = shellID; workspace.maximized = '';
  state.section = 'workspace'; state.selected = { type: 'shell', id: shellID };
  saveWorkspaces(); render();
}

function toast(message) {
  const element = document.querySelector('#toast');
  if (!element) return;
  element.textContent = String(message); element.hidden = false;
  clearTimeout(toast.timer); toast.timer = setTimeout(() => { element.hidden = true; }, 3200);
}

async function run(label, operation, options = {}) {
  try {
    const result = await operation();
    if (label) toast(label);
    if (options.refresh !== false) await refresh({ quiet: true });
    else if (options.render) render();
    return result;
  } catch (error) { toast(String(error)); return null; }
}

async function openConnectionEditor(name = '') {
  state.dialog = { type: 'connection', name, original: name, raw: '', result: '' }; render();
  try {
    if (name) {
      const result = await core.api('GET', `/api/connections/${encode(name)}`);
      state.dialog.raw = result.data || result.body || '';
    } else {
      const result = await core.api('GET', '/api/connection-templates');
      state.dialog.raw = result.data?.remote || '';
    }
  } catch (error) { state.dialog.result = String(error); }
  render();
}

async function testConnection(raw, label = true) {
  const result = await core.api('POST', '/api/connections/test', raw, 'text/plain; charset=utf-8');
  if (label) toast(result.data?.ok ? `连接成功${result.data.latency_ms ? ` · ${result.data.latency_ms} ms` : ''}` : '连接测试完成');
  return result;
}

function confirmDialog(title, message, action, payload, confirm = '确认') {
  state.dialog = { type: 'confirm', title, message, action, payload, confirm }; render();
}

document.addEventListener('click', async event => {
  const toggle = event.target.closest('[data-toggle]');
  if (toggle) { const key = toggle.dataset.toggle; state.expanded.has(key) ? state.expanded.delete(key) : state.expanded.add(key); render(); return; }
  const closeWorkspace = event.target.closest('[data-close-workspace]');
  if (closeWorkspace) { event.stopPropagation(); state.workspaces = state.workspaces.filter(item => item.id !== closeWorkspace.dataset.closeWorkspace); state.activeWorkspace = state.workspaces[0]?.id || ''; saveWorkspaces(); render(); return; }
  const target = event.target.closest('button');
  if (!target) return;
  if (target.dataset.section) { state.section = target.dataset.section; if (state.section === 'history') state.selected = { type: 'history-index', id: '' }; if (state.section === 'settings') state.selected = { type: 'settings', id: '' }; if (state.section === 'service') state.selected = { type: 'service', id: 'local' }; render(); return; }
  if (target.dataset.select) { const [type, ...parts] = target.dataset.select.split(':'); state.selected = { type, id: parts.join(':') }; if (type === 'history') state.historyTranscript = ''; render(); return; }
  if (target.dataset.openShell) { openShell(target.dataset.openShell, target.dataset.session); return; }
  if (target.dataset.workspace) { state.activeWorkspace = target.dataset.workspace; state.section = 'workspace'; saveWorkspaces(); render(); return; }
  if (target.dataset.copy !== undefined) { try { await navigator.clipboard.writeText(target.dataset.copy); toast('已复制'); } catch { toast('复制失败'); } return; }
  if (target.dataset.window) { core.window(target.dataset.window); return; }
  if (target.dataset.close !== undefined) { state.dialog = null; render(); return; }
  if (target.dataset.activePane) { const workspace = activeWorkspace(); workspace.activeShell = target.dataset.activePane; state.inspector.path = '/'; saveWorkspaces(); render(); return; }
  if (target.dataset.maxPane) { const workspace = activeWorkspace(); workspace.maximized = workspace.maximized ? '' : target.dataset.maxPane; saveWorkspaces(); render(); return; }
  if (target.dataset.removePane) { const workspace = activeWorkspace(); workspace.panes = workspace.panes.filter(pane => pane.shellID !== target.dataset.removePane); workspace.maximized = ''; workspace.activeShell = workspace.panes[0]?.shellID || ''; saveWorkspaces(); render(); return; }
  if (target.dataset.inspectorTab) { state.inspector.tab = target.dataset.inspectorTab; state.inspector.data = null; render(); return; }
  if (target.dataset.fileBrowse !== undefined) { const input = document.querySelector('[data-file-path]'); state.inspector.path = input?.value.trim() || '/'; await loadInspector(); return; }
  if (target.dataset.fileUp !== undefined) { state.inspector.path = parentPath(state.inspector.path); await loadInspector(); return; }
  if (target.dataset.fileOpen) { if (target.dataset.isDir === '1') { state.inspector.path = target.dataset.fileOpen; await loadInspector(); } else await downloadFile(target.dataset.fileOpen); return; }
  if (target.dataset.fileDownload) { await downloadFile(target.dataset.fileDownload); return; }
  if (target.dataset.fileDelete) { confirmDialog('删除文件', `确认删除 ${target.dataset.fileDelete}？`, 'delete-file', target.dataset.fileDelete, '删除'); return; }
  if (target.dataset.fileRename) { state.dialog = { type: 'rename', title: '重命名文件', target: 'file', id: target.dataset.fileRename, value: target.dataset.fileRename.split('/').pop() }; render(); return; }
  if (target.dataset.fileUpload !== undefined) { await run('文件已上传', () => core.upload(state.inspector.sessionID, state.inspector.path), { refresh: false }); await loadInspector(); return; }
  if (target.dataset.forwardDelete) { await run('转发已关闭', () => core.api('DELETE', `/api/forwards/${encode(target.dataset.forwardDelete)}`), { refresh: false }); await loadInspector(); return; }
  if (target.dataset.notificationDelete) { await run('通知规则已移除', () => core.api('DELETE', `/api/notifications/${encode(target.dataset.notificationDelete)}`), { refresh: false }); await loadInspector(); return; }
  if (target.dataset.confirm) { const action = target.dataset.confirm; const payload = target.dataset.payload; state.dialog = null; await executeConfirmed(action, payload); return; }

  const action = target.dataset.action;
  if (!action) return;
  if (action === 'refresh') { await refresh(); return; }
  if (action === 'install-service') { await run('系统服务已注册并启动', () => core.installService(false)); return; }
  if (action === 'install-service-autostart') { await run('系统服务已注册，开机自启已开启', () => core.installService(true)); return; }
  if (action === 'uninstall-service') { confirmDialog('卸载 Core 系统服务', '系统服务将停止并移除，Core 会自动切回应用内运行。', 'uninstall-service', '', '卸载服务'); return; }
  if (action === 'start-core') { await run('Core 已启动', () => core.start()); return; }
  if (action === 'stop-core') { await run('Core 已停止', () => core.stop()); return; }
  if (action === 'restart-core') { await run('Core 已重启', () => core.restart()); return; }
  if (action === 'new-connection') { await openConnectionEditor(); return; }
  if (action === 'edit-connection') { await openConnectionEditor(target.dataset.name); return; }
  if (action === 'test-saved-connection') {
    await run('', async () => { const raw = await core.api('GET', `/api/connections/${encode(target.dataset.name)}`); return testConnection(raw.data || raw.body); }, { refresh: false }); return;
  }
  if (action === 'delete-connection') { confirmDialog('删除连接配置', `确认删除 ${target.dataset.name}？现有会话不会立即终止。`, 'delete-connection', target.dataset.name, '删除'); return; }
  if (action === 'connect' || action === 'new-session') { state.dialog = { type: 'session', connection: target.dataset.connection || '' }; render(); return; }
  if (action === 'new-shell') { state.dialog = { type: 'shell', session: target.dataset.id }; render(); return; }
  if (action === 'terminate-session') { confirmDialog('结束会话', '所有 Shell 和端口转发都会关闭，输出会保留到历史记录。', 'terminate-session', target.dataset.id, '结束并归档'); return; }
  if (action === 'purge-session') { confirmDialog('永久删除会话', '会话、消息和历史记录将永久删除。', 'purge-session', target.dataset.id, '永久删除'); return; }
  if (action === 'rename-session') { const item = sessionByID(target.dataset.id); state.dialog = { type: 'rename', title: '重命名会话', target: 'session', id: item.id, value: item.name }; render(); return; }
  if (action === 'open-selected-shell') { openShell(target.dataset.shell, target.dataset.session); return; }
  if (action === 'close-shell') { confirmDialog('关闭 Shell', '当前 Shell 进程将结束；其他 Shell 和 SSH 连接保持运行。', 'close-shell', target.dataset.shell, '关闭 Shell'); return; }
  if (action === 'new-workspace') { const workspace = { id: `workspace-${Date.now()}`, name: `工作区 ${state.workspaces.length + 1}`, panes: [], maximized: '' }; state.workspaces.push(workspace); state.activeWorkspace = workspace.id; saveWorkspaces(); render(); return; }
  if (action === 'rename-workspace') { const workspace = activeWorkspace(); state.dialog = { type: 'rename', title: '重命名工作区', target: 'workspace', id: workspace.id, value: workspace.name }; render(); return; }
  if (action === 'add-pane') { state.dialog = { type: 'add-pane' }; render(); return; }
  if (action === 'layout-columns') { const workspace = activeWorkspace(); workspace.layout = 'columns'; workspace.maximized = ''; saveWorkspaces(); render(); return; }
  if (action === 'layout-rows') { const workspace = activeWorkspace(); workspace.layout = 'rows'; workspace.maximized = ''; saveWorkspaces(); render(); return; }
  if (action === 'tile-panes') { const workspace = activeWorkspace(); workspace.layout = 'grid'; workspace.maximized = ''; saveWorkspaces(); render(); return; }
  if (action === 'new-directory') { state.dialog = { type: 'directory' }; render(); return; }
  if (action === 'new-forward') { state.dialog = { type: 'forward', session: state.inspector.sessionID }; render(); return; }
  if (action === 'edit-history') { state.dialog = { type: 'history', item: historyByID(target.dataset.id) }; render(); return; }
  if (action === 'load-transcript') { await loadTranscript(target.dataset.id); return; }
  if (action === 'export-transcript') { await run('已保存历史正文', () => core.save(`/api/history/${encode(target.dataset.id)}/transcript?format=markdown`, `${historyByID(target.dataset.id)?.name || 'history'}.md`), { refresh: false }); return; }
  if (action === 'export-screenshot') { await run('已保存历史截图', () => core.save(`/api/history/${encode(target.dataset.id)}/screenshot?start=0&lines=80&cols=120&theme=dark`, `${historyByID(target.dataset.id)?.name || 'history'}.png`), { refresh: false }); return; }
  if (action === 'delete-history') { confirmDialog('删除历史记录', '历史正文和关联消息将永久删除。', 'delete-history', target.dataset.id, '删除'); return; }
  if (action === 'history-search') { await searchHistory(); return; }
  if (action === 'test-editor-connection') {
    const form = target.closest('form'); const data = new FormData(form);
    try { const result = await testConnection(data.get('config'), false); state.dialog.result = result.data?.ok ? `连接成功${result.data.latency_ms ? ` · ${result.data.latency_ms} ms` : ''}` : '测试完成'; } catch (error) { state.dialog.result = String(error); }
    render(); return;
  }
});

document.addEventListener('input', event => {
  if (event.target.matches('[data-filter]')) { state.filter = event.target.value; const treeElement = document.querySelector('.tree-scroll'); if (treeElement) treeElement.innerHTML = tree(state.filter.trim().toLowerCase()); }
  if (event.target.matches('[data-history-query]')) state.historyQuery = event.target.value;
  if (event.target.matches('[data-split-ratio]')) {
    const workspace = activeWorkspace(); workspace.ratio = Number(event.target.value);
    document.querySelector('.terminal-workspace')?.style.setProperty('--split-ratio', `${workspace.ratio}%`);
    const label = event.target.nextElementSibling; if (label) label.textContent = `${workspace.ratio}%`;
    saveWorkspaces();
  }
});

document.addEventListener('change', async event => {
  if (event.target.matches('[data-service-autostart]')) {
    const enabled = event.target.checked;
    await run(enabled ? '已开启开机自启' : '已关闭开机自启', () => core.setAutostart(enabled));
  }
});

document.addEventListener('keydown', async event => {
  if (event.key === 'Enter' && event.target.matches('[data-file-path]')) { event.preventDefault(); state.inspector.path = event.target.value.trim() || '/'; await loadInspector(); }
  if (event.key === 'Enter' && event.target.matches('[data-history-query]')) { event.preventDefault(); await searchHistory(); }
  if (event.key === 'Escape' && state.dialog) { state.dialog = null; render(); }
});

document.addEventListener('submit', async event => {
  event.preventDefault();
  const form = event.target; const data = new FormData(form); const type = form.dataset.form;
  if (type === 'session') {
    state.dialog = null;
    await run('会话已创建', async () => {
      const result = await core.api('POST', '/api/sessions', { ssh_config: data.get('connection'), name: data.get('name'), command: data.get('command'), mode: data.get('mode'), rows: 24, cols: 100 });
      await refresh({ quiet: true });
      if (result.data?.shell_id) openShell(result.data.shell_id, result.data.session_id);
    }, { refresh: false }); return;
  }
  if (type === 'shell') {
    state.dialog = null;
    await run('Shell 已创建', async () => {
      const sessionID = data.get('session');
      const result = await core.api('POST', `/api/sessions/${encode(sessionID)}/shells`, { name: data.get('name'), command: data.get('command'), mode: data.get('mode'), rows: 24, cols: 100 });
      await refresh({ quiet: true }); if (result.data?.shell_id) openShell(result.data.shell_id, sessionID);
    }, { refresh: false }); return;
  }
  if (type === 'connection') {
    const name = String(data.get('name')).trim(); const original = state.dialog.original;
    state.dialog = null;
    await run('连接配置已保存', () => core.api('PUT', `/api/connections/${encode(name)}${original && original !== name ? `?from=${encode(original)}` : ''}`, data.get('config'), 'text/plain; charset=utf-8')); return;
  }
  if (type === 'forward') {
    state.dialog = null;
    const payload = { direction: data.get('direction'), local_host: data.get('local_host'), local_port: Number(data.get('local_port') || 0), remote_host: data.get('remote_host'), remote_port: Number(data.get('remote_port') || 0) };
    await run('端口转发已创建', () => core.api('POST', `/api/sessions/${encode(data.get('session'))}/forwards`, payload), { refresh: false }); await loadInspector(); return;
  }
  if (type === 'history') {
    state.dialog = null; const id = data.get('id'); const tags = String(data.get('tags') || '').split(',').map(tag => tag.trim()).filter(Boolean);
    await run('历史记录已更新', () => core.api('PATCH', `/api/history/${encode(id)}`, { name: data.get('name'), notes: data.get('notes'), tags })); return;
  }
  if (type === 'rename') {
    const target = data.get('target'); const id = data.get('id'); const name = String(data.get('name')).trim(); state.dialog = null;
    if (target === 'session') await run('会话已重命名', () => core.api('PATCH', `/api/sessions/${encode(id)}`, { name }));
    if (target === 'workspace') { const workspace = state.workspaces.find(item => item.id === id); if (workspace) workspace.name = name; saveWorkspaces(); render(); }
    if (target === 'file') { const destination = joinPath(parentPath(id), name); await run('文件已重命名', () => core.api('PUT', `/api/sessions/${encode(state.inspector.sessionID)}/files?from=${encode(id)}&to=${encode(destination)}`), { refresh: false }); await loadInspector(); }
    return;
  }
  if (type === 'directory') { state.dialog = null; await run('目录已创建', () => core.api('POST', `/api/sessions/${encode(state.inspector.sessionID)}/files/dir?path=${encode(data.get('path'))}`), { refresh: false }); await loadInspector(); return; }
  if (type === 'add-pane') { state.dialog = null; const shellID = data.get('shell'); const item = shellByID(shellID); if (item) openShell(shellID, item.session.id); }
});

async function executeConfirmed(action, payload) {
  if (action === 'uninstall-service') await run('系统服务已卸载，Core 已切回应用内运行', () => core.uninstallService());
  if (action === 'delete-connection') await run('连接配置已删除', () => core.api('DELETE', `/api/connections/${encode(payload)}`));
  if (action === 'terminate-session') await run('会话已结束并归档', () => core.api('POST', `/api/sessions/${encode(payload)}/terminate`));
  if (action === 'purge-session') await run('会话已永久删除', () => core.api('DELETE', `/api/sessions/${encode(payload)}`));
  if (action === 'delete-history') { await run('历史记录已删除', () => core.api('DELETE', `/api/history/${encode(payload)}`)); state.selected = { type: 'history-index', id: '' }; }
  if (action === 'close-shell') {
    await run('Shell 已关闭', () => core.api('DELETE', `/api/shells/${encode(payload)}`));
    for (const workspace of state.workspaces) workspace.panes = workspace.panes.filter(pane => pane.shellID !== payload);
    saveWorkspaces(); render();
  }
  if (action === 'delete-file') { await run('文件已删除', () => core.api('DELETE', `/api/sessions/${encode(state.inspector.sessionID)}/files?path=${encode(payload)}`), { refresh: false }); await loadInspector(); }
}

async function downloadFile(filePath) {
  const name = filePath.split('/').pop() || 'download';
  await run('文件已保存', () => core.save(`/api/sessions/${encode(state.inspector.sessionID)}/files/download?path=${encode(filePath)}`, name), { refresh: false });
}

async function loadTranscript(id) {
  await run('', async () => { const result = await core.api('GET', `/api/history/${encode(id)}/transcript?format=text`); state.historyTranscript = result.data || result.body || ''; }, { refresh: false, render: true });
}

async function searchHistory() {
  const query = state.historyQuery.trim();
  if (!query) { await refresh(); return; }
  await run('', async () => { const result = await core.api('GET', `/api/history/search?q=${encode(query)}&limit=100`); state.data.history = result.data?.hits || []; }, { refresh: false, render: true });
}

render();
refresh();
terminals.connect();
