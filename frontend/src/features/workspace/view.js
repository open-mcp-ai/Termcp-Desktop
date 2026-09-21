import { dot, esc, icon } from '../../ui/render.js';

export function renderWorkspacePage({ state, activeWorkspace, workspaceSession, shellByID, inspectorShell }) {
  const workspace = activeWorkspace();
  workspace.layout ||= 'grid';
  workspace.shellTabs ||= workspace.panes || [];
  const panes = workspace.maximized ? workspace.panes.filter(pane => pane.shellID === workspace.maximized) : workspace.panes;
  const tabs = state.workspaces.map(item => {
    const session = workspaceSession(item);
    return `<button class="workspace-tab ${item.id === workspace.id ? 'active' : ''}" data-workspace="${esc(item.id)}" data-workspace-drag="${esc(item.id)}" data-session-drag="${esc(session?.id || '')}" draggable="true" title="拖动会话标签排序">${dot(session?.status || 'archived')}<span>${esc(session?.name || item.name || '新会话')}</span><small>${esc(session?.ssh_endpoint || 'local')}</small>${state.workspaces.length > 1 ? `<i data-close-workspace="${esc(item.id)}" title="关闭标签">×</i>` : ''}</button>`;
  }).join('');
  const dropClass = state.dropRegion ? `drop-${state.dropRegion}` : '';
  const terminal = panes.length
    ? `<div class="terminal-workspace layout-${esc(workspace.layout)} ${workspace.maximized ? 'maximized' : ''} ${dropClass}" data-pane-count="${panes.length}" data-session-drop-zone>${panes.map(pane => renderTerminalPane(pane, workspace, shellByID)).join('')}<div class="session-drop-hint"><span>${state.dropRegion === 'top' ? '在上方分屏' : state.dropRegion === 'bottom' ? '在下方分屏' : state.dropRegion === 'left' ? '在左侧分屏' : '在右侧分屏'}</span></div></div>`
    : `<div class="workspace-empty ${dropClass}" data-session-drop-zone><span>›_</span><h2>还没有打开的会话</h2><p>前往“连接”栏目，双击连接配置即可创建并打开新会话。</p><button class="button primary" data-section="resources">打开连接配置</button><div class="session-drop-hint"><span>松开以打开会话</span></div></div>`;
  const shellTabs = renderShellTabs(workspace, workspaceSession(workspace), shellByID);
  return `<div class="workspace-page ${state.inspector.collapsed ? 'inspector-collapsed' : ''}"><div class="workspace-tabs"><div class="workspace-tab-scroll">${tabs}</div><button class="new-workspace-tab" data-action="new-session" title="新建 SSH 会话" aria-label="新建 SSH 会话">${icon('plus', 16)}</button></div>${terminal}${shellTabs}<aside class="workspace-inspector ${state.inspector.collapsed ? 'collapsed' : ''}">${inspectorShell()}</aside></div>`;
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
    return `<div class="shell-tab ${active ? 'active' : ''}" data-shell-context="${esc(tab.shellID)}" data-shell-drag="${esc(tab.shellID)}" data-session-drag="${esc(tab.sessionID)}" draggable="true"><button data-shell-tab="${esc(tab.shellID)}" data-session="${esc(tab.sessionID)}" title="${esc(item.session.name)} / ${esc(item.name || 'shell')}">${dot(item.status)}<span>${esc(item.name || 'shell')}</span><small>${esc(item.session.name)}</small></button><button class="shell-tab-close" data-close-shell-tab="${esc(tab.shellID)}" title="关闭 Shell" aria-label="关闭 Shell">×</button></div>`;
  }).join('');
  return `<div class="shell-tabbar" aria-label="Shell 标签"><div class="shell-tab-scroll">${tabs || '<span class="shell-tabs-empty">没有 Shell</span>'}</div>${session ? `<button class="new-shell-tab" data-action="new-shell" data-id="${esc(session.id)}" title="新建 Shell">${icon('plus', 15)}</button>` : ''}<small class="shell-tab-hint">拖动 Shell 标签到终端区域可分屏</small></div>`;
}

export function renderSessionContextMenu({ state, sessionByID }) {
  const menu = state.contextMenu;
  const session = menu && sessionByID(menu.sessionID);
  if (!session) return '';
  const firstShell = (session.shells || []).find(shell => shell.status === 'running') || (session.shells || [])[0];
  return `<div class="context-menu session-context-menu" role="menu" style="left:${menu.x}px;top:${menu.y}px" data-session-menu="${esc(session.id)}"><header>${dot(session.status)}<span><b>${esc(session.name)}</b><small>${esc(session.ssh_endpoint || session.mode)}</small></span></header><button role="menuitem" data-context-action="open" ${firstShell ? `data-shell="${esc(firstShell.id)}"` : ''}>${icon('external', 15)}<span>${firstShell ? '在工作台打开' : '新建 Shell'}</span></button><button role="menuitem" data-context-action="new-shell">${icon('plus', 15)}<span>新建 Shell</span></button><button role="menuitem" data-context-action="details">${icon('sessions', 15)}<span>查看会话详情</span></button><button role="menuitem" data-context-action="rename">${icon('edit', 15)}<span>重命名</span></button><button role="menuitem" data-context-action="copy-uri">${icon('copy', 15)}<span>复制资源地址</span><kbd>termcp://</kbd></button><hr><button role="menuitem" data-context-action="terminate" class="warning" ${session.status === 'running' ? '' : 'disabled'}>${icon('trash', 15)}<span>结束并归档</span></button><button role="menuitem" data-context-action="purge" class="danger">${icon('trash', 15)}<span>永久删除</span></button></div>`;
}

export function renderTerminalContextMenu({ state, shellByID, activeWorkspace }) {
  const menu = state.terminalMenu;
  const item = menu && shellByID(menu.shellID);
  if (!item) return '';
  const paneCount = activeWorkspace().panes.length;
  const shortcut = navigator.platform?.toLowerCase().includes('mac') ? '⌘' : 'Ctrl+';
  return `<div class="context-menu terminal-context-menu" role="menu" style="left:${menu.x}px;top:${menu.y}px" data-terminal-menu="${esc(item.id)}"><header>${dot(item.status)}<span><b>${esc(item.name || 'shell')}</b><small>${esc(item.session.name)} · ${esc(item.status)}</small></span></header><button role="menuitem" data-terminal-action="copy" ${menu.selection ? '' : 'disabled'}>${icon('copy', 15)}<span>复制选中文本</span><kbd>${shortcut}C</kbd></button><button role="menuitem" data-terminal-action="paste">${icon('paste', 15)}<span>粘贴</span><kbd>${shortcut}V</kbd></button><button role="menuitem" data-terminal-action="select-all">${icon('select', 15)}<span>全选</span><kbd>${shortcut}A</kbd></button><button role="menuitem" data-terminal-action="clear">${icon('clear', 15)}<span>清空终端</span><kbd>${shortcut}K</kbd></button><hr><button role="menuitem" data-terminal-action="new-shell">${icon('plus', 15)}<span>新建 Shell</span><kbd>⇧${shortcut}T</kbd></button><button role="menuitem" data-terminal-action="remove-pane" ${paneCount > 1 ? '' : 'disabled'}>${icon('split', 15)}<span>移除此分屏</span></button><button role="menuitem" data-terminal-action="close" class="danger">${icon('trash', 15)}<span>关闭 Shell</span></button></div>`;
}
