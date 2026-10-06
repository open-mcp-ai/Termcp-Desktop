import { dot, esc, icon } from '../../ui/render.js';
import { activeWorkspace, state, workspaceSession } from '../../state.js';
import { knownTags, tagsFor } from './tags.js';

const matches = (value, query) => !query || String(value).toLowerCase().includes(query);

const group = (name, content, key = name, actions = '') => {
  const collapsed = state.collapsedGroups.has(key);
  return `<section class="tree-group ${collapsed ? 'collapsed' : ''}"><header><button class="tree-group-toggle" data-group-toggle="${esc(key)}" aria-expanded="${collapsed ? 'false' : 'true'}"><span>${icon('chevron', 11)}</span><b>${name}</b></button>${actions}</header><div class="tree-group-content">${collapsed ? '' : (content || '<p>No resources</p>')}</div></section>`;
};

function connectionNode(connection) {
  const endpoint = connection.kind === 'internal' ? 'Local Core' : `${connection.user ? `${connection.user}@` : ''}${connection.host || 'SSH'}`;
  const tags = tagsFor('connection', connection.name);
  return `<button class="tree-row connection-profile ${state.selected.type === 'connection' && state.selected.id === connection.name ? 'selected' : ''}" data-connection-profile="${esc(connection.name)}" title="Double-click to create a session"><span class="resource-icon ${esc(connection.kind)}">${connection.kind === 'internal' ? '›_' : '⌁'}</span><span><b>${esc(connection.name)}</b><small>${esc(endpoint)}${tags.length ? ` · #${esc(tags.join(' #'))}` : ''}</small></span><em>Double-click</em></button>`;
}

function sessionNode(session) {
  const key = `session:${session.id}`;
  const taggedShells = (session.shells || []).filter(shell => tagMatch('shell', shell.id));
  const open = state.expanded.has(key) || Boolean(state.tagFilter && taggedShells.length);
  const visibleShells = state.tagFilter && !tagMatch('session', session.id) ? taggedShells : session.shells || [];
  const action = state.section === 'workspace' ? `data-open-session="${esc(session.id)}"` : `data-select="session:${esc(session.id)}"`;
  const tags = tagsFor('session', session.id);
  return `<div class="tree-node" data-session-context="${esc(session.id)}" data-session-drag="${esc(session.id)}" draggable="true"><div class="tree-row-wrap"><button class="disclosure-button ${open ? 'open' : ''}" data-toggle="${esc(key)}" aria-label="${open ? 'Collapse' : 'Expand'} ${esc(session.name)}">${icon('chevron', 11)}</button><button class="tree-row sub ${workspaceSession(activeWorkspace())?.id === session.id || (state.selected.type === 'session' && state.selected.id === session.id) ? 'selected' : ''}" ${action}>${dot(session.status)}<span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint || 'remote')} · ${(session.shells || []).length} Shell${tags.length ? ` · #${esc(tags.join(' #'))}` : ''}</small></span></button></div>${open ? `<div class="tree-children shells">${visibleShells.map(shell => shellNode(shell, session)).join('') || '<p>No shells</p>'}</div>` : ''}</div>`;
}

function shellNode(shell, session) {
  const tags = tagsFor('shell', shell.id);
  return `<button class="tree-row leaf ${state.selected.type === 'shell' && state.selected.id === shell.id ? 'selected' : ''}" data-open-shell="${esc(shell.id)}" data-session="${esc(session.id)}"><span class="terminal-glyph">›_</span><span><b>${esc(shell.name || 'shell')}</b><small>${esc(session.name)} · ${esc(shell.status)}${tags.length ? ` · #${esc(tags.join(' #'))}` : ''}</small></span></button>`;
}

const historyNode = item => `<button class="tree-row leaf ${state.selected.type === 'history' && state.selected.id === item.id ? 'selected' : ''}" data-select="history:${esc(item.id)}">${icon('history', 13)}<span><b>${esc(item.name)}</b><small>${esc(item.status || 'exited')} · ${(item.shells || []).length} shells${tagsFor('history', item.id).length ? ` · #${esc(tagsFor('history', item.id).join(' #'))}` : ''}</small></span></button>`;

function tagMatch(type, id) { return !state.tagFilter || tagsFor(type, id).includes(state.tagFilter); }
function sessionTagMatch(item, type) { return tagMatch(type, item.id) || (item.shells || []).some(shell => tagMatch('shell', shell.id)); }

export function tree(query) {
  if (state.section === 'history') {
    const items = state.data.history.filter(item => sessionTagMatch(item, 'history') && matches(`${item.name} ${tagsFor('history', item.id).join(' ')} ${(item.shells || []).flatMap(shell => tagsFor('shell', shell.id)).join(' ')}`, query));
    return group('History entries', items.map(historyNode).join(''));
  }
  if (['sessions', 'workspace', 'resources'].includes(state.section)) {
    const sessions = state.data.sessions.filter(item => sessionTagMatch(item, 'session') && matches(`${item.name} ${item.ssh_endpoint || ''} ${item.id} ${tagsFor('session', item.id).join(' ')} ${(item.shells || []).flatMap(shell => tagsFor('shell', shell.id)).join(' ')}`, query));
    const connections = state.data.connections.filter(item => tagMatch('connection', item.name) && matches(`${item.name} ${item.host || ''} ${item.user || ''} ${tagsFor('connection', item.name).join(' ')}`, query));
    const add = `<button class="tree-group-add icon-btn" data-action="new-connection" title="New host configuration" aria-label="New host configuration">${icon('plus', 14)}</button>`;
    return group('Host configurations', connections.map(connectionNode).join(''), 'connection-list', add)
      + group('Session list', sessions.map(sessionNode).join(''), 'session-list');
  }
  if (state.section === 'settings') return '<div class="tree-empty">App, Core and API</div>';
  const connections = state.data.connections.filter(item => tagMatch('connection', item.name) && matches(`${item.name} ${item.host || ''} ${item.user || ''} ${tagsFor('connection', item.name).join(' ')}`, query));
  return group('Connections', connections.map(connectionNode).join(''), 'connection-list');
}

export function rail() {
  const items = [['workspace', 'Connection', 'resources'], ['history', 'History', 'history']];
  const item = ([key, label, glyph]) => `<button data-section="${key}" class="${state.section === key || (key === 'workspace' && ['sessions', 'resources'].includes(state.section)) ? 'active' : ''}" title="${label}" aria-label="${label}"><span>${icon(glyph, 19)}</span><small>${label}</small></button>`;
  return `<aside class="rail"><button class="rail-sidebar-toggle" data-sidebar-toggle title="${state.sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}" aria-label="${state.sidebarOpen ? 'Collapse sidebar' : 'Expand sidebar'}" aria-expanded="${state.sidebarOpen}">${icon(state.sidebarOpen ? 'collapse' : 'expand', 17)}</button><nav aria-label="Main navigation">${items.map(item).join('')}</nav><div class="rail-bottom">${item(['settings', 'Settings', 'settings'])}</div></aside>`;
}

export function explorer() {
  const workspace = state.section === 'workspace';
  const sessionArea = workspace || state.section === 'sessions' || state.section === 'resources';
  const resources = state.section === 'history' ? state.data.history.flatMap(item => [{ type: 'history', id: item.id }, ...(item.shells || []).map(shell => ({ type: 'shell', id: shell.id }))]) : sessionArea ? [...state.data.connections.map(item => ({ type: 'connection', id: item.name })), ...state.data.sessions.flatMap(item => [{ type: 'session', id: item.id }, ...(item.shells || []).map(shell => ({ type: 'shell', id: shell.id }))])] : [];
  const tags = knownTags(resources);
  return `<aside class="explorer"><div class="explorer-head"><div><span>${state.section === 'history' ? 'History' : 'Connection'}</span><small>${sessionArea ? 'SESSION WORKSPACE' : 'RESOURCE EXPLORER'}</small></div><div class="mini-actions"><button class="icon-btn ${state.loading ? 'spinning' : ''}" data-action="refresh" title="Refresh resources">${icon('refresh', 14)}</button>${sessionArea ? `<button class="icon-btn" data-action="new-session" title="New session">${icon('plus', 14)}</button>` : ''}<button class="icon-btn sidebar-pin ${state.sidebarPinned ? 'active' : ''}" data-sidebar-pin title="${state.sidebarPinned ? 'Unpin sidebar' : 'Pin sidebar'}" aria-label="${state.sidebarPinned ? 'Unpin sidebar' : 'Pin sidebar'}" aria-pressed="${state.sidebarPinned}">${icon('pin', 15)}</button><button class="icon-btn" data-sidebar-toggle title="Collapse sidebar" aria-label="Collapse sidebar">${icon('collapse', 15)}</button></div></div><label class="search">${icon('search', 14)}<input type="search" placeholder="${sessionArea ? 'Filter connections and sessions' : 'Filter history'}" value="${esc(state.filter)}" data-filter></label>${tags.length ? `<select class="tag-filter" data-tag-filter aria-label="Filter by tag"><option value="">All tags</option>${tags.map(tag => `<option value="${esc(tag)}" ${state.tagFilter === tag ? 'selected' : ''}>${esc(tag)}</option>`).join('')}</select>` : ''}<div class="tree-scroll">${tree(state.filter.trim().toLowerCase())}</div></aside>`;
}
