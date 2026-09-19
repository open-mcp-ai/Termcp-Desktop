const textEncoder = new TextEncoder();

function bytesToBase64(bytes) {
  let binary = '';
  for (let i = 0; i < bytes.length; i += 1) binary += String.fromCharCode(bytes[i]);
  return btoa(binary);
}

function base64ToBytes(value) {
  const binary = atob(value || '');
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i += 1) bytes[i] = binary.charCodeAt(i);
  return bytes;
}

export class TerminalController {
  constructor(core, hooks = {}) {
    this.core = core;
    this.hooks = hooks;
    this.ws = null;
    this.retry = null;
    this.backoff = 800;
    this.instances = new Map();
    this.wanted = new Set();
    this.disposed = false;
  }

  async connect() {
    if (this.core.preview || this.disposed) return;
    const url = await this.core.websocketURL();
    if (!url || this.disposed) return;
    if (this.ws) { try { this.ws.close(); } catch {} }
    const ws = new WebSocket(url);
    this.ws = ws;
    this.hooks.status?.('connecting');
    ws.onopen = () => {
      if (this.ws !== ws) return;
      this.backoff = 800;
      this.hooks.status?.('connected');
      for (const id of this.wanted) this.send({ type: 'watch_add', id });
    };
    ws.onmessage = event => {
      let message;
      try { message = JSON.parse(event.data); } catch { return; }
      if (message.type === 'terminal') this.instances.get(message.id)?.term.write(base64ToBytes(message.d));
      if (message.type === 'terminal_done') {
        const instance = this.instances.get(message.id);
        if (instance && !instance.ended) {
          instance.ended = true;
          instance.term.write('\r\n\x1b[33m[Shell 已结束]\x1b[0m\r\n');
        }
      }
      if (message.type === 'sessions') this.hooks.sessions?.(message);
      if (message.type === 'ui_notify') this.hooks.notify?.(message);
    };
    ws.onerror = () => {};
    ws.onclose = () => {
      if (this.ws !== ws || this.disposed) return;
      this.ws = null;
      this.hooks.status?.('disconnected');
      clearTimeout(this.retry);
      this.retry = setTimeout(() => this.connect(), this.backoff);
      this.backoff = Math.min(10000, Math.round(this.backoff * 1.8));
    };
  }

  send(message) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(message));
  }

  clear() {
    for (const [id, instance] of this.instances) {
      this.send({ type: 'watch_remove', id });
      instance.resize?.disconnect();
      try { instance.term.dispose(); } catch {}
    }
    this.instances.clear();
    this.wanted.clear();
  }

  async mount(shellID, element, readOnly = false) {
    if (!element || this.instances.has(shellID) || !window.Terminal) return;
    const term = new window.Terminal({
      cursorBlink: !readOnly,
      disableStdin: readOnly,
      convertEol: false,
      scrollback: 12000,
      fontSize: 13,
      lineHeight: 1.22,
      fontFamily: 'SFMono-Regular, Menlo, Monaco, Consolas, monospace',
      theme: { background: '#111814', foreground: '#c8d5cc', cursor: '#7fc893', selectionBackground: '#47705888', black: '#17201b', green: '#71b985', brightGreen: '#92d5a4' },
      allowProposedApi: false,
    });
    let fit = null;
    if (window.FitAddon?.FitAddon) {
      fit = new window.FitAddon.FitAddon();
      term.loadAddon(fit);
    }
    term.open(element);
    try { fit?.fit(); } catch {}
    const instance = { term, fit, ended: readOnly, userReady: false, resize: null };
    this.instances.set(shellID, instance);
    element.addEventListener('mousedown', () => { instance.userReady = true; }, { capture: true });
    element.addEventListener('keydown', () => { instance.userReady = true; }, { capture: true });
    element.addEventListener('paste', () => { instance.userReady = true; }, { capture: true });
    term.onData(data => {
      if (readOnly || instance.ended || !instance.userReady) return;
      this.send({ type: 'input', id: shellID, d: bytesToBase64(textEncoder.encode(data)), nl: false });
    });
    let resizeTimer;
    const resize = new ResizeObserver(() => {
      clearTimeout(resizeTimer);
      resizeTimer = setTimeout(() => {
        try { fit?.fit(); } catch {}
        if (!readOnly && !instance.ended) this.send({ type: 'resize', id: shellID, rows: term.rows, cols: term.cols });
      }, 100);
    });
    resize.observe(element);
    instance.resize = resize;
    try {
      const result = await this.core.api('GET', `/api/shells/${encodeURIComponent(shellID)}/output-range?tail=1&max=524288`);
      if (result.data?.d) term.write(base64ToBytes(result.data.d));
    } catch (error) {
      term.write(`\r\n\x1b[31m[读取输出失败: ${String(error)}]\x1b[0m\r\n`);
    }
    if (readOnly) term.write('\r\n\x1b[33m[只读历史]\x1b[0m\r\n');
    else {
      this.wanted.add(shellID);
      this.send({ type: 'watch_add', id: shellID });
      this.send({ type: 'resize', id: shellID, rows: term.rows, cols: term.cols });
    }
  }

  dispose() {
    this.disposed = true;
    clearTimeout(this.retry);
    this.clear();
    try { this.ws?.close(); } catch {}
  }
}
