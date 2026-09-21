const WORKSPACES_KEY = 'termcp-desktop-workspaces';
const ACTIVE_WORKSPACE_KEY = 'termcp-desktop-active-workspace';
const CLOSED_TABS_KEY = 'termcp-desktop-closed-session-tabs';
const COLLAPSED_GROUPS_KEY = 'termcp-desktop-collapsed-groups';

function readArray(key) {
  try {
    const value = JSON.parse(localStorage.getItem(key) || '[]');
    return Array.isArray(value) ? value : [];
  } catch {
    return [];
  }
}

export function loadWorkspaceState() {
  return {
    workspaces: readArray(WORKSPACES_KEY),
    activeWorkspace: localStorage.getItem(ACTIVE_WORKSPACE_KEY) || 'workspace-main',
    closedSessionTabs: new Set(readArray(CLOSED_TABS_KEY)),
    collapsedGroups: new Set(readArray(COLLAPSED_GROUPS_KEY)),
  };
}

export function saveWorkspaceState(state) {
  localStorage.setItem(WORKSPACES_KEY, JSON.stringify(state.workspaces));
  localStorage.setItem(ACTIVE_WORKSPACE_KEY, state.activeWorkspace);
  localStorage.setItem(CLOSED_TABS_KEY, JSON.stringify([...state.closedSessionTabs]));
}

export function saveCollapsedGroups(state) {
  localStorage.setItem(COLLAPSED_GROUPS_KEY, JSON.stringify([...state.collapsedGroups]));
}

function uniqueTabs(tabs) {
  const seen = new Set();
  return tabs.filter(tab => tab?.shellID && !seen.has(tab.shellID) && seen.add(tab.shellID));
}

function tabsForSessions(sessionIDs, sessionByID) {
  return sessionIDs.flatMap(sessionID => {
    const session = sessionByID(sessionID);
    return (session?.shells || []).map(shell => ({ shellID: shell.id, sessionID }));
  });
}

export function reconcileWorkspaceState(state, { sessionByID, shellByID, workspaceSession }) {
  for (const workspace of state.workspaces) {
    workspace.panes = uniqueTabs((workspace.panes || []).filter(pane => shellByID(pane.shellID)));
    workspace.primarySessionID ||= workspace.panes[0]?.sessionID || workspace.shellTabs?.[0]?.sessionID || '';
    if (workspace.primarySessionID && !sessionByID(workspace.primarySessionID)) workspace.primarySessionID = workspace.panes[0]?.sessionID || '';
    workspace.mergedSessionIDs = (workspace.mergedSessionIDs || []).filter(id => id !== workspace.primarySessionID && sessionByID(id));
    const sessionIDs = [workspace.primarySessionID, ...workspace.mergedSessionIDs].filter(Boolean);
    const discoveredTabs = tabsForSessions(sessionIDs, sessionByID);
    workspace.shellTabs = uniqueTabs([...(workspace.shellTabs || workspace.panes), ...discoveredTabs].filter(tab => shellByID(tab.shellID)));
    workspace.panes = workspace.panes.filter(pane => workspace.shellTabs.some(tab => tab.shellID === pane.shellID));
    if (!workspace.panes.length && workspace.shellTabs.length) workspace.panes = [{ ...workspace.shellTabs[0] }];
    const session = sessionByID(workspace.primarySessionID);
    if (session) workspace.name = session.name;
    if (!workspace.panes.some(pane => pane.shellID === workspace.activeShell)) workspace.activeShell = workspace.panes[0]?.shellID || '';
    if (workspace.maximized && !workspace.panes.some(pane => pane.shellID === workspace.maximized)) workspace.maximized = '';
  }

  if (state.data.sessions.length) state.workspaces = state.workspaces.filter(workspace => workspaceSession(workspace));
  else state.workspaces = state.workspaces.filter(workspace => !workspace.primarySessionID);

  const covered = new Set(state.workspaces.flatMap(workspace => [workspace.primarySessionID, ...(workspace.mergedSessionIDs || [])]).filter(Boolean));
  for (const session of state.data.sessions) {
    const shells = session.shells || [];
    const shell = shells.find(item => item.status === 'running') || shells[0];
    if (!shell || covered.has(session.id) || state.closedSessionTabs.has(session.id)) continue;
    state.workspaces.push({
      id: `session-tab-${session.id}`,
      name: session.name,
      primarySessionID: session.id,
      shellTabs: shells.map(item => ({ shellID: item.id, sessionID: session.id })),
      panes: [{ shellID: shell.id, sessionID: session.id }],
      activeShell: shell.id,
      maximized: '',
      layout: 'grid',
    });
  }
  if (!state.workspaces.some(workspace => workspace.id === state.activeWorkspace)) state.activeWorkspace = state.workspaces[0]?.id || '';
  saveWorkspaceState(state);
}

export function selectShellTab(workspace, shellID, sessionID) {
  workspace.shellTabs ||= [];
  if (!workspace.shellTabs.some(tab => tab.shellID === shellID)) workspace.shellTabs.push({ shellID, sessionID });
  const visible = workspace.panes.find(pane => pane.shellID === shellID);
  if (!visible) {
    const activeIndex = Math.max(0, workspace.panes.findIndex(pane => pane.shellID === workspace.activeShell));
    if (workspace.panes.length) workspace.panes.splice(activeIndex, 1, { shellID, sessionID });
    else workspace.panes.push({ shellID, sessionID });
  }
  workspace.activeShell = shellID;
  workspace.maximized = '';
}

export function removeShellFromWorkspace(workspace, shellID) {
  workspace.shellTabs = (workspace.shellTabs || []).filter(tab => tab.shellID !== shellID);
  workspace.panes = (workspace.panes || []).filter(pane => pane.shellID !== shellID);
  if (!workspace.panes.length && workspace.shellTabs.length) workspace.panes = [{ ...workspace.shellTabs[0] }];
  if (!workspace.panes.some(pane => pane.shellID === workspace.activeShell)) workspace.activeShell = workspace.panes[0]?.shellID || '';
  if (workspace.maximized === shellID) workspace.maximized = '';
}
