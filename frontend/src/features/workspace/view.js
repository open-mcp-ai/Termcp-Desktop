import { dot, esc, icon } from '../../ui/render.js';

export function renderWorkspacePage({ state, activeWorkspace, workspaceSession, shellByID, inspectorShell }) {
  const workspace = activeWorkspace();
  workspace.layout ||= 'grid';
  workspace.shellTabs ||= workspace.panes || [];
  const panes = workspace.maximized ? workspace.panes.filter(pane => pane.shellID === workspace.maximized) : workspace.panes;
  const tabs = state.workspaces.map(item => {
    const session = workspaceSession(item);
    return `<button class="workspace-tab ${item.id === workspace.id ? 'active' : ''}" data-workspace="${esc(item.id)}" data-workspace-drag="${esc(item.id)}" data-session-drag="${esc(session?.id || '')}" draggable="true" title="Drag session tabs to reorder">${dot(session?.status || 'archived')}<span>${esc(session?.name || item.name || 'New session')}</span><small>${esc(session?.ssh_endpoint || 'local')}</small>${state.workspaces.length > 1 ? `<i data-close-workspace="${esc(item.id)}" title="Close tab">×</i>` : ''}</button>`;
  }).join('');
  const dropClass = state.dropRegion ? `drop-${state.dropRegion}` : '';
  const terminal = panes.length
    ? `<div class="terminal-workspace layout-${esc(workspace.layout)} ${workspace.maximized ? 'maximized' : ''} ${dropClass}" data-pane-count="${panes.length}" data-session-drop-zone>${panes.map(pane => renderTerminalPane(pane, workspace, shellByID)).join('')}<div class="session-drop-hint"><span>${state.dropRegion === 'top' ? 'Split above' : state.dropRegion === 'bottom' ? 'Split below' : state.dropRegion === 'left' ? 'Split on the left' : 'Split on the right'}</span></div></div>`
    : `<div class="workspace-empty ${dropClass}" data-session-drop-zone><span>›_</span><h2>No open sessions</h2><p>Open Connections and double-click a connection to create and open a new session.</p><button class="button primary" data-section="resources">Open connections</button><div class="session-drop-hint"><span>Drop to open session</span></div></div>`;
  const shellTabs = renderShellTabs(workspace, workspaceSession(workspace), shellByID);
  return `<div class="workspace-page ${state.inspector.collapsed ? 'inspector-collapsed' : ''}"><div class="workspace-tabs"><div class="workspace-tab-scroll">${tabs}</div><button class="new-workspace-tab" data-action="new-session" title="New SSH session" aria-label="New SSH session">${icon('plus', 16)}</button></div>${terminal}${shellTabs}<aside class="workspace-inspector ${state.inspector.collapsed ? 'collapsed' : ''}">${inspectorShell()}</aside></div>`;
}

function renderTerminalPane(pane, workspace, shellByID) {
  const item = shellByID(pane.shellID);
  if (!item) return '';
  const active = workspace.activeShell === pane.shellID;
  return `<section class="terminal-pane ${active ? 'active' : ''}" data-pane-shell="${esc(pane.shellID)}" data-pane-activate="${esc(pane.shellID)}"><div class="terminal-host" data-terminal-shell="${esc(pane.shellID)}" data-shell-context="${esc(pane.shellID)}"></div></section>`;
}

function renderShellTabs(workspace, session, shellByID) {
  const tabs = (workspace.shellTabs || []).map(tab => {
    const item = shellByID(tab.shellID);
    if (!item) return '';
    const active = workspace.activeShell === tab.shellID;
    return `<div class="shell-tab ${active ? 'active' : ''}" data-shell-context="${esc(tab.shellID)}" data-shell-drag="${esc(tab.shellID)}" data-session-drag="${esc(tab.sessionID)}" draggable="true"><button data-shell-tab="${esc(tab.shellID)}" data-session="${esc(tab.sessionID)}" title="${esc(item.session.name)} / ${esc(item.name || 'shell')}">${dot(item.status)}<span>${esc(item.name || 'shell')}</span><small>${esc(item.session.name)}</small></button><button class="shell-tab-close" data-close-shell-tab="${esc(tab.shellID)}" title="Close shell" aria-label="Close shell">×</button></div>`;
  }).join('');
  return `<div class="shell-tabbar" aria-label="Shell tabs"><div class="shell-tab-scroll">${tabs || '<span class="shell-tabs-empty">No shells</span>'}</div>${session ? `<button class="new-shell-tab" data-action="new-shell" data-id="${esc(session.id)}" title="New shell">${icon('plus', 15)}</button>` : ''}<small class="shell-tab-hint">Drag a shell tab into the terminal to split</small></div>`;
}

export function renderSessionContextMenu({ state, sessionByID }) {
  const menu = state.contextMenu;
  const session = menu && sessionByID(menu.sessionID);
  if (!session) return '';
  const firstShell = (session.shells || []).find(shell => shell.status === 'running') || (session.shells || [])[0];
  return `<div class="context-menu session-context-menu" role="menu" style="left:${menu.x}px;top:${menu.y}px" data-session-menu="${esc(session.id)}"><header>${dot(session.status)}<span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint || session.mode)}</small></span></header><button role="menuitem" data-context-action="open" ${firstShell ? `data-shell="${esc(firstShell.id)}"` : ''}>${icon('external', 15)}<span>${firstShell ? 'Open in workspace' : 'New shell'}</span></button><button role="menuitem" data-context-action="new-shell">${icon('plus', 15)}<span>New shell</span></button><button role="menuitem" data-context-action="details">${icon('sessions', 15)}<span>View session details</span></button><button role="menuitem" data-context-action="rename">${icon('edit', 15)}<span>Rename</span></button><button role="menuitem" data-context-action="copy-uri">${icon('copy', 15)}<span>Copy resource URI</span><kbd>termcp://</kbd></button><hr><button role="menuitem" data-context-action="terminate" class="warning" ${session.status === 'running' ? '' : 'disabled'}>${icon('trash', 15)}<span>Terminate and archive</span></button><button role="menuitem" data-context-action="purge" class="danger">${icon('trash', 15)}<span>Delete permanently</span></button></div>`;
}

export function renderTerminalContextMenu({ state, shellByID, activeWorkspace }) {
  const menu = state.terminalMenu;
  const item = menu && shellByID(menu.shellID);
  if (!item) return '';
  const paneCount = activeWorkspace().panes.length;
  const shortcut = navigator.platform?.toLowerCase().includes('mac') ? '⌘' : 'Ctrl+';
  return `<div class="context-menu terminal-context-menu" role="menu" style="left:${menu.x}px;top:${menu.y}px" data-terminal-menu="${esc(item.id)}"><header>${dot(item.status)}<span><b>${esc(item.name || 'shell')}</b><small>${esc(item.session.name)} · ${esc(item.status)}</small></span></header><button role="menuitem" data-terminal-action="copy" ${menu.selection ? '' : 'disabled'}>${icon('copy', 15)}<span>Copy selection</span><kbd>${shortcut}C</kbd></button><button role="menuitem" data-terminal-action="paste">${icon('paste', 15)}<span>Paste</span><kbd>${shortcut}V</kbd></button><button role="menuitem" data-terminal-action="select-all">${icon('select', 15)}<span>Select all</span><kbd>${shortcut}A</kbd></button><button role="menuitem" data-terminal-action="clear">${icon('clear', 15)}<span>Clear terminal</span><kbd>${shortcut}K</kbd></button><hr><button role="menuitem" data-terminal-action="new-shell">${icon('plus', 15)}<span>New shell</span><kbd>⇧${shortcut}T</kbd></button><button role="menuitem" data-terminal-action="remove-pane" ${paneCount > 1 ? '' : 'disabled'}>${icon('split', 15)}<span>Remove from split view</span></button><button role="menuitem" data-terminal-action="close" class="danger">${icon('trash', 15)}<span>Close shell</span></button></div>`;
}
