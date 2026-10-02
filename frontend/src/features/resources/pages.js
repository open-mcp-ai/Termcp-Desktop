import { dot, esc, icon } from '../../ui/render.js';
import { state } from '../../state.js';
import { button, emptyPage, emptyState, header } from '../../ui/components.js';
import { tagsFor } from './tags.js';
import { effectiveApproval, policyFor } from '../approval/policy.js';
import { historyPreviewSpan, readableTerminalText } from './history.js';

function tagSection(type, id, label = 'Tags') {
  const tags = tagsFor(type, id);
  return `<section class="resource-tags"><div><b>${label}</b>${tags.map(tag => `<span class="tag">${esc(tag)}</span>`).join('') || '<small>No tags</small>'}</div>${button('Edit tags', 'edit-tags', '', `data-kind="${esc(type)}" data-id="${esc(id)}"`)}</section>`;
}

export function connectionPage(connection) {
  if (!connection) return emptyPage();
  const sessions = state.data.sessions.filter(session => session.ssh_endpoint === connection.name || (connection.kind === 'internal' && session.ssh_endpoint === 'internal'));
  const manage = connection.kind === 'internal' ? '' : `${button(`${icon('edit', 14)}Edit`, 'edit-connection', '', `data-name="${esc(connection.name)}"`)}${button('Test', 'test-saved-connection', '', `data-name="${esc(connection.name)}"`)}${button(`${icon('trash', 14)}Delete`, 'delete-connection', 'danger', `data-name="${esc(connection.name)}"`)}`;
  const policy = policyFor(connection.name, connection);
  const approval = `<section class="resource-tags approval-policy-row"><div><b>Approval for new sessions</b><small>Effective: ${effectiveApproval(connection.name, connection) ? 'On' : 'Off'} · AI Shell, file changes and port forwards</small></div><select data-approval-policy="${esc(connection.name)}" aria-label="Approval policy for ${esc(connection.name)}"><option value="inherit" ${policy === 'inherit' ? 'selected' : ''}>Use global default</option><option value="on" ${policy === 'on' ? 'selected' : ''}>Always on</option><option value="off" ${policy === 'off' ? 'selected' : ''}>Always off</option></select></section>`;
  return `${header(connection.name, 'SSH CONNECTION', `${manage}${button(`${icon('terminal', 14)}Connect`, 'connect', 'primary', `data-connection="${esc(connection.name)}"`)}`)}<section class="connection-hero"><div class="connection-symbol">${connection.kind === 'internal' ? '›_' : '⌁'}</div><div><span>${connection.kind === 'internal' ? 'Local connection' : 'SSH host'}</span><h2>${esc(connection.kind === 'internal' ? 'This computer' : `${connection.user || ''}@${connection.host || ''}:${connection.port || 22}`)}</h2><p>${esc(connection.description || 'Credentials are managed by termcp Core. Configuration content is read only when you edit it.')}</p></div></section>${approval}${tagSection('connection', connection.name)}<div class="section-title"><h2>Related sessions</h2><span>${sessions.length}</span></div><div class="session-cards">${sessions.map(sessionCard).join('') || emptyState('Core did not return sessions attributable to this connection. All running sessions remain available in the resource tree.')}</div>`;
}

export function sessionPage(session) {
  if (!session) return emptyPage();
  const running = session.status === 'running';
  const actions = `${running ? button(`${icon('plus', 14)}New shell`, 'new-shell', '', `data-id="${esc(session.id)}"`) : ''}${button(`${icon('edit', 14)}Rename`, 'rename-session', '', `data-id="${esc(session.id)}"`)}${running ? button('Terminate and archive', 'terminate-session', 'danger', `data-id="${esc(session.id)}"`) : ''}${button(`${icon('trash', 14)}Delete permanently`, 'purge-session', 'danger', `data-id="${esc(session.id)}"`)}`;
  const approval = running ? `<section class="resource-tags approval-policy-row"><div><b>AI approval</b><small>${session.approval_mode ? 'On · AI operations wait for your decision' : 'Off · AI operations run immediately'}</small></div>${button(session.approval_mode ? 'Turn off' : 'Turn on', 'toggle-session-approval', '', `data-id="${esc(session.id)}" data-enabled="${session.approval_mode ? '0' : '1'}"`)}</section>` : '';
  return `${header(session.name, running ? 'ACTIVE SESSION' : 'DEAD SESSION', actions)}<div class="session-summary"><div><span>${dot(session.status)}${esc(session.status)}</span><code>termcp://#${esc(session.id)}</code></div><dl><div><dt>Connection type</dt><dd>${esc(session.ssh_endpoint || 'remote')}</dd></div><div><dt>Status</dt><dd>${esc(session.status)}</dd></div><div><dt>Shells</dt><dd>${(session.shells || []).length}</dd></div></dl></div>${approval}${tagSection('session', session.id)}<div class="section-title"><h2>Shell resources</h2><span>Shared connection</span></div><div class="shell-grid">${(session.shells || []).map((shell, index) => shellCard(shell, session, index)).join('') || emptyState('This session has no shells.')}</div>`;
}

export function shellPage(item) {
  if (!item) return emptyPage();
  const close = item.status === 'running' ? button(`${icon('trash', 14)}Close shell`, 'close-shell', 'danger', `data-shell="${esc(item.id)}"`) : '';
  return `${header(item.name || 'shell', 'TERMINAL SHELL', `${button(`${icon('terminal', 14)}Open in workspace`, 'open-selected-shell', 'primary', `data-shell="${esc(item.id)}" data-session="${esc(item.session.id)}"`)}${close}`)}<section class="panel shell-detail"><dl><div><dt>Session</dt><dd>${esc(item.session.name)}</dd></div><div><dt>Status</dt><dd>${dot(item.status)} ${esc(item.status)}</dd></div><div><dt>Mode</dt><dd>${esc(item.mode)}</dd></div><div><dt>Resource address</dt><dd><code>termcp://#${esc(item.session.id)}:${esc(item.id)}</code></dd></div></dl></section>${tagSection('shell', item.id)}`;
}

const sessionCard = session => `<button class="session-card" data-select="session:${esc(session.id)}"><span class="session-icon">${icon('terminal', 18)}</span><span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint)} · ${(session.shells || []).length} Shell</small></span>${dot(session.status)}</button>`;
const shellCard = (shell, session, index) => `<button class="shell-card" data-open-shell="${esc(shell.id)}" data-session="${esc(session.id)}"><span>›_</span><div><b>${esc(shell.name || `shell-${index + 1}`)}</b><small>${esc(shell.mode)} · ${esc(shell.status)}</small><code>termcp://#${esc(session.id)}:${index + 1}</code></div>${dot(shell.status)}</button>`;

export function historyIndexPage() {
  const query = state.historyQuery.trim().toLowerCase();
  const items = state.data.history.filter(item => {
    const shellTags = (item.shells || []).flatMap(shell => tagsFor('shell', shell.id));
    return (!state.tagFilter || tagsFor('history', item.id).includes(state.tagFilter) || shellTags.includes(state.tagFilter))
      && `${item.name} ${item.id} ${tagsFor('history', item.id).join(' ')} ${shellTags.join(' ')}`.toLowerCase().includes(query);
  });
  return `${header('Conversation history', 'READ-ONLY SESSIONS')}<label class="history-search"><input value="${esc(state.historyQuery)}" placeholder="Filter by name, ID or tag" data-history-query><button class="button" data-action="history-search">Filter</button></label><div class="history-list">${items.map(item => `<button data-select="history:${esc(item.id)}"><span>${icon('history', 16)}</span><span><b>${esc(item.name)}</b><small>${esc(formatHistoryTime(item.updated_at))} · ${(item.shells || []).length} shells</small></span><span>${tagsFor('history', item.id).map(tag => `<em>${esc(tag)}</em>`).join('')}</span></button>`).join('') || emptyState('No archived conversations match this filter.')}</div>`;
}

function formatHistoryTime(value) {
  const time = typeof value === 'number' ? value : Date.parse(value || '');
  return Number.isFinite(time) && time > 0 ? new Date(time).toLocaleString() : 'Time unavailable';
}

const spanLabels = { o: 'Terminal output', a: 'AI input', i: 'Human input', q: 'Approval requested', A: 'Approved input' };

export function historyPage(item) {
  if (!item) return historyIndexPage();
  const shellID = state.historyShellID || item.shells?.[0]?.id || '';
  const spans = state.historySpans;
  const selected = spans[state.historySpanIndex];
  const preview = historyPreviewSpan(spans, state.historySpanIndex);
  const more = preview && state.historyNextOffset < preview.end && state.historyText ? button('Load more output', 'history-more') : '';
  const transcript = state.historyError ? `<div class="inspector-error">${esc(state.historyError)}</div>` : state.historyLoading ? emptyState('Loading conversation index…') : selected ? preview ? `<pre class="transcript">${esc(state.historyText ? readableTerminalText(state.historyText) : 'Loading output…')}</pre>${more}` : emptyState(['i', 'a', 'A'].includes(selected.status) ? 'Input events are markers only; Core does not store their text.' : 'This event has no terminal output bytes.') : emptyState('Select an index entry to inspect its output.');
  const actions = button(`${icon('edit', 14)}Rename`, 'rename-session', '', `data-id="${esc(item.id)}"`)
    + (shellID ? button(`${icon('download', 14)}Export raw log`, 'export-history-log', '', `data-id="${esc(item.id)}"`) : '')
    + button(`${icon('trash', 14)}Delete permanently`, 'delete-history', 'danger', `data-id="${esc(item.id)}"`);
  const shellTabs = (item.shells || []).map(shell => `<button class="button ${shell.id === shellID ? 'active' : ''}" data-history-shell="${esc(shell.id)}">${esc(shell.name || shell.id)} · ${esc(shell.mode || 'pty')}</button>`).join('') || '<span>No retained shells</span>';
  const events = spans.map((span, index) => `<button class="history-event ${index === state.historySpanIndex ? 'active' : ''}" data-history-span="${index}"><b>${esc(spanLabels[span.status] || span.status)}</b><small>${esc(formatHistoryTime(span.time))}</small><code>${span.start}–${span.end}</code></button>`).join('') || emptyState('No indexed events for this shell.');
  return `<div class="history-page">${header(item.name, 'CONVERSATION HISTORY', actions)}
    <section class="panel history-detail"><span class="archive-badge">Read-only session</span><dl><div><dt>Connection</dt><dd>${esc(item.ssh_endpoint || 'remote')}</dd></div><div><dt>Ended</dt><dd>${esc(formatHistoryTime(item.updated_at))}</dd></div><div><dt>Shells</dt><dd>${(item.shells || []).length}</dd></div></dl></section>
    ${tagSection('history', item.id)}
    <div class="history-workspace">
      <section class="history-events-panel" data-shell-id="${esc(shellID)}" aria-label="Shell timeline">
        <div class="section-title"><h2>Shell timeline</h2></div>
        <p class="history-index-note">Input markers record submitted lines. Output ranges are separate; the index does not pair commands with results.</p>
        <div class="history-shells">${shellTabs}</div>
        ${shellID ? tagSection('shell', shellID, 'Shell tags') : ''}
        <div class="history-timeline" data-shell-id="${esc(shellID)}">${events}</div>
      </section>
      <section class="history-output-panel" aria-label="Event content">
        <div class="section-title"><h2>${esc(selected ? spanLabels[selected.status] || selected.status : 'Output')}</h2><span>${selected ? `${preview ? preview.end - preview.start : 0} bytes` : 'Select an event'}</span></div>
        <div class="history-output-scroll" data-shell-id="${esc(shellID)}" data-span-index="${state.historySpanIndex}">${transcript}</div>
      </section>
    </div></div>`;
}
