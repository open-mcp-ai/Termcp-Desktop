import './styles.css';
import './theme.css';
import './features/workspace/workspace.css';
import './ui/feedback.css';
import { core } from './core.js';
import { getAppearance, setAppearance } from './appearance.js';
import { getLanguage, localizeDOM, setLanguage, t } from './i18n/index.js';
import { TerminalController } from './terminal.js';

import { dot, encode, esc, fmtSize, icon, joinPath, parentPath } from './ui/render.js';

import { connectionFormToTOML, connectionProfileFromTOML, emptyConnectionProfile } from './features/connections/profile.js';
import { loadWorkspaceState, reconcileWorkspaceState, removeShellFromWorkspace, saveCollapsedGroups as persistCollapsedGroups, saveWorkspaceState, selectShellTab } from './features/workspace/model.js';
import { renderSessionContextMenu, renderTerminalContextMenu, renderWorkspacePage } from './features/workspace/view.js';

const persistedWorkspace = loadWorkspaceState();

const state = {
  data: { core: { running: false, managed: false, address: 'http://127.0.0.1:18765', state: 'starting' }, connections: [], sessions: [], history: [], forwards: [] },
  loading: true,
  error: '',
  section: 'resources',
  filter: '',
  selected: { type: 'connection', id: '' },
  expanded: new Set(['connection:internal']),
  dialog: null,
  appearance: getAppearance(),
  contextMenu: null,
  terminalMenu: null,
  draggedWorkspace: '',
  draggedSession: '',
  draggedShell: '',
  dropRegion: '',
  workspaces: persistedWorkspace.workspaces,
  closedSessionTabs: persistedWorkspace.closedSessionTabs,
  collapsedGroups: persistedWorkspace.collapsedGroups,
  activeWorkspace: persistedWorkspace.activeWorkspace,
  inspector: { tab: 'files', path: '/', data: null, loading: false, error: '', sessionID: '', collapsed: localStorage.getItem('termcp-desktop-inspector-collapsed') === '1' },
  historyQuery: '',
  historyTranscript: '',
  wsStatus: core.preview ? 'preview' : 'connecting',
  service: { supported: true, platform: '', installed: false, running: false, autostart: false, pid: 0, label: '', definition: '', log_path: '', executable: '', description: '' },
  systemFonts: { items: ['system-ui'], loading: false, loaded: false, error: '' },
};

let refreshTimer;
let connectionClickTimer;
const terminals = new TerminalController(core, {
  status(status) { state.wsStatus = status; updateConnectionBadge(); },
  sessions() { clearTimeout(refreshTimer); refreshTimer = setTimeout(() => refresh({ quiet: true }), 180); },
  notify(message) { toast(message.message || message.text || 'Core 通知'); },
});

function saveWorkspaces() { saveWorkspaceState(state); }
function saveCollapsedGroups() { persistCollapsedGroups(state); }

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
    workspace = state.workspaces[0] || { id: `session-tab-empty`, name: '新会话', panes: [], maximized: '' };
    if (!state.workspaces.length) state.workspaces.push(workspace);
    state.activeWorkspace = workspace.id;
  }
  return workspace;
}
function workspaceSession(workspace) {
  if (!workspace) return null;
  return sessionByID(workspace.primarySessionID || workspace.panes?.[0]?.sessionID);
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
    if (state.section === 'resources' && state.selected.type === 'connection' && !connectionByName(state.selected.id)) {
      state.selected = { type: 'connection', id: state.data.connections[0]?.name || '' };
    }
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
  reconcileWorkspaceState(state, { sessionByID, shellByID, workspaceSession });
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
  const items = [['resources', '连接', 'resources'], ['workspace', '终端', 'terminal'], ['history', '历史', 'history'], ['service', '服务', 'service']];
  const item = ([key, label, glyph]) => `<button data-section="${key}" class="${state.section === key || (key === 'workspace' && state.section === 'sessions') ? 'active' : ''}" title="${label}" aria-label="${label}"><span>${icon(glyph, 19)}</span><small>${label}</small></button>`;
  return `<aside class="rail"><nav aria-label="主导航">${items.map(item).join('')}</nav><div class="rail-bottom">${item(['settings', '设置', 'settings'])}</div></aside>`;
}

function explorer() {
  const workspace = state.section === 'workspace';
  const sessionArea = workspace || state.section === 'sessions';
  return `<aside class="explorer"><div class="explorer-head"><div><span>${sessionArea ? '会话' : '连接配置'}</span><small>${sessionArea ? 'SESSION WORKSPACE' : 'DOUBLE-CLICK TO CONNECT'}</small></div><div class="mini-actions"><button class="icon-btn ${state.loading ? 'spinning' : ''}" data-action="refresh" title="刷新资源">${icon('refresh', 14)}</button>${sessionArea ? `<button class="icon-btn" data-action="new-session" title="新建会话">${icon('plus', 14)}</button>` : `<button class="icon-btn" data-action="new-connection" title="新建连接">${icon('plus', 14)}</button>`}</div></div><label class="search">${icon('search', 14)}<input type="search" placeholder="${sessionArea ? '筛选会话' : '筛选连接配置'}" value="${esc(state.filter)}" data-filter></label><div class="tree-scroll">${tree(state.filter.trim().toLowerCase())}</div></aside>`;
}

function coreMode() { return state.service.installed ? '系统服务' : '应用内运行'; }

const group = (name, content, key = name) => {
  const collapsed = state.collapsedGroups.has(key);
  return `<section class="tree-group ${collapsed ? 'collapsed' : ''}"><header><button class="tree-group-toggle" data-group-toggle="${esc(key)}" aria-expanded="${collapsed ? 'false' : 'true'}"><span>${icon('chevron', 11)}</span><b>${name}</b></button></header><div class="tree-group-content">${collapsed ? '' : (content || '<p>没有资源</p>')}</div></section>`;
};
const matches = (value, query) => !query || String(value).toLowerCase().includes(query);

function tree(query) {
  if (state.section === 'history') {
    const items = state.data.history.filter(item => matches(`${item.name} ${(item.tags || []).join(' ')}`, query));
    return group('历史记录', items.map(historyNode).join(''));
  }
  if (state.section === 'sessions' || state.section === 'workspace') {
    const sessions = state.data.sessions.filter(item => matches(`${item.name} ${item.ssh_endpoint || ''} ${item.id}`, query));
    return group('会话列表', sessions.map(sessionNode).join(''), 'session-list');
  }
  if (state.section === 'service') {
    const service = state.service;
    return group('本机服务', `<button class="tree-row leaf selected" data-section="service">${icon('service', 13)}<span><b>Core 系统服务</b><small>${service.installed ? (service.running ? '已注册 · 运行中' : '已注册 · 已停止') : '未注册'}</small></span>${dot(service.running ? 'running' : 'error')}</button>`);
  }
  if (state.section === 'settings') return '<div class="tree-empty">应用、Core 与 API</div>';
  const connections = state.data.connections.filter(item => matches(`${item.name} ${item.host || ''} ${item.user || ''}`, query));
  return group('连接配置', connections.map(connectionNode).join(''), 'connection-list');
}

function connectionNode(connection) {
  const endpoint = connection.kind === 'internal' ? '本机 Core' : `${connection.user ? `${connection.user}@` : ''}${connection.host || 'SSH'}`;
  return `<button class="tree-row connection-profile ${state.selected.type === 'connection' && state.selected.id === connection.name ? 'selected' : ''}" data-connection-profile="${esc(connection.name)}" title="双击新建会话"><span class="resource-icon ${esc(connection.kind)}">${connection.kind === 'internal' ? '›_' : '⌁'}</span><span><b>${esc(connection.name)}</b><small>${esc(endpoint)}</small></span><em>双击连接</em></button>`;
}

function sessionNode(session) {
  const key = `session:${session.id}`;
  const open = state.expanded.has(key);
  const action = state.section === 'workspace' ? `data-open-session="${esc(session.id)}"` : `data-select="session:${esc(session.id)}"`;
  return `<div class="tree-node" data-session-context="${esc(session.id)}" data-session-drag="${esc(session.id)}" draggable="true"><div class="tree-row-wrap"><button class="disclosure-button ${open ? 'open' : ''}" data-toggle="${esc(key)}" aria-label="${open ? '收起' : '展开'} ${esc(session.name)}">${icon('chevron', 11)}</button><button class="tree-row sub ${workspaceSession(activeWorkspace())?.id === session.id || (state.selected.type === 'session' && state.selected.id === session.id) ? 'selected' : ''}" ${action}>${dot(session.status)}<span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint || session.mode)} · ${(session.shells || []).length} Shell</small></span></button></div>${open ? `<div class="tree-children shells">${(session.shells || []).map(shell => shellNode(shell, session)).join('') || '<p>没有 Shell</p>'}</div>` : ''}</div>`;
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
  const pages = { connection: connectionPage, session: sessionPage, shell: shellPage, history: historyPage };
  return (pages[state.selected.type] || emptyPage)(resource);
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
  return `${header('Core 系统服务', 'LOCAL SERVICE', actions)}${supportNotice}<div class="service-metrics"><article><span>注册状态</span><b>${registered ? '已注册' : '未注册'}</b><small>${esc(platform)}</small></article><article><span>进程状态</span><b>${dot(service.running ? 'running' : 'error')}${service.running ? '运行中' : '已停止'}</b><small>${service.pid ? `PID ${service.pid}` : '没有服务进程'}</small></article><article><span>开机自启</span><b>${service.autostart ? '已开启' : '已关闭'}</b><small>${autostartScope}</small></article></div><section class="panel service-control"><div class="service-control-main"><span class="service-symbol">${icon('service', 25)}</span><div><b>本机 termcp Core</b><small>固定监听 127.0.0.1:18765，由 Termcp 的同一可执行文件提供后台服务。</small></div>${registered ? `<label class="switch"><input type="checkbox" data-service-autostart ${service.autostart ? 'checked' : ''}><span></span><em>开机自启</em></label>` : ''}</div><dl><div><dt>服务标识</dt><dd><code>${esc(service.label || '—')}</code></dd></div><div><dt>管理方式</dt><dd>${esc(service.description || platform)}</dd></div><div><dt>持久化目录</dt><dd><code>${esc(service.data_dir || '~/.termcp')}</code></dd></div><div><dt>服务定义</dt><dd><code>${esc(service.definition || '—')}</code></dd></div><div><dt>可执行文件</dt><dd><code>${esc(service.executable || '—')}</code></dd></div><div><dt>日志</dt><dd><code>${esc(service.log_path || (service.platform === 'windows' ? 'Windows Event Log' : '—'))}</code></dd></div></dl></section><div class="service-note"><b>本机管理边界</b><p>Termcp 只管理本机 Core。注册服务后，关闭桌面窗口不会停止 Core；卸载服务会自动切回应用内运行。SSH 主机仍作为连接资源由本机 Core 管理。</p></div>`;
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

function coreSettingsCard() {
  const status = state.data.core;
  const runtimeAction = status.running
    ? `${button('停止 Core', 'stop-core')}${button(`${icon('refresh', 14)}重启 Core`, 'restart-core', 'primary')}`
    : button('启动 Core', 'start-core', 'primary');
  const metrics = [
    ['连接配置', state.data.connections.length],
    ['活跃会话', state.data.sessions.length],
    ['运行中 Shell', shellCount()],
    ['端口转发', state.data.forwards.length],
    ['历史记录', state.data.history.length],
  ];
  return `<section class="settings-card core-settings-card" id="core-settings"><header><div><span>LOCAL CORE</span><h2>Core 管理</h2></div><div class="core-settings-actions">${button(`${icon('plus', 14)}新建连接`, 'new-connection')}${runtimeAction}</div></header><div class="core-settings-runtime"><div class="core-runtime-state">${dot(status.running ? 'running' : 'error')}<span><b>${status.running ? '运行中' : '已停止'}</b><small>${coreMode()}</small></span></div><dl><div><dt>本机服务地址</dt><dd><code>${esc(status.address || 'http://127.0.0.1:18765')}</code></dd></div><div><dt>运行模式</dt><dd>${coreMode()}</dd></div><div><dt>事件通道</dt><dd>${esc(state.wsStatus)}</dd></div><div><dt>系统服务</dt><dd><button class="setting-link" data-section="service">${state.service.installed ? '已注册' : '未注册'} ${icon('chevron', 14)}</button></dd></div></dl></div><div class="core-settings-metrics">${metrics.map(([label, value]) => `<article><strong>${value}</strong><span>${label}</span></article>`).join('')}</div></section>`;
}

function settingsPage() {
  const base = state.data.core.address || 'http://127.0.0.1:18765';
  const mcp = JSON.stringify({ mcpServers: { termcp: { url: `${base}/stream` } } }, null, 2);
  const appearance = state.appearance;
  const fonts = [...new Set(['system-ui', ...state.systemFonts.items])];
  const fontStatus = state.systemFonts.loading ? '正在读取系统字体…' : state.systemFonts.error ? '无法读取系统字体，可直接输入字体名称。' : `已找到 ${Math.max(0, fonts.length - 1)} 个系统字体`;
  const appearanceCard = `<section class="settings-card appearance-card"><header><div><span>APPEARANCE</span><h2>界面外观</h2></div><small>即时生效</small></header><div class="setting-row"><label><b>主题</b><small>浅色与深色使用同一套中性色阶。</small></label><select data-appearance="theme"><option value="light" ${appearance.theme === 'light' ? 'selected' : ''}>浅色</option><option value="dark" ${appearance.theme === 'dark' ? 'selected' : ''}>深色</option></select></div><div class="setting-row"><label><b>字号</b><small>使用明确的像素值，同时应用到终端。</small></label><div class="font-size-control"><select data-appearance="fontSize">${Array.from({ length: 9 }, (_, index) => index + 12).map(size => `<option value="${size}" ${appearance.fontSize === size ? 'selected' : ''}>${size}</option>`).join('')}</select><span>px</span></div></div><div class="setting-row font-setting"><label><b>系统字体</b><small>${fontStatus}</small></label><input list="system-font-families" value="${esc(appearance.fontFamily)}" data-appearance="fontFamily" data-font-family placeholder="搜索或输入字体名称"><datalist id="system-font-families">${fonts.map(font => `<option value="${esc(font)}">`).join('')}</datalist></div><div class="font-preview" data-font-preview><span>Aa 文</span><p>Termcp 让本机 Core、SSH 会话和文件管理保持在一个清晰的工作流中。</p><code>debian@host:~$ termcp</code></div></section>`;
  const applicationCard = `<section class="settings-card"><header><div><span>APPLICATION</span><h2>应用设置</h2></div></header><div class="setting-row"><label><b>界面语言</b><small>同步更新应用界面与系统托盘。</small></label><select data-language data-i18n-ignore><option value="zh-CN" ${getLanguage() === 'zh-CN' ? 'selected' : ''}>简体中文</option><option value="en" ${getLanguage() === 'en' ? 'selected' : ''}>English</option></select></div><div class="setting-row"><label><b>Core 运行方式</b><small>当前电脑上的 termcp Core 运行模式。</small></label><em>${coreMode()}</em></div><div class="setting-row"><label><b>系统服务</b><small>注册、开机自启和后台运行。</small></label><button class="setting-link" data-section="service">${state.service.installed ? '已注册' : '未注册'} ${icon('chevron', 14)}</button></div><div class="setting-row"><label><b>终端事件通道</b><small>终端输出、输入与 resize 共用 WebSocket。</small></label><em>${esc(state.wsStatus)}</em></div><div class="setting-row"><label><b>渲染引擎</b><small>Wails 系统 WebView。</small></label><em>Native WebView</em></div></section>`;
  const integrationCard = `<section class="settings-card developer-card"><header><div><span>INTEGRATIONS</span><h2>MCP 接入</h2></div><small>Streamable HTTP</small></header><div class="mcp-endpoint"><div><span>本机服务地址</span><code>${esc(base)}/stream</code></div><button data-copy="${esc(`${base}/stream`)}">复制地址</button></div><pre>${esc(mcp)}</pre><button class="button" data-copy="${esc(mcp)}">复制 MCP 配置</button></section>`;
  return `${header('设置', 'TERMCP DESKTOP', button(`${icon('refresh', 14)}刷新`, 'refresh'))}<div class="settings-grid">${coreSettingsCard()}${appearanceCard}${applicationCard}${integrationCard}</div>`;
}
function emptyPage() { return `${header('选择资源', 'RESOURCE EXPLORER')}<div class="empty-state large">从左侧选择连接、会话、Shell 或历史记录。</div>`; }

function workspacePage() {
  return renderWorkspacePage({ state, activeWorkspace, workspaceSession, shellByID, inspectorShell });
}

function inspectorShell() {
  if (state.inspector.collapsed) {
    return `<button class="inspector-expand" data-toggle-inspector title="展开会话工具">${icon('collapse', 16)}</button><nav class="inspector-rail" aria-label="会话工具"><button data-inspector-tab="files" class="${state.inspector.tab === 'files' ? 'active' : ''}" title="文件">${icon('folder', 16)}</button><button data-inspector-tab="forwards" class="${state.inspector.tab === 'forwards' ? 'active' : ''}" title="转发">${icon('forward', 16)}</button><button data-inspector-tab="notifications" class="${state.inspector.tab === 'notifications' ? 'active' : ''}" title="通知">${icon('bell', 16)}</button></nav>`;
  }
  const pane = activePane();
  const item = pane && shellByID(pane.shellID);
  if (!item) return `<header class="inspector-empty-head"><div><span>会话工具</span><b>文件与转发</b></div><div class="inspector-head-actions"><button data-toggle-inspector title="收起文件管理">${icon('expand', 15)}</button></div></header><div class="inspector-empty">选择终端窗格后可管理文件、转发和通知。</div>`;
  const tabs = [['files', '文件', 'folder'], ['forwards', '转发', 'forward'], ['notifications', '通知', 'bell']];
  return `<header><div><span>会话工具</span><b>${esc(item.session.name)}</b></div><div class="inspector-head-actions"><code>${esc(item.name)}</code><button data-toggle-inspector title="收起文件管理">${icon('expand', 15)}</button></div></header><nav>${tabs.map(([key, label, glyph]) => `<button data-inspector-tab="${key}" class="${state.inspector.tab === key ? 'active' : ''}">${icon(glyph, 13)}${label}</button>`).join('')}</nav><div class="inspector-body">${state.inspector.loading ? '<div class="inspector-empty">加载中…</div>' : inspectorBody(item.session)}</div>`;
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

import { activateConnectionTab, connectionEditor, setConnectionAuthMode, setJumpEnabled, setTrustUnknownHost, validateConnectionForm } from './features/connections/editor.js';

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
    body = connectionEditor(dialog.profile || emptyConnectionProfile(dialog.name), dialog.loading);
    footer = `<div class="modal-result" role="status">${esc(dialog.result || (dialog.loading ? '正在读取连接配置…' : '凭据仅保存在本机 ~/.termcp 目录中。'))}</div><button type="button" class="button" data-action="test-editor-connection" ${dialog.loading ? 'disabled' : ''}>测试连接</button><span class="modal-spacer"></span><button type="button" class="button" data-close>取消</button><button class="button primary" type="submit" ${dialog.loading ? 'disabled' : ''}>保存</button>`;
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
  return `<div class="modal-backdrop"><form class="modal ${dialog.type === 'connection' ? 'wide connection-modal' : ''}" data-form="${esc(dialog.type)}" autocomplete="off" ${dialog.type === 'connection' ? 'novalidate' : ''}><header><div><span>${eyebrow}</span><h2>${esc(title)}</h2></div><button type="button" class="icon-btn" data-close>×</button></header>${body}<footer>${footer}</footer></form></div>`;
}

function sessionContextMenu() { return renderSessionContextMenu({ state, sessionByID }); }
function terminalContextMenu() { return renderTerminalContextMenu({ state, shellByID, activeWorkspace }); }

function coreIndicator() {
  const status = state.data.core.running ? 'running' : 'error';
  const statusText = state.data.core.running ? 'Core 运行中' : 'Core 已停止';
  return `<div class="core-indicator" id="core-indicator"><button data-section="settings" aria-label="${statusText}">${dot(status)}<span>Core</span></button><div class="core-tooltip" role="tooltip"><header>${dot(status)}<div><b id="core-indicator-title">${statusText}</b><small>${coreMode()}</small></div></header><dl><div><dt>本机服务地址</dt><dd>${esc(state.data.core.address || '127.0.0.1:18765')}</dd></div><div><dt>终端事件通道</dt><dd id="core-channel-status">${esc(state.wsStatus)}</dd></div><div><dt>系统服务</dt><dd>${state.service.installed ? (state.service.running ? '已注册 · 运行中' : '已注册 · 已停止') : '未注册'}</dd></div><div><dt>开机自启</dt><dd>${state.service.autostart ? '已开启' : '已关闭'}</dd></div></dl><small>点击打开设置中的 Core 管理</small></div></div>`;
}

function render() {
  terminals.clear();
  const fullWidth = state.section === 'settings';
  document.querySelector('#app').innerHTML = `<a class="skip-link" href="#main-content">跳到主要内容</a><div class="app-shell"><header class="titlebar" style="--wails-draggable:drag"><div class="title-brand"><span>t_</span><b>Termcp</b></div><div class="window-controls" style="--wails-draggable:no-drag"><button data-window="min" aria-label="最小化">—</button><button data-window="max" aria-label="最大化窗口">□</button><button data-window="close" aria-label="隐藏到系统托盘">×</button></div></header><div class="body ${fullWidth ? 'single-content' : ''}">${rail()}${fullWidth ? '' : explorer()}<main id="main-content" class="content ${state.section === 'workspace' ? 'workspace-content' : ''} ${fullWidth ? 'settings-content' : ''}">${state.error ? `<div class="error-banner">${esc(state.error)}<button data-action="refresh">重试</button></div>` : ''}${content()}</main></div></div>${coreIndicator()}${sessionContextMenu()}${terminalContextMenu()}${modal()}`;
  localizeDOM(document.querySelector('#app'));
  if (state.section === 'workspace') mountWorkspace();
  if (state.section === 'settings') {
    const preview = document.querySelector('[data-font-preview]');
    if (preview) preview.style.fontFamily = state.appearance.fontFamily === 'system-ui' ? 'system-ui' : `"${state.appearance.fontFamily.replace(/["\\]/g, '')}", sans-serif`;
    queueMicrotask(loadSystemFonts);
  }
}

function updateConnectionBadge() {
  const channel = document.querySelector('#core-channel-status');
  if (channel) channel.textContent = state.wsStatus;
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
  if (state.section !== 'workspace' || state.inspector.collapsed) return;
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
  const next = document.querySelector('.inspector-body'); if (next) { next.innerHTML = inspectorBody(shell.session); localizeDOM(next); }
}

function openShell(shellID, sessionID) {
  const session = sessionByID(sessionID);
  state.closedSessionTabs.delete(sessionID);
  let workspace = state.workspaces.find(item => item.primarySessionID === sessionID || item.mergedSessionIDs?.includes(sessionID));
  if (!workspace) {
    workspace = {
      id: `session-tab-${sessionID}-${Date.now()}`,
      name: session?.name || 'SSH 会话',
      primarySessionID: sessionID,
      shellTabs: (session?.shells || []).map(shell => ({ shellID: shell.id, sessionID })),
      panes: [],
      maximized: '',
      layout: 'grid',
    };
    state.workspaces.push(workspace);
  }
  state.activeWorkspace = workspace.id;
  selectShellTab(workspace, shellID, sessionID);
  state.section = 'workspace'; state.selected = { type: 'shell', id: shellID };
  state.inspector.path = '/'; state.inspector.data = null;
  saveWorkspaces(); render();
}

function openSession(sessionID) {
  const session = sessionByID(sessionID);
  state.closedSessionTabs.delete(sessionID);
  const shell = session?.shells?.find(item => item.status === 'running') || session?.shells?.[0];
  if (shell) openShell(shell.id, sessionID);
  else { state.dialog = { type: 'shell', session: sessionID }; render(); }
}

async function createSessionFromConnection(connectionName) {
  const connection = connectionByName(connectionName);
  if (!connection) return;
  const sameConnection = state.data.sessions.filter(session => session.ssh_endpoint === connectionName || (connection.kind === 'internal' && session.ssh_endpoint === 'internal'));
  const name = sameConnection.length ? `${connection.name} ${sameConnection.length + 1}` : connection.name;
  await run('会话已创建', async () => {
    const result = await core.api('POST', '/api/sessions', {
      ssh_config: connection.name,
      name,
      command: '',
      mode: 'pty',
      rows: 24,
      cols: 100,
    });
    await refresh({ quiet: true });
    if (result.data?.shell_id && result.data?.session_id) openShell(result.data.shell_id, result.data.session_id);
  }, { refresh: false });
}

function addSessionSplit(sessionID, region = 'right', sourceWorkspaceID = '', preferredShellID = '') {
  const session = sessionByID(sessionID);
  const workspace = activeWorkspace();
  if (!session || !workspace) return;
  state.closedSessionTabs.delete(sessionID);
  if (preferredShellID && workspace.panes.some(pane => pane.shellID === preferredShellID)) {
    toast('Shell 已在分屏中', 'warning');
    state.draggedWorkspace = ''; state.draggedSession = ''; state.draggedShell = ''; state.dropRegion = '';
    return;
  }
  const shell = (session.shells || []).find(item => item.id === preferredShellID && !workspace.panes.some(pane => pane.shellID === item.id))
    || (session.shells || []).find(item => item.status === 'running' && !workspace.panes.some(pane => pane.shellID === item.id))
    || (session.shells || []).find(item => !workspace.panes.some(pane => pane.shellID === item.id));
  if (!shell) { toast('该会话没有可添加的 Shell', 'warning'); return; }
  const pane = { shellID: shell.id, sessionID: session.id };
  workspace.shellTabs ||= [];
  if (!workspace.shellTabs.some(tab => tab.shellID === shell.id)) workspace.shellTabs.push({ ...pane });
  if (region === 'left' || region === 'top') workspace.panes.unshift(pane);
  else workspace.panes.push(pane);
  workspace.layout = region === 'top' || region === 'bottom' ? 'rows' : 'columns';
  if (workspace.panes.length > 2) workspace.layout = 'grid';
  workspace.activeShell = shell.id;
  workspace.maximized = '';
  if (session.id !== workspace.primarySessionID) workspace.mergedSessionIDs = [...new Set([...(workspace.mergedSessionIDs || []), session.id])];
  if (sourceWorkspaceID && sourceWorkspaceID !== workspace.id) state.workspaces = state.workspaces.filter(item => item.id !== sourceWorkspaceID);
  state.draggedWorkspace = ''; state.draggedSession = ''; state.draggedShell = ''; state.dropRegion = '';
  saveWorkspaces(); render();
}

async function loadSystemFonts() {
  if (state.systemFonts.loaded || state.systemFonts.loading) return;
  state.systemFonts.loading = true; render();
  try {
    const fonts = await core.systemFonts();
    state.systemFonts.items = Array.isArray(fonts) ? fonts : [];
    state.systemFonts.loaded = true; state.systemFonts.error = '';
  } catch (error) {
    state.systemFonts.error = String(error); state.systemFonts.loaded = true;
  } finally {
    state.systemFonts.loading = false; render();
  }
}

function navigateToSection(section) {
  const allowed = new Set(['resources', 'workspace', 'sessions', 'history', 'service', 'core', 'settings']);
  if (!allowed.has(section)) return;
  if (section === 'core') section = 'settings';
  state.section = section;
  if (section === 'history') state.selected = { type: 'history-index', id: '' };
  if (section === 'settings') state.selected = { type: 'settings', id: '' };
  if (section === 'service') state.selected = { type: 'service', id: 'local' };
  render();
}

function toast(message, tone = 'info') {
  const element = document.querySelector('#toast');
  if (!element) return;
  element.textContent = t(String(message));
  element.dataset.tone = tone;
  element.hidden = false;
  clearTimeout(toast.timer); toast.timer = setTimeout(() => { element.hidden = true; }, tone === 'error' ? 6200 : 3600);
}

async function run(label, operation, options = {}) {
  try {
    const result = await operation();
    if (label) toast(label, 'success');
    if (options.refresh !== false) await refresh({ quiet: true });
    else if (options.render) render();
    return result;
  } catch (error) { toast(String(error), 'error'); return null; }
}

async function openConnectionEditor(name = '') {
  state.dialog = { type: 'connection', name, original: name, profile: emptyConnectionProfile(name), loading: Boolean(name), result: '' }; render();
  if (!name) return;
  try {
    const result = await core.api('GET', `/api/connections/${encode(name)}`);
    if (!state.dialog || state.dialog.type !== 'connection') return;
    const profile = connectionProfileFromTOML(name, result.data || result.body || '');
    const summary = connectionByName(name);
    if (summary) {
      profile.host ||= summary.host || '';
      profile.user ||= summary.user || '';
      profile.port ||= summary.port || 22;
      profile.description ||= summary.description || '';
    }
    state.dialog.profile = profile;
  } catch (error) { if (state.dialog?.type === 'connection') state.dialog.result = String(error); }
  if (!state.dialog || state.dialog.type !== 'connection') return;
  state.dialog.loading = false;
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
  const terminalAction = event.target.closest('[data-terminal-action]');
  if (terminalAction) {
    const menu = state.terminalMenu;
    const shellID = terminalAction.closest('[data-terminal-menu]')?.dataset.terminalMenu;
    const item = shellByID(shellID);
    const action = terminalAction.dataset.terminalAction;
    state.terminalMenu = null;
    document.querySelector('.terminal-context-menu')?.remove();
    if (!item) return;
    try {
      if (action === 'copy') {
        const copied = await terminals.copy(shellID, menu?.selection || '');
        toast(copied ? '已复制' : '没有选中的文本', copied ? 'success' : 'warning');
      }
      if (action === 'paste') {
        const pasted = await terminals.paste(shellID);
        if (!pasted) toast('剪贴板不可用', 'warning');
      }
      if (action === 'select-all') terminals.selectAll(shellID);
      if (action === 'clear') terminals.clearShell(shellID);
    } catch {
      toast(action === 'copy' ? '复制失败' : '剪贴板不可用', 'error');
    }
    if (action === 'new-shell') { state.dialog = { type: 'shell', session: item.session.id }; render(); }
    if (action === 'remove-pane') {
      const workspace = activeWorkspace();
      workspace.panes = workspace.panes.filter(pane => pane.shellID !== shellID);
      workspace.maximized = '';
      workspace.activeShell = workspace.panes[0]?.shellID || '';
      saveWorkspaces(); render();
    }
    if (action === 'close') confirmDialog('关闭 Shell', '当前 Shell 进程将结束；其他 Shell 和 SSH 连接保持运行。', 'close-shell', shellID, '关闭 Shell');
    return;
  }
  const contextAction = event.target.closest('[data-context-action]');
  if (contextAction) {
    const sessionID = contextAction.closest('[data-session-menu]')?.dataset.sessionMenu;
    const session = sessionByID(sessionID);
    const action = contextAction.dataset.contextAction;
    state.contextMenu = null;
    if (!session) { render(); return; }
    if (action === 'open') {
      const shellID = contextAction.dataset.shell;
      if (shellID) openShell(shellID, session.id);
      else { state.dialog = { type: 'shell', session: session.id }; render(); }
      return;
    }
    if (action === 'new-shell') { state.dialog = { type: 'shell', session: session.id }; render(); return; }
    if (action === 'details') { state.section = 'sessions'; state.selected = { type: 'session', id: session.id }; render(); return; }
    if (action === 'rename') { state.dialog = { type: 'rename', title: '重命名会话', target: 'session', id: session.id, value: session.name }; render(); return; }
    if (action === 'copy-uri') { try { await navigator.clipboard.writeText(`termcp://#${session.id}`); toast('已复制'); } catch { toast('复制失败'); } render(); return; }
    if (action === 'terminate') { confirmDialog('结束会话', '所有 Shell 和端口转发都会关闭，输出会保留到历史记录。', 'terminate-session', session.id, '结束并归档'); return; }
    if (action === 'purge') { confirmDialog('永久删除会话', '会话、消息和历史记录将永久删除。', 'purge-session', session.id, '永久删除'); return; }
  }
  if (state.contextMenu && !event.target.closest('.session-context-menu')) { state.contextMenu = null; document.querySelector('.session-context-menu')?.remove(); }
  if (state.terminalMenu && !event.target.closest('.terminal-context-menu')) { state.terminalMenu = null; document.querySelector('.terminal-context-menu')?.remove(); }
  const groupToggle = event.target.closest('[data-group-toggle]');
  if (groupToggle) {
    const key = groupToggle.dataset.groupToggle;
    state.collapsedGroups.has(key) ? state.collapsedGroups.delete(key) : state.collapsedGroups.add(key);
    saveCollapsedGroups(); render(); return;
  }
  const toggle = event.target.closest('[data-toggle]');
  if (toggle) { const key = toggle.dataset.toggle; state.expanded.has(key) ? state.expanded.delete(key) : state.expanded.add(key); render(); return; }
  const inspectorToggle = event.target.closest('[data-toggle-inspector]');
  if (inspectorToggle) { state.inspector.collapsed = !state.inspector.collapsed; localStorage.setItem('termcp-desktop-inspector-collapsed', state.inspector.collapsed ? '1' : '0'); render(); return; }
  const closeWorkspace = event.target.closest('[data-close-workspace]');
  if (closeWorkspace) {
    event.stopPropagation();
    const closing = state.workspaces.find(item => item.id === closeWorkspace.dataset.closeWorkspace);
    [closing?.primarySessionID, ...(closing?.mergedSessionIDs || [])].filter(Boolean).forEach(id => state.closedSessionTabs.add(id));
    state.workspaces = state.workspaces.filter(item => item.id !== closeWorkspace.dataset.closeWorkspace);
    state.activeWorkspace = state.workspaces[0]?.id || '';
    saveWorkspaces(); render(); return;
  }
  const paneTarget = event.target.closest('[data-pane-activate]');
  if (paneTarget) {
    const workspace = activeWorkspace();
    if (workspace.activeShell !== paneTarget.dataset.paneActivate) {
      workspace.activeShell = paneTarget.dataset.paneActivate;
      state.inspector.path = '/';
      saveWorkspaces();
      document.querySelectorAll('.terminal-pane.active').forEach(item => item.classList.remove('active'));
      paneTarget.classList.add('active');
      await loadInspector();
    }
  }
  const target = event.target.closest('button');
  if (!target) return;
  if (target.dataset.closeShellTab) { confirmDialog('关闭 Shell', '当前 Shell 进程将结束；其他 Shell 和 SSH 连接保持运行。', 'close-shell', target.dataset.closeShellTab, '关闭 Shell'); return; }
  if (target.dataset.shellTab) {
    const workspace = activeWorkspace();
    selectShellTab(workspace, target.dataset.shellTab, target.dataset.session);
    state.selected = { type: 'shell', id: target.dataset.shellTab };
    state.inspector.path = '/'; state.inspector.data = null;
    saveWorkspaces(); render(); return;
  }
  if (target.dataset.connectionTab) {
    const form = target.closest('form'); const tab = target.dataset.connectionTab;
    activateConnectionTab(form, tab);
    return;
  }
  if (target.dataset.authMode) { setConnectionAuthMode(target.closest('form'), target.dataset.authScope, target.dataset.authMode); return; }
  if (target.dataset.toggleSecret !== undefined) {
    const input = target.closest('.secret-input')?.querySelector('input');
    if (input) { const visible = input.type === 'text'; input.type = visible ? 'password' : 'text'; target.classList.toggle('active', !visible); target.setAttribute('aria-label', visible ? '显示密码' : '隐藏密码'); target.title = visible ? '显示密码' : '隐藏密码'; }
    return;
  }
  if (target.dataset.section) { navigateToSection(target.dataset.section); return; }
  if (target.dataset.connectionProfile) {
    clearTimeout(connectionClickTimer);
    const name = target.dataset.connectionProfile;
    connectionClickTimer = setTimeout(() => {
      state.section = 'resources';
      state.selected = { type: 'connection', id: name };
      render();
    }, 240);
    return;
  }
  if (target.dataset.select) { const [type, ...parts] = target.dataset.select.split(':'); state.selected = { type, id: parts.join(':') }; if (type === 'history') state.historyTranscript = ''; if (type === 'core') state.section = 'resources'; render(); return; }
  if (target.dataset.openSession) { openSession(target.dataset.openSession); return; }
  if (target.dataset.openShell) { openShell(target.dataset.openShell, target.dataset.session); return; }
  if (target.dataset.workspace) { state.activeWorkspace = target.dataset.workspace; state.section = 'workspace'; saveWorkspaces(); render(); return; }
  if (target.dataset.copy !== undefined) { try { await navigator.clipboard.writeText(target.dataset.copy); toast('已复制'); } catch { toast('复制失败'); } return; }
  if (target.dataset.window) { core.window(target.dataset.window); return; }
  if (target.dataset.close !== undefined) { state.dialog = null; render(); return; }
  if (target.dataset.activePane) { const workspace = activeWorkspace(); workspace.activeShell = target.dataset.activePane; state.inspector.path = '/'; saveWorkspaces(); render(); return; }
  if (target.dataset.maxPane) { const workspace = activeWorkspace(); workspace.maximized = workspace.maximized ? '' : target.dataset.maxPane; saveWorkspaces(); render(); return; }
  if (target.dataset.removePane) { const workspace = activeWorkspace(); workspace.panes = workspace.panes.filter(pane => pane.shellID !== target.dataset.removePane); workspace.maximized = ''; workspace.activeShell = workspace.panes[0]?.shellID || ''; saveWorkspaces(); render(); return; }
  if (target.dataset.inspectorTab) { state.inspector.tab = target.dataset.inspectorTab; state.inspector.data = null; if (state.inspector.collapsed) { state.inspector.collapsed = false; localStorage.setItem('termcp-desktop-inspector-collapsed', '0'); } render(); return; }
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
  if (action === 'add-pane') { state.dialog = { type: 'add-pane', direction: 'right' }; render(); return; }
  if (action === 'new-directory') { state.dialog = { type: 'directory' }; render(); return; }
  if (action === 'new-forward') { state.dialog = { type: 'forward', session: state.inspector.sessionID }; render(); return; }
  if (action === 'edit-history') { state.dialog = { type: 'history', item: historyByID(target.dataset.id) }; render(); return; }
  if (action === 'load-transcript') { await loadTranscript(target.dataset.id); return; }
  if (action === 'export-transcript') { await run('已保存历史正文', () => core.save(`/api/history/${encode(target.dataset.id)}/transcript?format=markdown`, `${historyByID(target.dataset.id)?.name || 'history'}.md`), { refresh: false }); return; }
  if (action === 'export-screenshot') { await run('已保存历史截图', () => core.save(`/api/history/${encode(target.dataset.id)}/screenshot?start=0&lines=80&cols=120&theme=dark`, `${historyByID(target.dataset.id)?.name || 'history'}.png`), { refresh: false }); return; }
  if (action === 'delete-history') { confirmDialog('删除历史记录', '历史正文和关联消息将永久删除。', 'delete-history', target.dataset.id, '删除'); return; }
  if (action === 'history-search') { await searchHistory(); return; }
  if (action === 'test-editor-connection') {
    const form = target.closest('form'); if (!validateConnectionForm(form)) return;
    const resultNode = form.querySelector('.modal-result'); target.disabled = true; if (resultNode) resultNode.textContent = t('正在测试连接…');
    try { const result = await testConnection(connectionFormToTOML(form), false); if (resultNode) resultNode.textContent = t(result.data?.ok ? `连接成功${result.data.latency_ms ? ` · ${result.data.latency_ms} ms` : ''}` : '测试完成'); } catch (error) { if (resultNode) resultNode.textContent = String(error); }
    target.disabled = false; return;
  }
});

document.addEventListener('dblclick', async event => {
  const connection = event.target.closest('[data-connection-profile]');
  if (!connection) return;
  event.preventDefault();
  clearTimeout(connectionClickTimer);
  await createSessionFromConnection(connection.dataset.connectionProfile);
});

document.addEventListener('input', event => {
  if (event.target.matches('[data-filter]')) { state.filter = event.target.value; const treeElement = document.querySelector('.tree-scroll'); if (treeElement) { treeElement.innerHTML = tree(state.filter.trim().toLowerCase()); localizeDOM(treeElement); } }
  if (event.target.matches('[data-history-query]')) state.historyQuery = event.target.value;
});

document.addEventListener('change', async event => {
  if (event.target.matches('[data-jump-enabled]')) { setJumpEnabled(event.target.closest('form'), event.target.checked); return; }
  if (event.target.matches('[data-trust-toggle]')) { setTrustUnknownHost(event.target.closest('form'), event.target.dataset.trustToggle, event.target.checked); return; }
  if (event.target.matches('[data-appearance]')) {
    state.appearance = setAppearance({ [event.target.dataset.appearance]: event.target.value });
    render();
    return;
  }
  if (event.target.matches('[data-language]')) {
    const language = setLanguage(event.target.value);
    try { await core.setLanguage(language); } catch { /* The DOM locale remains usable if a legacy backend lacks this bridge. */ }
    render();
    return;
  }
  if (event.target.matches('[data-service-autostart]')) {
    const enabled = event.target.checked;
    await run(enabled ? '已开启开机自启' : '已关闭开机自启', () => core.setAutostart(enabled));
  }
});

document.addEventListener('keydown', async event => {
  if (event.key === 'Enter' && event.target.matches('[data-file-path]')) { event.preventDefault(); state.inspector.path = event.target.value.trim() || '/'; await loadInspector(); }
  if (event.key === 'Enter' && event.target.matches('[data-history-query]')) { event.preventDefault(); await searchHistory(); }
  if (event.key === 'Escape' && (state.dialog || state.contextMenu || state.terminalMenu)) { state.dialog = null; state.contextMenu = null; state.terminalMenu = null; render(); }
  if (state.section !== 'workspace' || event.target.matches('input,textarea,select')) return;
  if (event.altKey && event.shiftKey && (event.key === 'ArrowRight' || event.key === 'ArrowDown')) {
    event.preventDefault();
    state.dialog = { type: 'add-pane', direction: event.key === 'ArrowDown' ? 'bottom' : 'right' };
    render();
    return;
  }
  if ((event.metaKey || event.ctrlKey) && event.shiftKey && event.key.toLowerCase() === 't') {
    event.preventDefault();
    const session = workspaceSession(activeWorkspace());
    if (session) { state.dialog = { type: 'shell', session: session.id }; render(); }
  }
});

document.addEventListener('contextmenu', event => {
  const shellTarget = event.target.closest('[data-shell-context]');
  if (shellTarget) {
    event.preventDefault();
    const shellID = shellTarget.dataset.shellContext;
    const width = 252; const height = 310; const margin = 8;
    state.contextMenu = null;
    state.terminalMenu = {
      shellID,
      selection: terminals.selection(shellID),
      x: Math.max(margin, Math.min(event.clientX, window.innerWidth - width - margin)),
      y: Math.max(margin, Math.min(event.clientY, window.innerHeight - height - margin)),
    };
    render();
    return;
  }
  const node = event.target.closest('[data-session-context]');
  if (!node) return;
  event.preventDefault();
  const width = 244; const height = 326; const margin = 8;
  state.terminalMenu = null;
  state.contextMenu = {
    sessionID: node.dataset.sessionContext,
    x: Math.max(margin, Math.min(event.clientX, window.innerWidth - width - margin)),
    y: Math.max(margin, Math.min(event.clientY, window.innerHeight - height - margin)),
  };
  render();
});

document.addEventListener('dragstart', event => {
  const tab = event.target.closest('[data-workspace-drag]');
  const shellTab = event.target.closest('[data-shell-drag]');
  const sessionNode = event.target.closest('[data-session-drag]');
  if (!tab && !shellTab && !sessionNode) return;
  state.draggedWorkspace = tab?.dataset.workspaceDrag || '';
  state.draggedShell = shellTab?.dataset.shellDrag || '';
  state.draggedSession = shellTab?.dataset.sessionDrag || tab?.dataset.sessionDrag || sessionNode?.dataset.sessionDrag || '';
  (tab || shellTab || sessionNode).classList.add('dragging');
  event.dataTransfer.effectAllowed = 'move';
  event.dataTransfer.setData('application/x-termcp-session', state.draggedSession);
  if (state.draggedShell) event.dataTransfer.setData('application/x-termcp-shell', state.draggedShell);
  event.dataTransfer.setData('text/plain', state.draggedWorkspace || state.draggedShell || state.draggedSession);
});

document.addEventListener('dragover', event => {
  const dropZone = event.target.closest('[data-session-drop-zone]');
  if (dropZone && state.draggedSession) {
    event.preventDefault();
    event.dataTransfer.dropEffect = 'move';
    const rect = dropZone.getBoundingClientRect();
    const x = (event.clientX - rect.left) / rect.width;
    const y = (event.clientY - rect.top) / rect.height;
    const region = x < .28 ? 'left' : x > .72 ? 'right' : y < .38 ? 'top' : y > .62 ? 'bottom' : (rect.width >= rect.height ? 'right' : 'bottom');
    state.dropRegion = region;
    dropZone.classList.remove('drop-left', 'drop-right', 'drop-top', 'drop-bottom');
    dropZone.classList.add(`drop-${region}`);
    const label = dropZone.querySelector('.session-drop-hint span');
    if (label) label.textContent = region === 'top' ? t('在上方分屏') : region === 'bottom' ? t('在下方分屏') : region === 'left' ? t('在左侧分屏') : t('在右侧分屏');
    return;
  }
  const tab = event.target.closest('[data-workspace-drag]');
  if (!tab || !state.draggedWorkspace || tab.dataset.workspaceDrag === state.draggedWorkspace) return;
  event.preventDefault();
  event.dataTransfer.dropEffect = 'move';
  document.querySelectorAll('.workspace-tab.drop-target').forEach(item => item.classList.remove('drop-target'));
  tab.classList.add('drop-target');
});

document.addEventListener('drop', event => {
  const dropZone = event.target.closest('[data-session-drop-zone]');
  if (dropZone && state.draggedSession) {
    event.preventDefault();
    addSessionSplit(state.draggedSession, state.dropRegion || 'right', state.draggedWorkspace, state.draggedShell);
    return;
  }
  const tab = event.target.closest('[data-workspace-drag]');
  const sourceID = state.draggedWorkspace || event.dataTransfer.getData('text/plain');
  const targetID = tab?.dataset.workspaceDrag;
  if (!sourceID || !targetID || sourceID === targetID) return;
  event.preventDefault();
  const sourceIndex = state.workspaces.findIndex(item => item.id === sourceID);
  const targetIndex = state.workspaces.findIndex(item => item.id === targetID);
  if (sourceIndex < 0 || targetIndex < 0) return;
  const [moved] = state.workspaces.splice(sourceIndex, 1);
  state.workspaces.splice(targetIndex, 0, moved);
  state.draggedWorkspace = '';
  saveWorkspaces();
  render();
});

document.addEventListener('dragend', () => {
  state.draggedWorkspace = ''; state.draggedSession = ''; state.draggedShell = ''; state.dropRegion = '';
  document.querySelectorAll('.dragging,.workspace-tab.drop-target,.drop-left,.drop-right,.drop-top,.drop-bottom').forEach(item => item.classList.remove('dragging', 'drop-target', 'drop-left', 'drop-right', 'drop-top', 'drop-bottom'));
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
    if (!validateConnectionForm(form)) return;
    const name = String(data.get('name')).trim(); const original = state.dialog.original;
    const config = connectionFormToTOML(form);
    state.dialog = null;
    await run('连接配置已保存', () => core.api('PUT', `/api/connections/${encode(name)}${original && original !== name ? `?from=${encode(original)}` : ''}`, config, 'text/plain; charset=utf-8')); return;
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
  if (type === 'add-pane') { const direction = state.dialog?.direction || 'right'; state.dialog = null; const shellID = data.get('shell'); const item = shellByID(shellID); if (item) addSessionSplit(item.session.id, direction, '', shellID); }
});

async function executeConfirmed(action, payload) {
  if (action === 'uninstall-service') await run('系统服务已卸载，Core 已切回应用内运行', () => core.uninstallService());
  if (action === 'delete-connection') await run('连接配置已删除', () => core.api('DELETE', `/api/connections/${encode(payload)}`));
  if (action === 'terminate-session') await run('会话已结束并归档', () => core.api('POST', `/api/sessions/${encode(payload)}/terminate`));
  if (action === 'purge-session') await run('会话已永久删除', () => core.api('DELETE', `/api/sessions/${encode(payload)}`));
  if (action === 'delete-history') { await run('历史记录已删除', () => core.api('DELETE', `/api/history/${encode(payload)}`)); state.selected = { type: 'history-index', id: '' }; }
  if (action === 'close-shell') {
    await run('Shell 已关闭', () => core.api('DELETE', `/api/shells/${encode(payload)}`));
    for (const workspace of state.workspaces) removeShellFromWorkspace(workspace, payload);
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

window.runtime?.EventsOn?.('termcp:navigate', section => navigateToSection(section));
window.runtime?.EventsOn?.('termcp:tray-result', (message, error) => {
  toast(error || message || '操作完成');
  refresh({ quiet: true });
});

core.setLanguage(getLanguage()).catch(() => {});
render();
refresh();
terminals.connect();
