import { esc, joinPath } from '../ui/render.js';
import { state } from '../state.js';
import { emptyConnectionProfile } from './connections/profile.js';
import { connectionEditor } from './connections/editor.js';

export function modal() {
  const dialog = state.dialog;
  if (!dialog) return '';
  let body = '';
  let title = '';
  let eyebrow = '';
  let footer = '';
  if (dialog.type === 'session') {
    title = 'Connect to host'; eyebrow = 'NEW SESSION';
    body = `<label>Connections<select name="connection">${state.data.connections.map(item => `<option value="${esc(item.name)}" ${dialog.connection === item.name ? 'selected' : ''}>${esc(item.name)} · ${esc(item.kind === 'internal' ? 'Local' : item.host)}</option>`).join('')}</select></label><label>Session name<input name="name" placeholder="Defaults to the connection name"></label><label>Startup command<input name="command" placeholder="Leave blank to open the default shell"></label><label>Terminal mode<select name="mode"><option value="pty">PTY interactive terminal</option><option value="pipe">Pipe non-interactive</option></select></label>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit">Connect and open</button>`;
  } else if (dialog.type === 'shell') {
    title = 'New shell'; eyebrow = 'SESSION CHANNEL';
    body = `<input type="hidden" name="session" value="${esc(dialog.session)}"><label>Shell name<input name="name" value="shell" required></label><label>Startup command<input name="command" placeholder="Leave blank to use the default shell"></label><label>Terminal mode<select name="mode"><option value="pty">PTY</option><option value="pipe">Pipe</option></select></label>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit">Create</button>`;
  } else if (dialog.type === 'connection') {
    title = dialog.original ? `Edit ${dialog.original}` : 'New SSH connection'; eyebrow = 'SSH PROFILE';
    body = connectionEditor(dialog.profile || emptyConnectionProfile(dialog.name), dialog.loading);
    footer = `<div class="modal-result" role="status">${esc(dialog.result || (dialog.loading ? 'Loading connection profile…' : 'Credentials are stored only under ~/.termcp on this computer.'))}</div><button type="button" class="button" data-action="test-editor-connection" ${dialog.loading ? 'disabled' : ''}>Test connection</button><span class="modal-spacer"></span><button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit" ${dialog.loading ? 'disabled' : ''}>Save</button>`;
  } else if (dialog.type === 'forward') {
    title = 'New port forward'; eyebrow = 'SSH FORWARD';
    body = `<input type="hidden" name="session" value="${esc(dialog.session)}"><label>Direction<select name="direction"><option value="local">Local · -L</option><option value="remote">Remote · -R</option><option value="dynamic">Dynamic · -D</option></select></label><div class="form-grid"><label>Local address<input name="local_host" value="127.0.0.1"></label><label>Local port<input name="local_port" type="number" min="0" max="65535" value="0"></label><label>Remote address<input name="remote_host" value="127.0.0.1"></label><label>Remote port<input name="remote_port" type="number" min="0" max="65535" value="80"></label></div>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit">Create</button>`;
  } else if (dialog.type === 'history') {
    title = 'Edit history entry'; eyebrow = 'SESSION ARCHIVE';
    body = `<input type="hidden" name="id" value="${esc(dialog.item.id)}"><label>Name<input name="name" value="${esc(dialog.item.name)}" required></label><label>Tags<input name="tags" value="${esc((dialog.item.tags || []).join(', '))}" placeholder="release, validation"></label><label>Notes<textarea name="notes" rows="5">${esc(dialog.item.notes || '')}</textarea></label>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit">Save</button>`;
  } else if (dialog.type === 'rename') {
    title = dialog.title || 'Rename'; eyebrow = 'RENAME';
    body = `<input type="hidden" name="target" value="${esc(dialog.target || '')}"><input type="hidden" name="id" value="${esc(dialog.id || '')}"><label>New name<input name="name" value="${esc(dialog.value || '')}" required autofocus></label>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit">Save</button>`;
  } else if (dialog.type === 'directory') {
    title = 'New directory'; eyebrow = 'FILE MANAGER';
    body = `<label>Directory path<input name="path" value="${esc(joinPath(state.inspector.path, 'new-directory'))}" required autofocus></label>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit">Create</button>`;
  } else if (dialog.type === 'add-pane') {
    title = 'Add terminal pane'; eyebrow = 'WORKSPACE';
    const options = state.data.sessions.flatMap(session => (session.shells || []).map(shell => `<option value="${esc(shell.id)}" data-session="${esc(session.id)}">${esc(session.name)} / ${esc(shell.name || 'shell')}</option>`)).join('');
    body = `<label>Shell<select name="shell" required>${options}</select></label>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button class="button primary" type="submit">Add</button>`;
  } else if (dialog.type === 'confirm') {
    title = dialog.title; eyebrow = 'CONFIRM'; body = `<p>${esc(dialog.message)}</p>`;
    footer = `<button type="button" class="button" data-close>Cancel</button><button type="button" class="button danger" data-confirm="${esc(dialog.action)}" data-payload="${esc(dialog.payload || '')}">${esc(dialog.confirm || 'Confirm')}</button>`;
  }
  return `<div class="modal-backdrop"><form class="modal ${dialog.type === 'connection' ? 'wide connection-modal' : ''}" data-form="${esc(dialog.type)}" autocomplete="off" ${dialog.type === 'connection' ? 'novalidate' : ''}><header><div><span>${eyebrow}</span><h2>${esc(title)}</h2></div><button type="button" class="icon-btn" data-close>×</button></header>${body}<footer>${footer}</footer></form></div>`;
}
