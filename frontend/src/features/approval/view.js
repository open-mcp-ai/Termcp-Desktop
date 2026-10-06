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

export function reviewedApprovals() {
  return state.approvals.filter(item => item.request && item.request.state !== 'pending');
}

function requestTitle(item) {
  const request = item.request;
  const session = state.data.sessions.find(value => value.id === request.session_id);
  return esc(session?.name || request.session_id || labels[request.kind] || 'AI action');
}

function decisionButtons(request) {
  return `<button class="button" data-approval-decision="reject" data-approval-id="${esc(request.id)}">Reject</button><button class="button primary" data-approval-decision="approve" data-approval-id="${esc(request.id)}">Approve</button>`;
}

function requestCard(item) {
  const request = item.request;
  const kind = request.kind || 'shell_input';
  const summary = request.summary || request.text || kind;
  const time = request.created_at ? new Date(request.created_at).toLocaleString() : '';
  const command = kind === 'shell_input' && request.text ? `<pre>${esc(request.text)}</pre>` : '';
  const keys = request.keys?.length ? `<small>Keys: ${esc(request.keys.join(', '))}</small>` : '';
  return `<article class="approval-card"><header><div><span>${esc(labels[kind] || kind)}</span><h3>${requestTitle(item)}</h3></div><small>${esc(time)}</small></header><p>${esc(summary)}</p>${command}${keys}<footer>${request.state === 'pending' ? decisionButtons(request) : `<span class="approval-state">${esc(request.state)}</span>`}</footer></article>`;
}

function compactRequest(item) {
  const request = item.request;
  return `<article class="approval-menu-item"><header><b>${requestTitle(item)}</b><small>${esc(labels[request.kind] || request.kind || 'AI action')}</small></header><p>${esc(request.summary || request.text || request.kind || '')}</p>${request.state === 'pending' ? `<footer>${decisionButtons(request)}</footer>` : `<span class="approval-state">${esc(request.state)}</span>`}</article>`;
}

export function approvalMenu() {
  if (!state.approvalMenuOpen) return '';
  const pending = pendingApprovals();
  const reviewed = reviewedApprovals();
  return `<section class="approval-menu" role="dialog" aria-label="Approval notifications"><header><b>Approval notifications</b><span>${pending.length} pending</span></header><div class="approval-menu-scroll"><h3>Pending approvals</h3>${pending.map(compactRequest).join('') || '<p class="approval-menu-empty">No actions are waiting for approval.</p>'}<details class="approval-reviewed"><summary>Reviewed approvals <span>${reviewed.length}</span></summary>${reviewed.map(compactRequest).join('') || '<p class="approval-menu-empty">No reviewed approvals.</p>'}</details></div><footer><button data-section="approvals">Open approval panel</button></footer></section>`;
}

export function approvalPrompt() {
  const item = pendingApprovals().find(value => value.request?.id === state.approvalPromptID);
  if (!item) return '';
  return `<aside class="approval-prompt" role="alertdialog" aria-label="Approval requested"><div><span>${icon('bell', 17)}</span><button data-dismiss-approval-prompt aria-label="Dismiss notification" title="Dismiss notification">×</button></div><b>Approval requested</b><p>${esc(item.request.summary || item.request.text || item.request.kind || 'AI action')}</p><small>${requestTitle(item)}</small><footer>${decisionButtons(item.request)}</footer></aside>`;
}

export function approvalPage() {
  const pending = pendingApprovals();
  const reviewed = reviewedApprovals();
  return `${header('Approvals', 'AI ACTION REVIEW', button(`${icon('refresh', 14)}Refresh`, 'refresh-approvals'))}<div class="approval-page"><section class="panel approval-intro"><b>${pending.length} pending request${pending.length === 1 ? '' : 's'}</b><p>Review AI requests to run Shell commands, change files, or start port forwards. Your own terminal and file actions remain available.</p></section><div class="approval-cards">${pending.map(requestCard).join('') || emptyState('No actions are waiting for approval.')}</div><details class="approval-page-reviewed"><summary>Reviewed approvals <span>${reviewed.length}</span></summary><div class="approval-cards">${reviewed.map(requestCard).join('') || emptyState('No reviewed approvals.')}</div></details></div>`;
}
