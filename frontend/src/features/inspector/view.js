import { esc, fmtSize, icon, joinPath } from '../../ui/render.js';
import { activePane, shellByID, state } from '../../state.js';

export function inspectorShell() {
  if (state.inspector.collapsed) {
    return `<button class="inspector-expand" data-toggle-inspector title="Expand session tools">${icon('collapse', 16)}</button><nav class="inspector-rail" aria-label="Session tools"><button data-inspector-tab="files" class="${state.inspector.tab === 'files' ? 'active' : ''}" title="Files">${icon('folder', 16)}</button><button data-inspector-tab="forwards" class="${state.inspector.tab === 'forwards' ? 'active' : ''}" title="Forwards">${icon('forward', 16)}</button><button data-inspector-tab="notifications" class="${state.inspector.tab === 'notifications' ? 'active' : ''}" title="Notifications">${icon('bell', 16)}</button></nav>`;
  }
  const pane = activePane();
  const item = pane && shellByID(pane.shellID);
  if (!item) return `<header class="inspector-empty-head"><div><span>Session tools</span><b>Files and forwards</b></div><div class="inspector-head-actions"><button data-toggle-inspector title="Collapse file manager">${icon('expand', 15)}</button></div></header><div class="inspector-empty">Select a terminal pane to manage files, forwards and notifications.</div>`;
  const tabs = [['files', 'Files', 'folder'], ['forwards', 'Forwards', 'forward'], ['notifications', 'Notifications', 'bell']];
  return `<header><div><span>Session tools</span><b>${esc(item.session.name)}</b></div><div class="inspector-head-actions"><code>${esc(item.name)}</code><button data-toggle-inspector title="Collapse file manager">${icon('expand', 15)}</button></div></header><nav>${tabs.map(([key, label, glyph]) => `<button data-inspector-tab="${key}" class="${state.inspector.tab === key ? 'active' : ''}">${icon(glyph, 13)}${label}</button>`).join('')}</nav><div class="inspector-body">${state.inspector.loading ? '<div class="inspector-empty">Loading…</div>' : inspectorBody(item.session)}</div>`;
}

export function inspectorBody(session) {
  if (state.inspector.error) return `<div class="inspector-error">${esc(state.inspector.error)}</div>`;
  if (state.inspector.tab === 'files') return fileInspector(session);
  if (state.inspector.tab === 'forwards') return forwardInspector(session);
  return notificationInspector(session);
}

function fileInspector(session) {
  const data = state.inspector.data;
  const rows = data?.is_dir ? (data.children || []).map(file => {
    const full = joinPath(state.inspector.path, file.name);
    return `<div class="file-item"><button data-file-open="${esc(full)}" data-is-dir="${file.is_dir ? '1' : '0'}"><span>${file.is_dir ? '▰' : '▤'}</span><span><b>${esc(file.name)}</b><small>${file.is_dir ? 'Directory' : fmtSize(file.size)}</small></span></button><div>${!file.is_dir ? `<button data-file-download="${esc(full)}" title="Download">${icon('download', 12)}</button>` : ''}<button data-file-rename="${esc(full)}" title="Rename">${icon('edit', 12)}</button><button data-file-delete="${esc(full)}" title="Delete">${icon('trash', 12)}</button></div></div>`;
  }).join('') : '';
  return `<div class="file-path"><button data-file-up title="Parent">↑</button><input value="${esc(state.inspector.path)}" data-file-path><button data-file-browse>Go</button></div><div class="inspector-actions"><button data-file-upload>${icon('plus', 12)}Upload</button><button data-action="new-directory">${icon('folder', 12)}New directory</button></div><div class="file-list">${data ? (data.is_dir ? rows || '<div class="inspector-empty">Empty directory</div>' : `<div class="file-preview"><b>${esc(data.name)}</b><span>${fmtSize(data.size)}</span><button data-file-download="${esc(state.inspector.path)}">Download file</button></div>`) : '<div class="inspector-empty">Enter an absolute path and select Go.</div>'}</div><small class="inspector-foot">${esc(session.ssh_endpoint)} · SFTP / local file API</small>`;
}

function forwardInspector(session) {
  const forwards = state.inspector.data?.forwards || [];
  return `<div class="inspector-actions"><button data-action="new-forward">${icon('plus', 12)}New forward</button></div><div class="forward-list">${forwards.map(forward => `<article><div><b>${esc(forward.direction || forward.kind)}</b><small>${esc(formatForward(forward))}</small></div><button data-forward-delete="${esc(forward.forward_id || forward.id)}">${icon('trash', 12)}</button></article>`).join('') || '<div class="inspector-empty">This session has no port forwards.</div>'}</div><small class="inspector-foot">Sessions ${esc(session.name)} closes when the session ends</small>`;
}

function notificationInspector() {
  const notifications = state.inspector.data?.notifications || [];
  return `<div class="notification-list">${notifications.map(rule => `<article><div><b>${esc(rule.event || rule.kind || 'Notification rule')}</b><small>${esc(rule.shell_id || rule.session_id || '')}</small></div><button data-notification-delete="${esc(rule.rule_id || rule.id)}">${icon('trash', 12)}</button></article>`).join('') || '<div class="inspector-empty">No active notification rules. Rules are registered by MCP shell_notify.</div>'}</div>`;
}

function formatForward(forward) {
  if (forward.listen_addr || forward.target_addr) return `${forward.listen_addr || '—'}${forward.target_addr ? ` → ${forward.target_addr}` : ''}`;
  if (forward.direction === 'dynamic') return `127.0.0.1:${forward.local_port || 0}`;
  return `${forward.local_host || '127.0.0.1'}:${forward.local_port || 0} → ${forward.remote_host || ''}:${forward.remote_port || 0}`;
}
