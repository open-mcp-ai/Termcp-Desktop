import './styles.css';
import './theme.css';
import './features/workspace/workspace.css';
import './features/resources/resources.css';
import './features/approval/approval.css';
import './ui/feedback.css';
import { core } from './core.js';
import { setAppearance } from './appearance.js';
import { getLanguage, localizeDOM, setLanguage, t } from './i18n/index.js';
import { TerminalController } from './terminal.js';

import { dot, encode, esc, joinPath, parentPath } from './ui/render.js';
import { emptyPage } from './ui/components.js';

import { connectionFormToTOML, connectionProfileFromTOML, emptyConnectionProfile } from './features/connections/profile.js';
import { activateConnectionTab, setConnectionAuthMode, setJumpEnabled, setTrustUnknownHost, validateConnectionForm } from './features/connections/editor.js';
import { rail, explorer, tree } from './features/resources/view.js';
import { connectionPage, historyIndexPage, historyPage, sessionPage, shellPage } from './features/resources/pages.js';
import { moveTags, setTags } from './features/resources/tags.js';
import { commandLinePayload } from './features/resources/command.js';
import { historyPreviewSpan } from './features/resources/history.js';
import { SessionCreationCoordinator } from './features/resources/session-create.js';
import { approvalPage, pendingApprovals } from './features/approval/view.js';
import { approvalSettings, effectiveApproval, moveConnectionPolicy, policyFor, rememberPolicies, removeConnectionPolicy, setConnectionPolicy, setDesktopNotifications, setGlobalApproval, withDefaultApproval } from './features/approval/policy.js';
import { servicePage } from './features/service/view.js';
import { settingsPage } from './features/settings/view.js';
import { inspectorBody, inspectorShell } from './features/inspector/view.js';
import { activateInspectorShell, followShellDirectory, performFileUpload, rememberInspectorPath } from './features/inspector/controller.js';
import { modal } from './features/modal.js';
import { removeShellFromWorkspace, selectShellTab } from './features/workspace/model.js';
import { renderSessionContextMenu, renderTerminalContextMenu, renderWorkspacePage, shellActivityLabel } from './features/workspace/view.js';
import { activePane, activeWorkspace, connectionByName, coreMode, historyByID, reconcileWorkspaces, saveCollapsedGroups, saveWorkspaces, sessionByID, shellByID, state, workspaceSession } from './state.js';

if (!core.preview && /Macintosh|Mac OS X/.test(navigator.userAgent)) {
  document.documentElement.dataset.nativeMacTitlebar = 'true';
}

let refreshTimer;
let connectionClickTimer;
let inspectorRequest = 0;
let historyRequest = 0;
let approvalSync = null;
const inspectorDirectoryRequests = new Map();
const sessionCreation = new SessionCreationCoordinator({
  create: payload => core.api('POST', '/api/sessions', payload),
  snapshot: () => core.snapshot(),
  connected: () => state.wsStatus === 'connected',
});
const terminals = new TerminalController(core, {
  status(status) { state.wsStatus = status; updateConnectionBadge(); },
  sessions(message) { sessionCreation.observe(message); clearTimeout(refreshTimer); refreshTimer = setTimeout(() => refresh({ quiet: true }), 180); },
  notify(message) { toast(message.message || message.text || 'Core notification'); },
  approval(message) { handleApprovalEvent(message); },
  log(level, message, details) { core.log(level, message, details); },
  cwd(shellID, directory) { handleShellDirectory(shellID, directory); },
  activity(shellID, status) { updateShellActivity(shellID, status); },
});

window.addEventListener('error', event => {
  core.log('error', 'Unhandled frontend error', `${event.message || 'Unknown error'}${event.filename ? ` · ${event.filename}:${event.lineno || 0}` : ''}`);
});
window.addEventListener('unhandledrejection', event => {
  core.log('error', 'Unhandled frontend promise rejection', String(event.reason || 'Unknown rejection'));
});

async function refresh({ quiet = false } = {}) {
  if (!quiet) { state.loading = true; state.error = ''; render(); }
  try {
    const [snapshot, service] = await Promise.all([core.snapshot(), core.serviceStatus()]);
    state.data = { ...snapshot, preview: core.preview, connections: snapshot.connections || [], sessions: snapshot.sessions || [], history: snapshot.history || [], forwards: snapshot.forwards || [] };
    rememberPolicies(state.data.connections);
    await syncApprovalProfiles(state.data.connections);
    await loadApprovals();
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

async function syncApprovalProfiles(connections) {
  if (approvalSync) await approvalSync;
  const pending = connections.filter(connection => Boolean(connection.default_approval) !== effectiveApproval(connection.name, connection));
  if (!pending.length) return;
  approvalSync = Promise.allSettled(pending.map(async connection => {
    const enabled = effectiveApproval(connection.name, connection);
    const result = await core.api('GET', `/api/connections/${encode(connection.name)}`);
    await core.api('PUT', `/api/connections/${encode(connection.name)}`, withDefaultApproval(result.data || result.body, enabled), 'text/plain; charset=utf-8');
    connection.default_approval = enabled;
  })).then(results => {
    const failed = results.find(result => result.status === 'rejected');
    if (failed) throw failed.reason;
  });
  try { await approvalSync; } finally { approvalSync = null; }
}

async function loadApprovals() {
  if (!state.data.core.running) { state.approvals = []; return; }
  const result = await core.api('GET', '/api/approvals');
  state.approvals = result.data?.approvals || [];
}

async function handleApprovalEvent(message) {
  try { await loadApprovals(); } catch (error) { core.log('warn', 'Approval list refresh failed', String(error)); }
  const request = message.request || {};
  if (request.state === 'pending') {
    toast(message.message || 'AI action needs approval', 'warning');
    if (approvalSettings.desktopNotifications) {
      try { await core.sendApprovalNotification(request.id || '', t(message.title || 'Approval requested'), message.message || request.summary || ''); }
      catch (error) { core.log('warn', 'System notification failed', String(error)); }
    }
  } else if (message.message) toast(message.message);
  render();
}

function selectedResource() {
  const { type, id } = state.selected;
  if (type === 'connection') return connectionByName(id);
  if (type === 'session') return sessionByID(id);
  if (type === 'shell') return shellByID(id);
  if (type === 'history') return historyByID(id);
  return state.data.core;
}

function content() {
  if (state.section === 'workspace') return workspacePage();
  if (state.section === 'approvals') return approvalPage();
  if (state.section === 'service') return servicePage();
  if (state.section === 'settings') return settingsPage();
  if (state.section === 'history' && state.selected.type !== 'history') return historyIndexPage();
  const resource = selectedResource();
  const pages = { connection: connectionPage, session: sessionPage, shell: shellPage, history: historyPage };
  return (pages[state.selected.type] || emptyPage)(resource);
}

function workspacePage() {
  return renderWorkspacePage({ state, activeWorkspace, workspaceSession, shellByID, inspectorShell, activityForShell: shellID => terminals.activity.status(shellID) });
}

function updateShellActivity(shellID, status) {
  for (const badge of document.querySelectorAll(`[data-shell-activity="${CSS.escape(shellID)}"]`)) {
    badge.dataset.kind = status;
    badge.textContent = shellActivityLabel(status);
  }
}

function sessionContextMenu() { return renderSessionContextMenu({ state, sessionByID }); }
function terminalContextMenu() { return renderTerminalContextMenu({ state, shellByID, activeWorkspace }); }

function coreIndicator() {
  const status = state.data.core.running ? 'running' : 'error';
  const statusText = state.data.core.running ? 'Core running' : 'Core stopped';
  return `<div class="core-indicator" id="core-indicator"><button data-section="settings" aria-label="${statusText}">${dot(status)}<span>Core</span></button><div class="core-tooltip" role="tooltip"><header>${dot(status)}<div><b id="core-indicator-title">${statusText}</b><small>${coreMode()}</small></div></header><dl><div><dt>Local service address</dt><dd>${esc(state.data.core.address || '127.0.0.1:18765')}</dd></div><div><dt>Terminal event channel</dt><dd id="core-channel-status">${esc(state.wsStatus)}</dd></div><div><dt>System service</dt><dd>${state.service.installed ? (state.service.running ? 'Registered · Running' : 'Registered · Stopped') : 'Not registered'}</dd></div><div><dt>Autostart</dt><dd>${state.service.autostart ? 'Enabled' : 'Disabled'}</dd></div></dl><small>Open Core management in Settings</small></div></div>`;
}

function render() {
  const historyView = state.section === 'history' && state.selected.type === 'history' ? state.selected.id : '';
  const previousContent = document.querySelector('#main-content');
  const historyScrollTop = historyView && previousContent?.dataset.historyView === historyView ? previousContent.scrollTop : null;
  const previousTimeline = historyScrollTop !== null ? previousContent.querySelector('.history-timeline') : null;
  const timelineScrollTop = previousTimeline?.dataset.shellId === state.historyShellID ? previousTimeline.scrollTop : null;
  const previousEvents = historyScrollTop !== null ? previousContent.querySelector('.history-events-panel') : null;
  const eventsScrollTop = previousEvents?.dataset.shellId === state.historyShellID ? previousEvents.scrollTop : null;
  const previousOutput = historyScrollTop !== null ? previousContent.querySelector('.history-output-scroll') : null;
  const outputScrollTop = previousOutput?.dataset.shellId === state.historyShellID && Number(previousOutput.dataset.spanIndex) === state.historySpanIndex ? previousOutput.scrollTop : null;
  terminals.detachAll();
  const fullWidth = state.section === 'settings' || state.section === 'approvals';
  const pending = pendingApprovals().length;
  document.querySelector('#app').innerHTML = `<a class="skip-link" href="#main-content">Skip to main content</a><div class="app-shell"><header class="titlebar" style="--wails-draggable:drag"><div class="title-brand"><span>T_</span><b>Termcp</b></div><div class="window-controls" style="--wails-draggable:no-drag"><button data-window="min" aria-label="Minimise">—</button><button data-window="max" aria-label="Maximise window">□</button><button data-window="close" aria-label="Hide to system tray">×</button></div></header><div class="body ${fullWidth ? 'single-content' : ''} ${state.section === 'workspace' ? `workspace-body ${state.explorerOpen ? 'explorer-open' : ''}` : ''}">${rail()}${fullWidth ? '' : explorer()}<main id="main-content" data-history-view="${esc(historyView)}" class="content ${historyView ? 'history-content' : ''} ${state.section === 'workspace' ? 'workspace-content' : ''} ${fullWidth ? 'settings-content' : ''}">${pending && state.section !== 'approvals' ? `<div class="approval-banner"><b>${pending} approval${pending === 1 ? '' : 's'} pending</b><span>AI actions are waiting for a decision.</span><button data-section="approvals">Review</button></div>` : ''}${state.error ? `<div class="error-banner">${esc(state.error)}<button data-action="refresh">Retry</button></div>` : ''}${content()}</main></div></div>${coreIndicator()}${sessionContextMenu()}${terminalContextMenu()}${modal()}`;
  localizeDOM(document.querySelector('#app'));
  if (historyScrollTop !== null) {
    document.querySelector('#main-content').scrollTop = historyScrollTop;
    const timeline = document.querySelector('.history-timeline');
    if (timeline && timelineScrollTop !== null) timeline.scrollTop = timelineScrollTop;
    const events = document.querySelector('.history-events-panel');
    if (events && eventsScrollTop !== null) events.scrollTop = eventsScrollTop;
    const output = document.querySelector('.history-output-scroll');
    if (output && outputScrollTop !== null) output.scrollTop = outputScrollTop;
  }
  if (state.section === 'workspace') mountWorkspace();
  const liveShells = new Set(state.data.sessions.flatMap(session => (session.shells || []).map(shell => shell.id)));
  terminals.retain(liveShells);
  terminals.refreshAppearance();
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

function selectInspectorShell(shellID) {
  const changed = activateInspectorShell(state.inspector, shellID, terminals.workingDirectory(shellID));
  if (changed) {
    state.inspector.data = null;
    state.inspector.error = '';
    state.inspector.loadedKey = '';
  }
  return changed;
}

function setInspectorPath(directory) {
  const shellID = activePane()?.shellID || state.inspector.shellID;
  if (!rememberInspectorPath(state.inspector, shellID, directory)) return false;
  state.inspector.data = null;
  state.inspector.error = '';
  state.inspector.loadedKey = '';
  return true;
}

function handleShellDirectory(shellID, directory) {
  if (!followShellDirectory(state.inspector, shellID, directory)) return;
  if (state.section !== 'workspace' || activePane()?.shellID !== shellID) return;
  state.inspector.data = null;
  state.inspector.error = '';
  state.inspector.loadedKey = '';
  queueMicrotask(() => loadInspector());
}

async function ensureInspectorDirectory(shell) {
  selectInspectorShell(shell.id);
  if (state.inspector.path) return true;
  let request = inspectorDirectoryRequests.get(shell.id);
  if (!request) {
    request = core.defaultDirectory(shell.session.id)
      .catch(error => {
        core.log('warn', 'Default file directory unavailable', `shell=${shell.id} session=${shell.session.id}: ${String(error)}`);
        return '/';
      })
      .finally(() => inspectorDirectoryRequests.delete(shell.id));
    inspectorDirectoryRequests.set(shell.id, request);
  }
  const directory = await request;
  followShellDirectory(state.inspector, shell.id, directory || '/');
  return state.section === 'workspace' && activePane()?.shellID === shell.id;
}

async function mountWorkspace() {
  const workspace = activeWorkspace();
  for (const pane of workspace.panes) {
    const shell = shellByID(pane.shellID);
    const element = document.querySelector(`[data-terminal-shell="${CSS.escape(pane.shellID)}"]`);
    if (shell && element) terminals.mount(pane.shellID, element, shell.status !== 'running');
  }
  await loadInspector({ force: false });
}

async function loadInspector({ force = true } = {}) {
  if (state.section !== 'workspace' || state.inspector.collapsed) return;
  const pane = activePane(); const shell = pane && shellByID(pane.shellID);
  if (!shell) return;
  if (!await ensureInspectorDirectory(shell)) return;
  const key = `${shell.session.id}|${state.inspector.tab}|${state.inspector.path}`;
  if (!force && state.inspector.loadedKey === key && (state.inspector.data !== null || state.inspector.error)) return;
  const request = ++inspectorRequest;
  state.inspector.sessionID = shell.session.id;
  state.inspector.loading = true; state.inspector.error = '';
  const body = document.querySelector('.inspector-body'); if (body) body.innerHTML = '<div class="inspector-empty">Loading…</div>';
  let inspectorData = null;
  let inspectorError = '';
  try {
    if (state.inspector.tab === 'files') {
      const result = await core.api('GET', `/api/sessions/${encode(shell.session.id)}/files?path=${encode(state.inspector.path)}`);
      inspectorData = result.data;
    } else if (state.inspector.tab === 'forwards') {
      const result = await core.api('GET', `/api/sessions/${encode(shell.session.id)}/forwards`);
      inspectorData = result.data;
    } else {
      const result = await core.api('GET', `/api/notifications?session_id=${encode(shell.session.id)}`);
      inspectorData = result.data;
    }
  } catch (error) { inspectorError = String(error); }
  if (request !== inspectorRequest || state.section !== 'workspace') return;
  const currentPane = activePane(); const currentShell = currentPane && shellByID(currentPane.shellID);
  const currentKey = currentShell ? `${currentShell.session.id}|${state.inspector.tab}|${state.inspector.path}` : '';
  if (currentKey !== key) return;
  state.inspector.data = inspectorData;
  state.inspector.error = inspectorError;
  state.inspector.loadedKey = key;
  state.inspector.loading = false;
  const next = document.querySelector('.inspector-body'); if (next) { next.innerHTML = inspectorBody(shell.session); localizeDOM(next); }
}

function openShell(shellID, sessionID) {
  state.explorerOpen = false;
  const session = sessionByID(sessionID);
  state.closedSessionTabs.delete(sessionID);
  let workspace = state.workspaces.find(item => item.primarySessionID === sessionID || item.mergedSessionIDs?.includes(sessionID));
  if (!workspace) {
    workspace = {
      id: `session-tab-${sessionID}-${Date.now()}`,
      name: session?.name || 'SSH session',
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
  selectInspectorShell(shellID);
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
  await createSessionAndOpen(connection.name, {
    ssh_config: connection.name,
    name,
    command: '',
    mode: 'pty',
    rows: 24,
    cols: 100,
  });
}

async function createSessionAndOpen(connectionName, payload) {
  if (sessionCreation.pending(connectionName)) { toast('Connection already in progress'); return; }
  await run('Session created', async () => {
    const result = await sessionCreation.create(connectionName, payload);
    if (!result) return;
    await refresh({ quiet: true });
    openShell(result.shell_id, result.session_id);
  }, { refresh: false });
}

function addSessionSplit(sessionID, region = 'right', sourceWorkspaceID = '', preferredShellID = '') {
  const session = sessionByID(sessionID);
  const workspace = activeWorkspace();
  if (!session || !workspace) return;
  state.closedSessionTabs.delete(sessionID);
  if (preferredShellID && workspace.panes.some(pane => pane.shellID === preferredShellID)) {
    toast('Shell is already visible in the split', 'warning');
    state.draggedWorkspace = ''; state.draggedSession = ''; state.draggedShell = ''; state.dropRegion = '';
    return;
  }
  const shell = (session.shells || []).find(item => item.id === preferredShellID && !workspace.panes.some(pane => pane.shellID === item.id))
    || (session.shells || []).find(item => item.status === 'running' && !workspace.panes.some(pane => pane.shellID === item.id))
    || (session.shells || []).find(item => !workspace.panes.some(pane => pane.shellID === item.id));
  if (!shell) { toast('This session has no shell available to add', 'warning'); return; }
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
  const allowed = new Set(['resources', 'workspace', 'sessions', 'approvals', 'history', 'service', 'core', 'settings']);
  if (!allowed.has(section)) return;
  if (section === 'core') section = 'settings';
  state.section = section;
  state.explorerOpen = false;
  state.tagFilter = '';
  if (section === 'history') state.selected = { type: 'history-index', id: '' };
  if (section === 'settings') state.selected = { type: 'settings', id: '' };
  if (section === 'approvals') state.selected = { type: 'approvals', id: '' };
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
  } catch (error) {
    core.log('error', 'Frontend operation failed', `${options.logLabel || label || 'unnamed operation'}: ${String(error)}`);
    toast(String(error), 'error'); return null;
  }
}

async function openConnectionEditor(name = '') {
  state.dialog = { type: 'connection', name, original: name, profile: emptyConnectionProfile(name), loading: Boolean(name), result: '' }; render();
  if (!name) return;
  try {
    const result = await core.api('GET', `/api/connections/${encode(name)}`);
    if (!state.dialog || state.dialog.type !== 'connection') return;
    const profile = connectionProfileFromTOML(name, result.data || result.body || '');
    const summary = connectionByName(name);
    profile.approvalPolicy = policyFor(name, summary);
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
  if (label) toast(result.data?.ok ? `Connection successful${result.data.latency_ms ? ` · ${result.data.latency_ms} ms` : ''}` : 'Connection test complete');
  return result;
}

function confirmDialog(title, message, action, payload, confirm = 'Confirm') {
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
        toast(copied ? 'Copied' : 'No text is selected', copied ? 'success' : 'warning');
      }
      if (action === 'paste') {
        const pasted = await terminals.paste(shellID);
        if (!pasted) toast('Clipboard is unavailable', 'warning');
      }
      if (action === 'select-all') terminals.selectAll(shellID);
      if (action === 'clear') terminals.clearShell(shellID);
    } catch {
      toast(action === 'copy' ? 'Copy failed' : 'Clipboard is unavailable', 'error');
    }
    if (action === 'new-shell') { state.dialog = { type: 'shell', session: item.session.id }; render(); }
    if (action === 'remove-pane') {
      const workspace = activeWorkspace();
      workspace.panes = workspace.panes.filter(pane => pane.shellID !== shellID);
      workspace.maximized = '';
      workspace.activeShell = workspace.panes[0]?.shellID || '';
      saveWorkspaces(); render();
    }
    if (action === 'close') confirmDialog('Close shell', 'This shell process will end. Other shells and the SSH connection keep running.', 'close-shell', shellID, 'Close shell');
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
    if (action === 'rename') { state.dialog = { type: 'rename', title: 'Rename session', target: 'session', id: session.id, value: session.name }; render(); return; }
    if (action === 'copy-uri') { try { await navigator.clipboard.writeText(`termcp://#${session.id}`); toast('Copied'); } catch { toast('Copy failed'); } render(); return; }
    if (action === 'terminate') { confirmDialog('Terminate session', 'All shells and port forwards will close. Output will be retained in history.', 'terminate-session', session.id, 'Terminate and archive'); return; }
    if (action === 'purge') { confirmDialog('Delete session permanently', 'The session, messages and history will be permanently deleted.', 'purge-session', session.id, 'Delete permanently'); return; }
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
      selectInspectorShell(workspace.activeShell);
      saveWorkspaces();
      document.querySelectorAll('.terminal-pane.active').forEach(item => item.classList.remove('active'));
      paneTarget.classList.add('active');
      await loadInspector();
    }
  }
  const target = event.target.closest('button');
  if (!target) return;
  if (target.dataset.toggleExplorer !== undefined) { state.explorerOpen = !state.explorerOpen; render(); return; }
  if (target.dataset.closeShellTab) { confirmDialog('Close shell', 'This shell process will end. Other shells and the SSH connection keep running.', 'close-shell', target.dataset.closeShellTab, 'Close shell'); return; }
  if (target.dataset.shellTab) {
    const workspace = activeWorkspace();
    selectShellTab(workspace, target.dataset.shellTab, target.dataset.session);
    state.selected = { type: 'shell', id: target.dataset.shellTab };
    selectInspectorShell(target.dataset.shellTab);
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
    if (input) { const visible = input.type === 'text'; input.type = visible ? 'password' : 'text'; target.classList.toggle('active', !visible); target.setAttribute('aria-label', visible ? 'Show password' : 'Hide password'); target.title = visible ? 'Show password' : 'Hide password'; }
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
  if (target.dataset.select) {
    const [type, ...parts] = target.dataset.select.split(':');
    state.selected = { type, id: parts.join(':') };
    if (type === 'core') state.section = 'resources';
    if (type === 'history') await loadHistoryIndex(state.selected.id);
    else render();
    return;
  }
  if (target.dataset.historyShell) { await loadHistoryIndex(state.selected.id, target.dataset.historyShell); return; }
  if (target.dataset.historySpan !== undefined) { await loadHistorySpan(Number(target.dataset.historySpan)); return; }
  if (target.dataset.openSession) { openSession(target.dataset.openSession); return; }
  if (target.dataset.openShell) { openShell(target.dataset.openShell, target.dataset.session); return; }
  if (target.dataset.workspace) { state.activeWorkspace = target.dataset.workspace; state.section = 'workspace'; saveWorkspaces(); render(); return; }
  if (target.dataset.copy !== undefined) { try { await navigator.clipboard.writeText(target.dataset.copy); toast('Copied'); } catch { toast('Copy failed'); } return; }
  if (target.dataset.window) { core.window(target.dataset.window); return; }
  if (target.dataset.close !== undefined) { state.dialog = null; render(); return; }
  if (target.dataset.activePane) { const workspace = activeWorkspace(); workspace.activeShell = target.dataset.activePane; selectInspectorShell(workspace.activeShell); saveWorkspaces(); render(); return; }
  if (target.dataset.maxPane) { const workspace = activeWorkspace(); workspace.maximized = workspace.maximized ? '' : target.dataset.maxPane; saveWorkspaces(); render(); return; }
  if (target.dataset.removePane) { const workspace = activeWorkspace(); workspace.panes = workspace.panes.filter(pane => pane.shellID !== target.dataset.removePane); workspace.maximized = ''; workspace.activeShell = workspace.panes[0]?.shellID || ''; saveWorkspaces(); render(); return; }
  if (target.dataset.inspectorTab) { state.inspector.tab = target.dataset.inspectorTab; state.inspector.data = null; if (state.inspector.collapsed) { state.inspector.collapsed = false; localStorage.setItem('termcp-desktop-inspector-collapsed', '0'); } render(); return; }
  if (target.dataset.fileBrowse !== undefined) { const input = document.querySelector('[data-file-path]'); setInspectorPath(input?.value.trim() || '/'); await loadInspector(); return; }
  if (target.dataset.fileUp !== undefined) { setInspectorPath(parentPath(state.inspector.path)); await loadInspector(); return; }
  if (target.dataset.fileOpen) { if (target.dataset.isDir === '1') { setInspectorPath(target.dataset.fileOpen); await loadInspector(); } else await downloadFile(target.dataset.fileOpen); return; }
  if (target.dataset.fileDownload) { await downloadFile(target.dataset.fileDownload); return; }
  if (target.dataset.fileDelete) { confirmDialog('Delete file', `Delete ${target.dataset.fileDelete}?`, 'delete-file', target.dataset.fileDelete, 'Delete'); return; }
  if (target.dataset.fileRename) { state.dialog = { type: 'rename', title: 'Rename file', target: 'file', id: target.dataset.fileRename, value: target.dataset.fileRename.split('/').pop() }; render(); return; }
  if (target.dataset.fileUpload !== undefined) {
    await run('', () => performFileUpload({
      upload: (sessionID, directory) => core.upload(sessionID, directory),
      sessionID: state.inspector.sessionID,
      directory: state.inspector.path,
      onSuccess: () => toast('File uploaded', 'success'),
      refresh: () => loadInspector(),
    }), { refresh: false, logLabel: 'File upload' });
    return;
  }
  if (target.dataset.forwardDelete) { await run('Forward closed', () => core.api('DELETE', `/api/forwards/${encode(target.dataset.forwardDelete)}`), { refresh: false }); await loadInspector(); return; }
  if (target.dataset.notificationDelete) { await run('Notification rule removed', () => core.api('DELETE', `/api/notifications/${encode(target.dataset.notificationDelete)}`), { refresh: false }); await loadInspector(); return; }
  if (target.dataset.approvalDecision) {
    const decision = target.dataset.approvalDecision;
    target.disabled = true;
    await run(decision === 'approve' ? 'Action approved' : 'Action rejected', () => core.api('POST', `/api/approvals/${encode(target.dataset.approvalId)}/${decision}`, {}), { refresh: false });
    try { await loadApprovals(); } catch (error) { core.log('warn', 'Approval list refresh failed', String(error)); }
    render(); return;
  }
  if (target.dataset.confirm) { const action = target.dataset.confirm; const payload = target.dataset.payload; state.dialog = null; await executeConfirmed(action, payload); return; }

  const action = target.dataset.action;
  if (!action) return;
  if (action === 'refresh') { await refresh(); return; }
  if (action === 'refresh-approvals') { await run('', loadApprovals, { refresh: false, render: true }); return; }
  if (action === 'install-service') { await run('System service registered and started', () => core.installService(false)); return; }
  if (action === 'install-service-autostart') { await run('System service registered with autostart enabled', () => core.installService(true)); return; }
  if (action === 'uninstall-service') { confirmDialog('Uninstall Core system service', 'The service will stop and be removed. Core will switch back to the in-app process.', 'uninstall-service', '', 'Uninstall service'); return; }
  if (action === 'start-core') { await run('Core started', () => core.start()); return; }
  if (action === 'stop-core') { await run('Core stopped', () => core.stop()); return; }
  if (action === 'restart-core') { await run('Core restarted', () => core.restart()); return; }
  if (action === 'new-connection') { await openConnectionEditor(); return; }
  if (action === 'edit-connection') { await openConnectionEditor(target.dataset.name); return; }
  if (action === 'test-saved-connection') {
    await run('', async () => { const raw = await core.api('GET', `/api/connections/${encode(target.dataset.name)}`); return testConnection(raw.data || raw.body); }, { refresh: false }); return;
  }
  if (action === 'delete-connection') { confirmDialog('Delete connection', `Delete ${target.dataset.name}? Existing sessions will not terminate immediately.`, 'delete-connection', target.dataset.name, 'Delete'); return; }
  if (action === 'connect' || action === 'new-session') { state.dialog = { type: 'session', connection: target.dataset.connection || '' }; render(); return; }
  if (action === 'new-shell') { state.dialog = { type: 'shell', session: target.dataset.id }; render(); return; }
  if (action === 'toggle-session-approval') {
    const enabled = target.dataset.enabled === '1';
    await run(enabled ? 'Approval enabled for this session' : 'Approval disabled for this session', () => core.api('PATCH', `/api/sessions/${encode(target.dataset.id)}/approval`, { enabled, timeout_seconds: 300 }));
    return;
  }
  if (action === 'terminate-session') { confirmDialog('Terminate session', 'All shells and port forwards will close. Output will be retained in history.', 'terminate-session', target.dataset.id, 'Terminate and archive'); return; }
  if (action === 'purge-session') { confirmDialog('Delete session permanently', 'The session, messages and history will be permanently deleted.', 'purge-session', target.dataset.id, 'Delete permanently'); return; }
  if (action === 'rename-session') { const item = sessionByID(target.dataset.id) || historyByID(target.dataset.id); if (!item) return; state.dialog = { type: 'rename', title: 'Rename session', target: 'session', id: item.id, value: item.name }; render(); return; }
  if (action === 'open-selected-shell') { openShell(target.dataset.shell, target.dataset.session); return; }
  if (action === 'close-shell') { confirmDialog('Close shell', 'This shell process will end. Other shells and the SSH connection keep running.', 'close-shell', target.dataset.shell, 'Close shell'); return; }
  if (action === 'add-pane') { state.dialog = { type: 'add-pane', direction: 'right' }; render(); return; }
  if (action === 'new-directory') { state.dialog = { type: 'directory' }; render(); return; }
  if (action === 'new-forward') { state.dialog = { type: 'forward', session: state.inspector.sessionID }; render(); return; }
  if (action === 'edit-tags') { state.dialog = { type: 'tags', kind: target.dataset.kind, id: target.dataset.id }; render(); return; }
  if (action === 'export-history-log') { await run('', async () => { const path = await core.saveConversationLog(target.dataset.id, state.historyShellID); if (path) toast('Log saved', 'success'); }, { refresh: false }); return; }
  if (action === 'history-more') { await loadHistorySpan(state.historySpanIndex, true); return; }
  if (action === 'delete-history') { confirmDialog('Delete history entry', 'The transcript and related messages will be permanently deleted.', 'delete-history', target.dataset.id, 'Delete'); return; }
  if (action === 'history-search') { await searchHistory(); return; }
  if (action === 'test-editor-connection') {
    const form = target.closest('form'); if (!validateConnectionForm(form)) return;
    const resultNode = form.querySelector('.modal-result'); target.disabled = true; if (resultNode) resultNode.textContent = t('Testing connection…');
    try { const result = await testConnection(connectionFormToTOML(form), false); if (resultNode) resultNode.textContent = t(result.data?.ok ? `Connection successful${result.data.latency_ms ? ` · ${result.data.latency_ms} ms` : ''}` : 'Test complete'); } catch (error) { if (resultNode) resultNode.textContent = String(error); }
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
  if (event.target.matches('[data-tag-filter]')) { state.tagFilter = event.target.value; render(); return; }
  if (event.target.matches('[data-global-approval]')) {
    const previous = approvalSettings.defaultEnabled;
    setGlobalApproval(event.target.value === 'on');
    const saved = await run('Global approval default saved', async () => { await syncApprovalProfiles(state.data.connections); return true; });
    if (!saved) {
      setGlobalApproval(previous);
      try { await syncApprovalProfiles(state.data.connections); } catch (error) { core.log('warn', 'Approval default rollback failed', String(error)); }
      render();
    }
    return;
  }
  if (event.target.matches('[data-approval-policy]')) {
    const name = event.target.dataset.approvalPolicy;
    const previous = policyFor(name, connectionByName(name));
    setConnectionPolicy(name, event.target.value);
    const saved = await run('Connection approval default saved', async () => { await syncApprovalProfiles(state.data.connections); return true; });
    if (!saved) {
      setConnectionPolicy(name, previous);
      try { await syncApprovalProfiles(state.data.connections); } catch (error) { core.log('warn', 'Connection approval rollback failed', String(error)); }
      render();
    }
    return;
  }
  if (event.target.matches('[data-approval-notifications]')) {
    const enabled = event.target.value === 'on';
    if (enabled) {
      let allowed = false;
      try {
        allowed = await core.requestApprovalNotificationPermission();
      } catch (error) { core.log('warn', 'System notification permission failed', String(error)); }
      setDesktopNotifications(allowed);
      if (!allowed) toast('System notifications are unavailable or permission was not granted', 'warning');
    } else {
      setDesktopNotifications(false);
    }
    render(); return;
  }
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
    await run(enabled ? 'Autostart enabled' : 'Autostart disabled', () => core.setAutostart(enabled));
  }
});

document.addEventListener('keydown', async event => {
  if (event.key === 'Enter' && event.target.matches('[data-file-path]')) { event.preventDefault(); setInspectorPath(event.target.value.trim() || '/'); await loadInspector(); }
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
    if (label) label.textContent = region === 'top' ? t('Split above') : region === 'bottom' ? t('Split below') : region === 'left' ? t('Split on the left') : t('Split on the right');
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
    let startup;
    try { startup = commandLinePayload(data.get('command')); } catch (error) { toast(String(error), 'error'); return; }
    if (data.get('mode') === 'pipe' && !startup.command) { toast('Pipe mode needs a startup command', 'error'); return; }
    const connectionName = String(data.get('connection') || '');
    if (sessionCreation.pending(connectionName)) { toast('Connection already in progress'); return; }
    state.dialog = null;
    await createSessionAndOpen(connectionName, { ssh_config: connectionName, name: data.get('name'), ...startup, mode: data.get('mode'), rows: 24, cols: 100 });
    return;
  }
  if (type === 'shell') {
    let startup;
    try { startup = commandLinePayload(data.get('command')); } catch (error) { toast(String(error), 'error'); return; }
    if (data.get('mode') === 'pipe' && !startup.command) { toast('Pipe mode needs a startup command', 'error'); return; }
    state.dialog = null;
    await run('Shell created', async () => {
      const sessionID = data.get('session');
      const result = await core.api('POST', `/api/sessions/${encode(sessionID)}/shells`, { name: data.get('name'), ...startup, mode: data.get('mode'), rows: 24, cols: 100 });
      await refresh({ quiet: true }); if (result.data?.shell_id) openShell(result.data.shell_id, sessionID);
    }, { refresh: false }); return;
  }
  if (type === 'connection') {
    if (!validateConnectionForm(form)) return;
    const name = String(data.get('name')).trim(); const original = state.dialog.original;
    const policy = String(data.get('approval_policy') || 'inherit');
    const enabled = policy === 'on' || (policy === 'inherit' && approvalSettings.defaultEnabled);
    const config = connectionFormToTOML(form, enabled);
    state.dialog = null;
    const saved = await run('Connection saved', async () => {
      const result = await core.api('PUT', `/api/connections/${encode(name)}${original && original !== name ? `?from=${encode(original)}` : ''}`, config, 'text/plain; charset=utf-8');
      if (original) moveConnectionPolicy(original, name);
      setConnectionPolicy(name, policy);
      return result;
    });
    if (saved && original) moveTags('connection', original, name);
    return;
  }
  if (type === 'forward') {
    state.dialog = null;
    const payload = { direction: data.get('direction'), local_host: data.get('local_host'), local_port: Number(data.get('local_port') || 0), remote_host: data.get('remote_host'), remote_port: Number(data.get('remote_port') || 0) };
    await run('Port forward created', () => core.api('POST', `/api/sessions/${encode(data.get('session'))}/forwards`, payload), { refresh: false }); await loadInspector(); return;
  }
  if (type === 'tags') { setTags(data.get('kind'), data.get('id'), data.get('tags')); state.dialog = null; render(); return; }
  if (type === 'rename') {
    const target = data.get('target'); const id = data.get('id'); const name = String(data.get('name')).trim(); state.dialog = null;
    if (target === 'session') await run('Session renamed', () => core.api('PATCH', `/api/sessions/${encode(id)}`, { name }));
    if (target === 'workspace') { const workspace = state.workspaces.find(item => item.id === id); if (workspace) workspace.name = name; saveWorkspaces(); render(); }
    if (target === 'file') { const destination = joinPath(parentPath(id), name); await run('File renamed', () => core.api('PUT', `/api/sessions/${encode(state.inspector.sessionID)}/files?from=${encode(id)}&to=${encode(destination)}`), { refresh: false }); await loadInspector(); }
    return;
  }
  if (type === 'directory') { state.dialog = null; await run('Directory created', () => core.api('POST', `/api/sessions/${encode(state.inspector.sessionID)}/files/dir?path=${encode(data.get('path'))}`), { refresh: false }); await loadInspector(); return; }
  if (type === 'add-pane') { const direction = state.dialog?.direction || 'right'; state.dialog = null; const shellID = data.get('shell'); const item = shellByID(shellID); if (item) addSessionSplit(item.session.id, direction, '', shellID); }
});

async function executeConfirmed(action, payload) {
  if (action === 'uninstall-service') await run('System service uninstalled; Core is running in the app', () => core.uninstallService());
  if (action === 'delete-connection') { const result = await run('Connection deleted', () => core.api('DELETE', `/api/connections/${encode(payload)}`)); if (result) { setTags('connection', payload, ''); removeConnectionPolicy(payload); } }
  if (action === 'terminate-session') await run('Session terminated and archived', () => core.api('POST', `/api/sessions/${encode(payload)}/terminate`));
  if (action === 'purge-session') {
    const shells = sessionByID(payload)?.shells || [];
    const result = await run('Session permanently deleted', () => core.api('DELETE', `/api/sessions/${encode(payload)}`));
    if (result) { setTags('session', payload, ''); shells.forEach(shell => setTags('shell', shell.id, '')); }
  }
  if (action === 'delete-history') {
    const shells = historyByID(payload)?.shells || [];
    const result = await run('History entry deleted', () => core.api('DELETE', `/api/sessions/${encode(payload)}`));
    if (result) { setTags('history', payload, ''); shells.forEach(shell => setTags('shell', shell.id, '')); state.selected = { type: 'history-index', id: '' }; render(); }
  }
  if (action === 'close-shell') {
    const result = await run('Shell closed', () => core.api('DELETE', `/api/shells/${encode(payload)}`));
    if (result) setTags('shell', payload, '');
    for (const workspace of state.workspaces) removeShellFromWorkspace(workspace, payload);
    saveWorkspaces(); render();
  }
  if (action === 'delete-file') { await run('File deleted', () => core.api('DELETE', `/api/sessions/${encode(state.inspector.sessionID)}/files?path=${encode(payload)}`), { refresh: false }); await loadInspector(); }
}

async function downloadFile(filePath) {
  const name = filePath.split('/').pop() || 'download';
  await run('File saved', () => core.save(`/api/sessions/${encode(state.inspector.sessionID)}/files/download?path=${encode(filePath)}`, name), { refresh: false });
}

async function searchHistory() {
  render();
}

async function loadHistoryIndex(sessionID, shellID = '') {
  const item = historyByID(sessionID);
  const selectedShell = shellID || item?.shells?.[0]?.id || '';
  const request = ++historyRequest;
  state.historyShellID = selectedShell;
  state.historySpans = [];
  state.historySpanIndex = -1;
  state.historyText = '';
  state.historyNextOffset = 0;
  state.historyError = '';
  state.historyLoading = Boolean(selectedShell);
  render();
  if (!selectedShell) return;
  try {
    let spans = await core.conversationIndex(sessionID, selectedShell);
    if (!spans?.length) {
      const output = await core.api('GET', `/api/shells/${encode(selectedShell)}/output-range?start=0&max=1`);
      if (output.data?.total > 0) spans = [{ status: 'o', time: item.updated_at, start: 0, end: output.data.total }];
    }
    if (request !== historyRequest || state.selected.id !== sessionID) return;
    state.historySpans = Array.isArray(spans) ? spans : [];
    state.historyLoading = false;
    render();
    const firstOutput = state.historySpans.findIndex(span => span.status === 'o' && span.end > span.start);
    if (firstOutput >= 0) await loadHistorySpan(firstOutput);
  } catch (error) {
    if (request !== historyRequest) return;
    state.historyLoading = false;
    state.historyError = String(error);
    render();
  }
}

async function loadHistorySpan(index, append = false) {
  const selected = state.historySpans[index];
  if (!selected) return;
  const span = historyPreviewSpan(state.historySpans, index);
  const request = ++historyRequest;
  const sessionID = state.selected.id;
  const shellID = state.historyShellID;
  const start = append ? state.historyNextOffset : span?.start;
  state.historySpanIndex = index;
  if (!append) { state.historyText = ''; state.historyNextOffset = span?.start || 0; }
  state.historyError = '';
  render();
  if (!span) return;
  try {
    const result = await core.api('GET', `/api/shells/${encode(shellID)}/output-range?start=${start}&max=${Math.min(524288, span.end - start)}`);
    if (request !== historyRequest || state.selected.id !== sessionID || state.historyShellID !== shellID) return;
    if (append) state.historyText += result.data?.d || '';
    else state.historyText = result.data?.d || '';
    state.historyNextOffset = Math.min(span.end, Number(result.data?.end || start));
    if (state.historyNextOffset <= start) state.historyNextOffset = span.end;
    render();
  } catch (error) {
    if (request !== historyRequest) return;
    state.historyError = String(error);
    render();
  }
}

window.runtime?.EventsOn?.('termcp:navigate', section => navigateToSection(section));
window.runtime?.EventsOn?.('termcp:tray-result', (message, error) => {
  toast(error || message || 'Operation complete');
  refresh({ quiet: true });
});

core.setLanguage(getLanguage()).catch(() => {});
render();
refresh();
terminals.connect();
