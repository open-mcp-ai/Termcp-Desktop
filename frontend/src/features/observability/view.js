import { esc, icon } from '../../ui/render.js';
import { emptyState, header } from '../../ui/components.js';
import { activityLabel, activitySessionLabel, activityTime } from './model.js';

function activityRow(event, sessions) {
  const shell = event.shell_id ? String(event.shell_id) : '';
  const label = activityLabel(event);
  const source = ['ai', 'human', 'system'].includes(event.source) ? event.source : 'system';
  return `<button class="observability-activity" data-activity-open data-session="${esc(event.session_id)}" data-shell="${esc(shell)}" data-byte="${esc(event.bytes ?? 0)}">
    <span class="activity-source ${source}" aria-hidden="true">${source === 'ai' ? 'AI' : source === 'human' ? 'H' : 'R'}</span>
    <span class="activity-copy"><b>${esc(activitySessionLabel(event, sessions))}</b><small>${esc(label)} · ${esc(shell || 'session event')} · ${esc(activityTime(event.time))}</small></span>
    <code>${esc(Number(event.bytes || 0).toLocaleString())}</code>
  </button>`;
}

function activitySection(state, sessions) {
  const data = state.observability;
  const options = sessions.map(session => `<option value="${esc(session.id)}" ${data.sessionID === session.id ? 'selected' : ''}>${esc(session.name || session.id)}</option>`).join('');
  return `<section class="observability-section activity-section">
    <header><div><span>ACTIVITY</span><h2>Agent and human actions</h2></div><label>Session<select data-activity-session><option value="">All sessions</option>${options}</select></label></header>
    <div class="activity-list">${data.activity.map(event => activityRow(event, sessions)).join('') || emptyState('No agent, human or review activity has been recorded.')}</div>
  </section>`;
}

function clientSection(clients) {
  const webui = clients.webui.map(client => `<article class="observability-client">
    <span class="client-kind">WEB</span>
    <div><b>${esc(client.user_agent || client.addr || 'Web UI client')}${client.self ? ' · This window' : ''}</b><small>${esc(client.addr || '')} · watching ${esc(client.watching ?? 0)} · since ${esc(activityTime(client.since))}</small></div>
  </article>`).join('');
  const mcp = clients.mcp.map(client => `<article class="observability-client">
    <span class="client-kind mcp">MCP</span>
    <div><b>${esc([client.name, client.version].filter(Boolean).join(' ') || client.session_id || 'MCP client')}</b><small>${esc(client.namespace || 'instance owner')} · since ${esc(activityTime(client.connected_at))}</small></div>
  </article>`).join('');
  return `<section class="observability-section client-section">
    <header><div><span>ATTACHED</span><h2>Connected clients</h2></div></header>
    <h3>Web UI <em>${clients.webui.length}</em></h3>
    <div class="client-list">${webui || emptyState('No Web UI windows are attached.')}</div>
    <h3>MCP <em>${clients.mcp.length}</em></h3>
    <div class="client-list">${mcp || emptyState('No MCP clients are attached.')}</div>
  </section>`;
}

export function observabilityPage(state) {
  const data = state.observability;
  const sessions = [...(state.data.sessions || []), ...(state.data.history || [])];
  const webui = data.clients.webui.length;
  const mcp = data.clients.mcp.length;
  return `${header('Runtime observability', 'AGENT ACTIVITY AND ATTACHED CLIENTS', `<button class="button" data-action="refresh-observability">${icon('refresh', 14)}Refresh</button>`)}
    <div class="observability-metrics"><div><small>ACTIONS</small><strong>${data.activity.length}</strong></div><div><small>WEB UI</small><strong>${webui}</strong></div><div><small>MCP</small><strong>${mcp}</strong></div><div><small>EVENT CHANNEL</small><strong>${esc(state.wsStatus)}</strong></div></div>
    ${data.error ? `<div class="error-banner">${esc(data.error)}</div>` : ''}
    <div class="observability-layout">${activitySection(state, sessions)}${clientSection(data.clients)}</div>`;
}
