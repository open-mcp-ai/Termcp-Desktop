import { dot, esc, icon } from '../../ui/render.js';
import { activeWorkspace, state, workspaceSession } from '../../state.js';

const matches = (value, query) => !query || String(value).toLowerCase().includes(query);

const group = (name, content, key = name) => {
  const collapsed = state.collapsedGroups.has(key);
  return `<section class="tree-group ${collapsed ? 'collapsed' : ''}"><header><button class="tree-group-toggle" data-group-toggle="${esc(key)}" aria-expanded="${collapsed ? 'false' : 'true'}"><span>${icon('chevron', 11)}</span><b>${name}</b></button></header><div class="tree-group-content">${collapsed ? '' : (content || '<p>No resources</p>')}</div></section>`;
};

function connectionNode(connection) {
  const endpoint = connection.kind === 'internal' ? 'Local Core' : `${connection.user ? `${connection.user}@` : ''}${connection.host || 'SSH'}`;
  return `<button class="tree-row connection-profile ${state.selected.type === 'connection' && state.selected.id === connection.name ? 'selected' : ''}" data-connection-profile="${esc(connection.name)}" title="Double-click to create a session"><span class="resource-icon ${esc(connection.kind)}">${connection.kind === 'internal' ? '›_' : '⌁'}</span><span><b>${esc(connection.name)}</b><small>${esc(endpoint)}</small></span><em>Double-click</em></button>`;
}

function sessionNode(session) {
  const key = `session:${session.id}`;
  const open = state.expanded.has(key);
  const action = state.section === 'workspace' ? `data-open-session="${esc(session.id)}"` : `data-select="session:${esc(session.id)}"`;
  return `<div class="tree-node" data-session-context="${esc(session.id)}" data-session-drag="${esc(session.id)}" draggable="true"><div class="tree-row-wrap"><button class="disclosure-button ${open ? 'open' : ''}" data-toggle="${esc(key)}" aria-label="${open ? 'Collapse' : 'Expand'} ${esc(session.name)}">${icon('chevron', 11)}</button><button class="tree-row sub ${workspaceSession(activeWorkspace())?.id === session.id || (state.selected.type === 'session' && state.selected.id === session.id) ? 'selected' : ''}" ${action}>${dot(session.status)}<span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint || session.mode)} · ${(session.shells || []).length} Shell</small></span></button></div>${open ? `<div class="tree-children shells">${(session.shells || []).map(shell => shellNode(shell, session)).join('') || '<p>No shells</p>'}</div>` : ''}</div>`;
}

function shellNode(shell, session) {
  return `<button class="tree-row leaf ${state.selected.type === 'shell' && state.selected.id === shell.id ? 'selected' : ''}" data-open-shell="${esc(shell.id)}" data-session="${esc(session.id)}"><span class="terminal-glyph">›_</span><span><b>${esc(shell.name || 'shell')}</b><small>${esc(session.name)} · ${esc(shell.status)}</small></span></button>`;
}

const historyNode = item => `<button class="tree-row leaf ${state.selected.type === 'history' && state.selected.id === item.id ? 'selected' : ''}" data-select="history:${esc(item.id)}">${icon('history', 13)}<span><b>${esc(item.name)}</b><small>${esc(item.reason || item.status || 'archived')}</small></span></button>`;

export function tree(query) {
  if (state.section === 'history') {
    const items = state.data.history.filter(item => matches(`${item.name} ${(item.tags || []).join(' ')}`, query));
    return group('History entries', items.map(historyNode).join(''));
  }
  if (state.section === 'sessions' || state.section === 'workspace') {
    const sessions = state.data.sessions.filter(item => matches(`${item.name} ${item.ssh_endpoint || ''} ${item.id}`, query));
    return group('Session list', sessions.map(sessionNode).join(''), 'session-list');
  }
  if (state.section === 'service') {
    const service = state.service;
    return group('Local service', `<button class="tree-row leaf selected" data-section="service">${icon('service', 13)}<span><b>Core system service</b><small>${service.installed ? (service.running ? 'Registered · Running' : 'Registered · Stopped') : 'Not registered'}</small></span>${dot(service.running ? 'running' : 'error')}</button>`);
  }
  if (state.section === 'settings') return '<div class="tree-empty">App, Core and API</div>';
  const connections = state.data.connections.filter(item => matches(`${item.name} ${item.host || ''} ${item.user || ''}`, query));
  return group('Connections', connections.map(connectionNode).join(''), 'connection-list');
}

export function rail() {
  const items = [['resources', 'Connect', 'resources'], ['workspace', 'Terminal', 'terminal'], ['history', 'History', 'history'], ['service', 'Service', 'service']];
  const item = ([key, label, glyph]) => `<button data-section="${key}" class="${state.section === key || (key === 'workspace' && state.section === 'sessions') ? 'active' : ''}" title="${label}" aria-label="${label}"><span>${icon(glyph, 19)}</span><small>${label}</small></button>`;
  return `<aside class="rail"><nav aria-label="Main navigation">${items.map(item).join('')}</nav><div class="rail-bottom">${item(['settings', 'Settings', 'settings'])}</div></aside>`;
}

export function explorer() {
  const workspace = state.section === 'workspace';
  const sessionArea = workspace || state.section === 'sessions';
  return `<aside class="explorer"><div class="explorer-head"><div><span>${sessionArea ? 'Sessions' : 'Connections'}</span><small>${sessionArea ? 'SESSION WORKSPACE' : 'DOUBLE-CLICK TO CONNECT'}</small></div><div class="mini-actions"><button class="icon-btn ${state.loading ? 'spinning' : ''}" data-action="refresh" title="Refresh resources">${icon('refresh', 14)}</button>${sessionArea ? `<button class="icon-btn" data-action="new-session" title="New session">${icon('plus', 14)}</button>` : `<button class="icon-btn" data-action="new-connection" title="New connection">${icon('plus', 14)}</button>`}</div></div><label class="search">${icon('search', 14)}<input type="search" placeholder="${sessionArea ? 'Filter sessions' : 'Filter connections'}" value="${esc(state.filter)}" data-filter></label><div class="tree-scroll">${tree(state.filter.trim().toLowerCase())}</div></aside>`;
}
