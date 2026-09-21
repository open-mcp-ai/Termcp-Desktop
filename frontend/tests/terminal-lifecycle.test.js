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
  }

  open(element) {
    this.node = new FakeNode();
    element.appendChild(this.node);
  }

  loadAddon() {}
  onData(callback) { this.onDataCallback = callback; }
  write(data) { this.output.push(data); }
  dispose() { this.disposed = true; }
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

const { TerminalController } = await import('../src/terminal.js');

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
