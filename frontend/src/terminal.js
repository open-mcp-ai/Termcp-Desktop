import { t } from './i18n/index.js';
import { terminalAppearance } from './appearance.js';

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
    let url;
    try {
      url = await this.core.websocketURL();
    } catch (error) {
      this.hooks.log?.('error', 'Terminal WebSocket URL failed', String(error));
      return;
    }
    if (!url || this.disposed) return;
    if (this.ws) { try { this.ws.close(); } catch {} }
    const ws = new WebSocket(url);
    this.ws = ws;
    this.hooks.status?.('connecting');
    ws.onopen = () => {
      if (this.ws !== ws) return;
      this.backoff = 800;
      this.hooks.status?.('connected');
      this.hooks.log?.('debug', 'Terminal WebSocket connected', 'Core event channel is ready');
      for (const id of this.wanted) this.send({ type: 'watch_add', id });
    };
    ws.onmessage = event => {
      let message;
      try { message = JSON.parse(event.data); } catch (error) {
        this.hooks.log?.('warn', 'Invalid terminal WebSocket message', String(error));
        return;
      }
      if (message.type === 'terminal') this.instances.get(message.id)?.term.write(base64ToBytes(message.d));
      if (message.type === 'terminal_done') {
        const instance = this.instances.get(message.id);
        if (instance && !instance.ended) {
          instance.ended = true;
          instance.term.write(`\r\n\x1b[33m[${t('Shell ended')}]\x1b[0m\r\n`);
        }
      }
      if (message.type === 'sessions') this.hooks.sessions?.(message);
      if (message.type === 'ui_notify') this.hooks.notify?.(message);
    };
    ws.onerror = () => { this.hooks.log?.('warn', 'Terminal WebSocket error', 'The event channel reported an error'); };
    ws.onclose = event => {
      if (this.ws !== ws || this.disposed) return;
      this.ws = null;
      this.hooks.status?.('disconnected');
      this.hooks.log?.('warn', 'Terminal WebSocket disconnected', `code=${event.code}`);
      clearTimeout(this.retry);
      this.retry = setTimeout(() => this.connect(), this.backoff);
      this.backoff = Math.min(10000, Math.round(this.backoff * 1.8));
    };
  }

  send(message) {
    if (this.ws?.readyState === WebSocket.OPEN) this.ws.send(JSON.stringify(message));
  }

  clear() {
    for (const id of [...this.instances.keys()]) this.remove(id);
    this.wanted.clear();
  }

  remove(shellID) {
    const instance = this.instances.get(shellID);
    if (!instance) return;
    this.send({ type: 'watch_remove', id: shellID });
    instance.resize?.disconnect();
    try { instance.term.dispose(); } catch {}
    this.instances.delete(shellID);
    this.wanted.delete(shellID);
  }

  retain(shellIDs) {
    for (const id of this.instances.keys()) if (!shellIDs.has(id)) this.remove(id);
  }

  // Keep xterm's DOM, scrollback, selection, and WebSocket watch alive while
  // the surrounding application shell is synchronously re-rendered.
  detachAll() {
    for (const instance of this.instances.values()) {
      if (!instance.element) continue;
      instance.resize?.disconnect();
      const fragment = document.createDocumentFragment();
      while (instance.element.firstChild) fragment.appendChild(instance.element.firstChild);
      instance.fragment = fragment;
      instance.element = null;
    }
  }

  refreshAppearance() {
    const appearance = terminalAppearance();
    for (const instance of this.instances.values()) {
      instance.term.options.fontSize = appearance.fontSize;
      instance.term.options.fontFamily = appearance.fontFamily;
      instance.term.options.theme = appearance.theme;
      try { instance.fit?.fit(); } catch {}
    }
  }

  selection(shellID) {
    return this.instances.get(shellID)?.term.getSelection() || '';
  }

  async copy(shellID, fallback = '') {
    const text = this.selection(shellID) || fallback;
    if (!text) return false;
    await navigator.clipboard.writeText(text);
    return true;
  }

  async paste(shellID) {
    const instance = this.instances.get(shellID);
    if (!instance) return false;
    const text = await navigator.clipboard.readText();
    if (!text) return false;
    instance.userReady = true;
    instance.term.focus();
    instance.term.paste(text);
    return true;
  }

  selectAll(shellID) {
    const term = this.instances.get(shellID)?.term;
    if (!term) return false;
    term.selectAll();
    term.focus();
    return true;
  }

  clearShell(shellID) {
    const term = this.instances.get(shellID)?.term;
    if (!term) return false;
    term.clear();
    term.focus();
    return true;
  }

  async mount(shellID, element, readOnly = false) {
    if (!element) return;
    const existing = this.instances.get(shellID);
    if (existing) {
      this.attach(existing, element);
      return;
    }
    if (!window.Terminal) return;
    const appearance = terminalAppearance();
    const term = new window.Terminal({
      cursorBlink: !readOnly,
      disableStdin: readOnly,
      convertEol: false,
      scrollback: 12000,
      fontSize: appearance.fontSize,
      lineHeight: 1.22,
      fontFamily: appearance.fontFamily,
      theme: appearance.theme,
      allowProposedApi: false,
    });
    let fit = null;
    if (window.FitAddon?.FitAddon) {
      fit = new window.FitAddon.FitAddon();
      term.loadAddon(fit);
    }
    term.open(element);
    try { fit?.fit(); } catch {}
    const instance = { term, fit, ended: readOnly, userReady: false, resize: null, element, fragment: null };
    this.instances.set(shellID, instance);
    this.bindActivity(instance, element);
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
    instance.resize = resize;
    resize.observe(element);
    try {
      const result = await this.core.api('GET', `/api/shells/${encodeURIComponent(shellID)}/output-range?tail=1&max=524288`);
      if (result.data?.d) term.write(base64ToBytes(result.data.d));
    } catch (error) {
      this.hooks.log?.('error', 'Failed to restore terminal output', `shell=${shellID}: ${String(error)}`);
      term.write(`\r\n\x1b[31m[${t('Failed to read output')}: ${String(error)}]\x1b[0m\r\n`);
    }
    if (readOnly) term.write(`\r\n\x1b[33m[${t('Read-only history')}]\x1b[0m\r\n`);
    else {
      this.wanted.add(shellID);
      this.send({ type: 'watch_add', id: shellID });
      this.send({ type: 'resize', id: shellID, rows: term.rows, cols: term.cols });
    }
  }

  attach(instance, element) {
    if (instance.element === element) return;
    instance.resize?.disconnect();
    if (instance.fragment) {
      element.appendChild(instance.fragment);
      instance.fragment = null;
    } else if (instance.element) {
      while (instance.element.firstChild) element.appendChild(instance.element.firstChild);
    }
    instance.element = element;
    this.bindActivity(instance, element);
    instance.resize?.observe(element);
    try { instance.fit?.fit(); } catch {}
  }

  bindActivity(instance, element) {
    const ready = () => { instance.userReady = true; };
    element.addEventListener('mousedown', ready, { capture: true });
    element.addEventListener('keydown', ready, { capture: true });
    element.addEventListener('paste', ready, { capture: true });
  }

  dispose() {
    this.disposed = true;
    clearTimeout(this.retry);
    this.clear();
    try { this.ws?.close(); } catch {}
  }
}
