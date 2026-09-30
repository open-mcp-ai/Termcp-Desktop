import test from 'node:test';
import assert from 'node:assert/strict';

class FakeNode {
  constructor(fragment = false) {
    this.children = [];
    this.parent = null;
    this.fragment = fragment;
    this.listeners = [];
  }

  get firstChild() { return this.children[0] || null; }

  appendChild(node) {
    if (node.fragment) {
      while (node.firstChild) this.appendChild(node.firstChild);
      return node;
    }
    if (node.parent) node.parent.children = node.parent.children.filter(child => child !== node);
    node.parent = this;
    this.children.push(node);
    return node;
  }

  addEventListener(type) { this.listeners.push(type); }
}

class FakeTerminal {
  constructor(options) {
    this.options = options;
    this.rows = 24;
    this.cols = 80;
    this.output = [];
    this.disposed = false;
    this.oscHandlers = new Map();
    this.parser = {
      registerOscHandler: (code, callback) => {
        this.oscHandlers.set(code, callback);
        return { dispose: () => this.oscHandlers.delete(code) };
      },
    };
  }

  open(element) {
    this.node = new FakeNode();
    element.appendChild(this.node);
  }

  loadAddon() {}
  onData(callback) { this.onDataCallback = callback; }
  write(data) { this.output.push(data); }
  dispose() { this.disposed = true; }
  emitOSC(code, data) { return this.oscHandlers.get(code)?.(data); }
}

class FakeFitAddon {
  fit() { this.fitCount = (this.fitCount || 0) + 1; }
}

class FakeResizeObserver {
  constructor(callback) { this.callback = callback; }
  observe(element) { this.element = element; }
  disconnect() { this.element = null; }
}

const storage = new Map();
globalThis.localStorage = {
  getItem(key) { return storage.get(key) || null; },
  setItem(key, value) { storage.set(key, String(value)); },
};
Object.defineProperty(globalThis, 'navigator', { value: { language: 'en-US' }, configurable: true });
globalThis.document = {
  documentElement: {
    dataset: {},
    style: { setProperty() {} },
  },
  createDocumentFragment() { return new FakeNode(true); },
};
globalThis.window = {
  Terminal: FakeTerminal,
  FitAddon: { FitAddon: FakeFitAddon },
};
globalThis.ResizeObserver = FakeResizeObserver;
globalThis.WebSocket = { OPEN: 1 };

const { TerminalController, workingDirectoryFromOSC7 } = await import('../src/terminal.js');
const { renderWorkspacePage } = await import('../src/features/workspace/view.js');

test('renders the Core input source on each Shell tab', () => {
  const session = { id: 'session-1', name: 'Example', status: 'running', ssh_endpoint: 'internal' };
  const shellByID = shellID => ({ id: shellID, name: shellID, status: 'running', session });
  const workspace = {
    id: 'workspace-1', layout: 'grid', panes: [], shellTabs: [
      { shellID: 'human-shell', sessionID: session.id },
      { shellID: 'agent-shell', sessionID: session.id },
    ],
  };
  const html = renderWorkspacePage({
    state: { workspaces: [workspace], inspector: { collapsed: true }, dropRegion: '' },
    activeWorkspace: () => workspace,
    workspaceSession: () => session,
    shellByID,
    inspectorShell: () => '',
    activityForShell: shellID => shellID === 'agent-shell' ? 'ai' : 'human',
  });
  assert.match(html, /data-shell-activity="human-shell" data-kind="human">Human typing…<\/em>/);
  assert.match(html, /data-shell-activity="agent-shell" data-kind="ai">AI typing…<\/em>/);
});

test('keeps the terminal instance and output subscription across workspace renders', async () => {
  let outputRequests = 0;
  const core = {
    preview: true,
    async api() {
      outputRequests += 1;
      return { data: { d: '' } };
    },
  };
  const controller = new TerminalController(core);
  const firstHost = new FakeNode();
  await controller.mount('shell-1', firstHost, false);

  const instance = controller.instances.get('shell-1');
  assert.ok(instance);
  assert.equal(firstHost.children.length, 1);
  assert.equal(outputRequests, 1);
  assert.equal(controller.wanted.has('shell-1'), true);

  controller.detachAll();
  instance.term.write('output received while another session is visible');
  assert.equal(firstHost.children.length, 0);
  assert.equal(instance.element, null);

  const secondHost = new FakeNode();
  await controller.mount('shell-1', secondHost, false);
  assert.equal(controller.instances.get('shell-1'), instance);
  assert.equal(secondHost.children.length, 1);
  assert.equal(outputRequests, 1, 'reattaching must not reload terminal history');
  assert.deepEqual(instance.term.output, ['output received while another session is visible']);

  controller.retain(new Set());
  assert.equal(instance.term.disposed, true);
  assert.equal(controller.instances.size, 0);
});

test('tracks the shell working directory from OSC 7 without writing input', async () => {
  const updates = [];
  const controller = new TerminalController({
    preview: true,
    async api() { return { data: { d: '' } }; },
  }, {
    cwd(shellID, directory) { updates.push([shellID, directory]); },
  });

  await controller.mount('shell-7', new FakeNode(), false);
  const terminal = controller.instances.get('shell-7').term;

  assert.equal(terminal.emitOSC(7, 'file://debian/home/debian/project%20one'), true);
  assert.equal(controller.workingDirectory('shell-7'), '/home/debian/project one');
  assert.deepEqual(updates, [['shell-7', '/home/debian/project one']]);
  assert.equal(workingDirectoryFromOSC7('file://windows/C:/Users/Alice/My%20Project'), 'C:/Users/Alice/My Project');
});

test('streams terminal input chunks as plain JSON text for Core v0.2.4', async () => {
  const sent = [];
  const controller = new TerminalController({ preview: true, async api() { return { data: { d: 'ready\r\n' } }; } });
  controller.ws = { readyState: 1, send(frame) { sent.push(JSON.parse(frame)); } };
  await controller.mount('shell-stream', new FakeNode(), false);
  const terminal = controller.instances.get('shell-stream').term;
  assert.equal(terminal.output[0], 'ready\r\n');
  controller.instances.get('shell-stream').userReady = true;
  terminal.onDataCallback('ech');
  terminal.onDataCallback('o hi');
  terminal.onDataCallback('\r');
  assert.deepEqual(sent.filter(frame => frame.type === 'input').map(frame => frame.d), ['ech', 'o hi', '\r']);
});

test('delivers Core approval notifications to the desktop review hook', async () => {
  const previous = globalThis.WebSocket;
  class FakeWebSocket { static OPEN = 1; close() {} }
  globalThis.WebSocket = FakeWebSocket;
  try {
    const events = [];
    const controller = new TerminalController({ preview: false, async websocketURL() { return 'ws://example.test/api/ui/ws'; } }, { approval(message) { events.push(message); } });
    await controller.connect();
    controller.ws.onmessage({ data: JSON.stringify({ type: 'approval', request: { id: 'request-1', state: 'pending', kind: 'file_write' } }) });
    assert.deepEqual(events.map(event => event.request.id), ['request-1']);
    controller.dispose();
  } finally {
    globalThis.WebSocket = previous;
  }
});

test('routes Core shell_activity frames into per-shell typing indicators', async () => {
  const previous = globalThis.WebSocket;
  class FakeWebSocket { static OPEN = 1; close() {} }
  globalThis.WebSocket = FakeWebSocket;
  try {
    const updates = [];
    const controller = new TerminalController({ preview: false, async websocketURL() { return 'ws://example.test/api/ui/ws'; } }, {
      activity(shellID, status) { updates.push([shellID, status]); },
    });
    await controller.connect();
    const receive = frame => controller.ws.onmessage({ data: JSON.stringify(frame) });
    receive({ type: 'shell_activity', shell_id: 'shell-1', src: 'api', submit: false });
    receive({ type: 'terminal', id: 'shell-1', d: 'echo' });
    receive({ type: 'shell_activity', shell_id: 'shell-2', src: 'ai', submit: false });
    receive({ type: 'shell_activity', shell_id: 'shell-2', src: 'ai', submit: true });
    receive({ type: 'terminal_done', id: 'shell-1' });
    assert.deepEqual(updates, [
      ['shell-1', 'human'],
      ['shell-2', 'ai'],
      ['shell-2', ''],
      ['shell-1', ''],
    ]);
    controller.dispose();
  } finally {
    globalThis.WebSocket = previous;
  }
});
