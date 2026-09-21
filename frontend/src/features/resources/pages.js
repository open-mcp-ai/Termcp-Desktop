import { dot, esc, icon } from '../../ui/render.js';
import { state } from '../../state.js';
import { button, emptyPage, emptyState, header } from '../../ui/components.js';

export function connectionPage(connection) {
  if (!connection) return emptyPage();
  const sessions = state.data.sessions.filter(session => session.ssh_endpoint === connection.name || (connection.kind === 'internal' && session.ssh_endpoint === 'internal'));
  const manage = connection.kind === 'internal' ? '' : `${button(`${icon('edit', 14)}Edit`, 'edit-connection', '', `data-name="${esc(connection.name)}"`)}${button('Test', 'test-saved-connection', '', `data-name="${esc(connection.name)}"`)}${button(`${icon('trash', 14)}Delete`, 'delete-connection', 'danger', `data-name="${esc(connection.name)}"`)}`;
  return `${header(connection.name, 'SSH CONNECTION', `${manage}${button(`${icon('terminal', 14)}Connect`, 'connect', 'primary', `data-connection="${esc(connection.name)}"`)}`)}<section class="connection-hero"><div class="connection-symbol">${connection.kind === 'internal' ? '›_' : '⌁'}</div><div><span>${connection.kind === 'internal' ? 'Local connection' : 'SSH host'}</span><h2>${esc(connection.kind === 'internal' ? 'This computer' : `${connection.user || ''}@${connection.host || ''}:${connection.port || 22}`)}</h2><p>${esc(connection.description || 'Credentials are managed by termcp Core. Configuration content is read only when you edit it.')}</p></div>${dot('running')}</section><div class="section-title"><h2>Related sessions</h2><span>${sessions.length}</span></div><div class="session-cards">${sessions.map(sessionCard).join('') || emptyState('Core did not return sessions attributable to this connection. All running sessions remain available in the resource tree.')}</div>`;
}

export function sessionPage(session) {
  if (!session) return emptyPage();
  const running = session.status === 'running';
  const actions = `${running ? button(`${icon('plus', 14)}New shell`, 'new-shell', '', `data-id="${esc(session.id)}"`) : ''}${button(`${icon('edit', 14)}Rename`, 'rename-session', '', `data-id="${esc(session.id)}"`)}${running ? button('Terminate and archive', 'terminate-session', 'danger', `data-id="${esc(session.id)}"`) : ''}${button(`${icon('trash', 14)}Delete permanently`, 'purge-session', 'danger', `data-id="${esc(session.id)}"`)}`;
  return `${header(session.name, running ? 'ACTIVE SESSION' : 'DEAD SESSION', actions)}<div class="session-summary"><div><span>${dot(session.status)}${esc(session.status)}</span><code>termcp://#${esc(session.id)}</code></div><dl><div><dt>Connection type</dt><dd>${esc(session.ssh_endpoint || 'remote')}</dd></div><div><dt>Terminal mode</dt><dd>${esc(session.mode)}</dd></div><div><dt>Shell</dt><dd>${(session.shells || []).length}</dd></div></dl></div><div class="section-title"><h2>Shell resources</h2><span>Shared connection</span></div><div class="shell-grid">${(session.shells || []).map((shell, index) => shellCard(shell, session, index)).join('') || emptyState('This session has no shells.')}</div>`;
}

export function shellPage(item) {
  if (!item) return emptyPage();
  const close = item.status === 'running' ? button(`${icon('trash', 14)}Close shell`, 'close-shell', 'danger', `data-shell="${esc(item.id)}"`) : '';
  return `${header(item.name || 'shell', 'TERMINAL SHELL', `${button(`${icon('terminal', 14)}Open in workspace`, 'open-selected-shell', 'primary', `data-shell="${esc(item.id)}" data-session="${esc(item.session.id)}"`)}${close}`)}<section class="panel shell-detail"><dl><div><dt>Session</dt><dd>${esc(item.session.name)}</dd></div><div><dt>Status</dt><dd>${dot(item.status)} ${esc(item.status)}</dd></div><div><dt>Mode</dt><dd>${esc(item.mode)}</dd></div><div><dt>Resource address</dt><dd><code>termcp://#${esc(item.session.id)}:${esc(item.id)}</code></dd></div></dl></section>`;
}

const sessionCard = session => `<button class="session-card" data-select="session:${esc(session.id)}"><span class="session-icon">${icon('terminal', 18)}</span><span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint)} · ${(session.shells || []).length} Shell</small></span>${dot(session.status)}</button>`;
const shellCard = (shell, session, index) => `<button class="shell-card" data-open-shell="${esc(shell.id)}" data-session="${esc(session.id)}"><span>›_</span><div><b>${esc(shell.name || `shell-${index + 1}`)}</b><small>${esc(shell.mode)} · ${esc(shell.status)}</small><code>termcp://#${esc(session.id)}:${index + 1}</code></div>${dot(shell.status)}</button>`;

export function historyIndexPage() {
  return `${header('History entries', 'SESSION ARCHIVE', button(`${icon('search', 14)}Search`, 'history-search'))}<label class="history-search"><input value="${esc(state.historyQuery)}" placeholder="Search names, commands, tags and output" data-history-query><button class="button" data-action="history-search">Search</button></label><div class="history-list">${state.data.history.map(item => `<button data-select="history:${esc(item.id)}"><span>${icon('history', 16)}</span><span><b>${esc(item.name)}</b><small>${esc(item.updated_at || item.reason || '')}</small></span><span>${(item.tags || []).map(tag => `<em>${esc(tag)}</em>`).join('')}</span></button>`).join('') || emptyState('No history entries.')}</div>`;
}

export function historyPage(item) {
  if (!item) return historyIndexPage();
  const transcript = state.historyTranscript ? `<pre class="transcript">${esc(state.historyTranscript)}</pre>` : emptyState('Select “View transcript” to load the complete output.');
  return `${header(item.name, 'SESSION HISTORY', `${button(`${icon('edit', 14)}Edit`, 'edit-history', '', `data-id="${esc(item.id)}"`)}${button('View transcript', 'load-transcript', '', `data-id="${esc(item.id)}"`)}${button(`${icon('download', 14)}Export transcript`, 'export-transcript', '', `data-id="${esc(item.id)}"`)}${button('Export screenshot', 'export-screenshot', '', `data-id="${esc(item.id)}"`)}${button(`${icon('trash', 14)}Delete`, 'delete-history', 'danger', `data-id="${esc(item.id)}"`)}`)}<section class="panel history-detail"><span class="archive-badge">Read-only archive</span><dl><div><dt>Connect</dt><dd>${esc(item.ssh_endpoint || 'remote')}</dd></div><div><dt>Reason</dt><dd>${esc(item.reason || item.status || 'archived')}</dd></div><div><dt>Tags</dt><dd>${(item.tags || []).map(tag => `<em class="tag">${esc(tag)}</em>`).join(' ') || '—'}</dd></div><div><dt>Notes</dt><dd>${esc(item.notes || '—')}</dd></div></dl></section><div class="section-title"><h2>Transcript</h2><span>Read-only</span></div>${transcript}`;
}
