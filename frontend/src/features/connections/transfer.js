import { esc, icon } from '../../ui/render.js';
import { state } from '../../state.js';

export function exportableConnections(connections = state.data.connections) {
  return (Array.isArray(connections) ? connections : []).filter(connection => connection?.name && connection.kind !== 'internal');
}

export function exportConnectionsBody() {
  const connections = exportableConnections();
  const rows = connections.map(connection => `<label class="transfer-host">
    <input type="checkbox" name="connections" value="${esc(connection.name)}" checked>
    <span><b>${esc(connection.name)}</b><small>${esc(connection.kind === 'internal' ? 'Local Core' : `${connection.user ? `${connection.user}@` : ''}${connection.host || 'SSH'}`)}</small></span>
  </label>`).join('');
  return `<p>Export remote host configurations as one document. The built-in local profile is always excluded.</p>
    <label class="transfer-select-all"><input type="checkbox" data-select-all-connections checked ${connections.length ? '' : 'disabled'}> <span>Select all remote hosts</span></label>
    <div class="transfer-host-list" role="group" aria-label="Hosts to export">${rows || '<p class="transfer-empty">No remote host configurations are available to export.</p>'}</div>
    <label>Password (optional)<input name="password" type="password" autocomplete="new-password" placeholder="Leave blank for readable TOML"></label>
    <p class="transfer-hint">A password exports an encrypted .enc file. It is sent to Core only in the request header.</p>
    <p class="modal-result" role="status">${esc(state.dialog?.error || '')}</p>`;
}

export function importConnectionsBody() {
  return `<p>Choose a Termcp connection export after entering its password. Plain TOML files do not need a password.</p>
    <label>Password<input name="password" type="password" autocomplete="current-password" placeholder="Only required for .enc files"></label>
    <p class="transfer-hint">The password is passed to Core in a request header and is never placed in the file path or URL.</p>
    <p class="modal-result" role="status">${esc(state.dialog?.error || '')}</p>`;
}

export function importResultBody(result = {}) {
  const hosts = Array.isArray(result.hosts) ? result.hosts : [];
  const summary = `${Number(result.imported || 0)} imported${result.renamed ? ` · ${result.renamed} renamed` : ''}${result.failed ? ` · ${result.failed} failed` : ''}`;
  const rows = hosts.map(host => {
    const status = host.status || 'failed';
    const finalName = host.final_name && host.final_name !== host.name ? `<small>Created as ${esc(host.final_name)}</small>` : '';
    const failure = host.error ? `<small class="transfer-host-error">${esc(host.error)}</small>` : '';
    return `<div class="transfer-result-row"><span class="transfer-result-status status-${esc(status)}">${esc(status)}</span><span><b>${esc(host.name)}</b>${finalName}${failure}</span></div>`;
  }).join('');
  return `<div class="transfer-result-summary">${icon('download', 18)}<span><b>${esc(summary)}</b><small>${esc(result.file_name || 'Connection import')}</small></span></div>
    <div class="transfer-result-list">${rows || '<p class="transfer-empty">Core returned no per-host result.</p>'}</div>`;
}
