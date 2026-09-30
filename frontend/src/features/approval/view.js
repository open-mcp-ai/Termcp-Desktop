import { esc, icon } from '../../ui/render.js';
import { state } from '../../state.js';
import { button, emptyState, header } from '../../ui/components.js';

const labels = {
  shell_input: 'Shell command', file_write: 'File write', file_delete: 'File delete',
  file_rename: 'File rename', file_mkdir: 'Create directory', file_perm: 'File permissions',
  file_link: 'File link', file_truncate: 'File truncate', forward_open: 'Start port forward',
  forward_close: 'Close port forward',
};

export function pendingApprovals() {
  return state.approvals.filter(item => item.request?.state === 'pending');
}

function requestCard(item) {
  const request = item.request;
  const session = state.data.sessions.find(value => value.id === request.session_id);
  const kind = request.kind || 'shell_input';
  const summary = request.summary || request.text || kind;
  const time = request.created_at ? new Date(request.created_at).toLocaleString() : '';
  const command = kind === 'shell_input' && request.text ? `<pre>${esc(request.text)}</pre>` : '';
  const keys = request.keys?.length ? `<small>Keys: ${esc(request.keys.join(', '))}</small>` : '';
  return `<article class="approval-card"><header><div><span>${esc(labels[kind] || kind)}</span><h3>${esc(session?.name || request.session_id)}</h3></div><small>${esc(time)}</small></header><p>${esc(summary)}</p>${command}${keys}<footer><button class="button" data-approval-decision="reject" data-approval-id="${esc(request.id)}">Reject</button><button class="button primary" data-approval-decision="approve" data-approval-id="${esc(request.id)}">Approve</button></footer></article>`;
}

export function approvalPage() {
  const pending = pendingApprovals();
  return `${header('Approvals', 'AI ACTION REVIEW', button(`${icon('refresh', 14)}Refresh`, 'refresh-approvals'))}<div class="approval-page"><section class="panel approval-intro"><b>${pending.length} pending request${pending.length === 1 ? '' : 's'}</b><p>Review AI requests to run Shell commands, change files, or start port forwards. Your own terminal and file actions remain available.</p></section><div class="approval-cards">${pending.map(requestCard).join('') || emptyState('No actions are waiting for approval.')}</div></div>`;
}
