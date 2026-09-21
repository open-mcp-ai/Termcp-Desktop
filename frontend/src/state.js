import { core } from './core.js';
import { getAppearance } from './appearance.js';
import { loadWorkspaceState, reconcileWorkspaceState, saveCollapsedGroups as persistCollapsedGroups, saveWorkspaceState } from './features/workspace/model.js';

const persistedWorkspace = loadWorkspaceState();

export const state = {
  data: { core: { running: false, managed: false, address: 'http://127.0.0.1:18765', state: 'starting' }, connections: [], sessions: [], history: [], forwards: [] },
  loading: true,
  error: '',
  section: 'resources',
  filter: '',
  selected: { type: 'connection', id: '' },
  expanded: new Set(['connection:internal']),
  dialog: null,
  appearance: getAppearance(),
  contextMenu: null,
  terminalMenu: null,
  draggedWorkspace: '',
  draggedSession: '',
  draggedShell: '',
  dropRegion: '',
  workspaces: persistedWorkspace.workspaces,
  closedSessionTabs: persistedWorkspace.closedSessionTabs,
  collapsedGroups: persistedWorkspace.collapsedGroups,
  activeWorkspace: persistedWorkspace.activeWorkspace,
  inspector: { tab: 'files', path: '/', data: null, loading: false, error: '', sessionID: '', collapsed: localStorage.getItem('termcp-desktop-inspector-collapsed') === '1' },
  historyQuery: '',
  historyTranscript: '',
  wsStatus: core.preview ? 'preview' : 'connecting',
  service: { supported: true, platform: '', installed: false, running: false, autostart: false, pid: 0, label: '', definition: '', log_path: '', executable: '', description: '' },
  systemFonts: { items: ['system-ui'], loading: false, loaded: false, error: '' },
};

export function saveWorkspaces() { saveWorkspaceState(state); }
export function saveCollapsedGroups() { persistCollapsedGroups(state); }
export function coreMode() { return state.service.installed ? 'System service' : 'In-app process'; }

export function shellCount() {
  return state.data.sessions.reduce((count, session) => count + (session.shells || []).filter(shell => shell.status === 'running').length, 0);
}

export function sessionByID(id) { return state.data.sessions.find(session => session.id === id); }

export function shellByID(id) {
  for (const session of state.data.sessions) {
    const shell = (session.shells || []).find(item => item.id === id);
    if (shell) return { ...shell, session };
  }
  return null;
}

export function connectionByName(name) { return state.data.connections.find(item => item.name === name); }
export function historyByID(id) { return state.data.history.find(item => item.id === id); }

export function activeWorkspace() {
  let workspace = state.workspaces.find(item => item.id === state.activeWorkspace);
  if (!workspace) {
    workspace = state.workspaces[0] || { id: `session-tab-empty`, name: 'New session', panes: [], maximized: '' };
    if (!state.workspaces.length) state.workspaces.push(workspace);
    state.activeWorkspace = workspace.id;
  }
  return workspace;
}

export function workspaceSession(workspace) {
  if (!workspace) return null;
  return sessionByID(workspace.primarySessionID || workspace.panes?.[0]?.sessionID);
}

export function activePane() {
  const workspace = activeWorkspace();
  const shellID = workspace.activeShell || workspace.panes[0]?.shellID;
  return workspace.panes.find(pane => pane.shellID === shellID) || workspace.panes[0];
}

export function reconcileWorkspaces() {
  reconcileWorkspaceState(state, { sessionByID, shellByID, workspaceSession });
}
