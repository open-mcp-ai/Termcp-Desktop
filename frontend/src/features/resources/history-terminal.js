import { terminalAppearance } from '../../appearance.js';
import { t } from '../../i18n/index.js';
import { historyRowCells } from './history.js';

const CHUNK_BYTES = 262144;
const labels = { i: 'Human input', a: 'AI input', o: 'Terminal output' };
const kinds = { i: 'input', a: 'agent', o: 'output' };

// JSON strings cannot carry half a UTF-8 character. Re-read a truncated suffix
// with the next byte window instead of committing replacement characters to xterm.
export function historyChunk(body) {
  const start = Number(body.start);
  const end = Number(body.end);
  let text = String(body.d || '');
  let next = end;
  if (text.endsWith('\ufffd') && new TextEncoder().encode(text).length > end - start) {
    const prefix = text.replace(/\ufffd+$/, '');
    const prefixBytes = new TextEncoder().encode(prefix).length;
    const missing = end - start - prefixBytes;
    if (missing > 0 && missing < 4) { text = prefix; next = start + prefixBytes; }
  }
  return { text, next };
}

export class HistoryTerminalController {
  constructor(core, status = () => {}) {
    this.core = core;
    this.status = status;
    this.instance = null;
    this.tintRows = false;
  }

  detach() {
    const instance = this.instance;
    if (!instance?.element) return;
    instance.resize.disconnect();
    instance.fragment = document.createDocumentFragment();
    while (instance.element.firstChild) instance.fragment.appendChild(instance.element.firstChild);
    instance.element = null;
  }

  setTintRows(enabled) {
    this.tintRows = Boolean(enabled);
    if (this.instance) this.paint(this.instance);
  }

  mount(element, shellID, tintRows = false) {
    this.tintRows = tintRows;
    if (!element || !shellID) { this.dispose(); return; }
    let instance = this.instance;
    if (instance?.shellID !== shellID) { this.dispose(); instance = null; }
    if (instance) {
      if (instance.element !== element) {
        element.appendChild(instance.fragment);
        instance.fragment = null;
        instance.element = element;
        instance.resize.observe(element);
      }
      const appearance = terminalAppearance();
      instance.term.options.fontSize = appearance.fontSize;
      instance.term.options.fontFamily = appearance.fontFamily;
      instance.term.options.theme = appearance.theme;
      this.fit(instance);
      this.showStatus(instance);
      this.paint(instance);
      return;
    }
    if (!window.Terminal || !window.FitAddon?.FitAddon) {
      const error = t('Terminal renderer unavailable');
      this.status({ loading: false, error });
      const message = element.closest('.history-page')?.querySelector('.history-status');
      if (message) { message.hidden = false; message.textContent = error; message.classList.add('inspector-error'); }
      return;
    }
    const term = new window.Terminal({
      ...terminalAppearance(), disableStdin: true, cursorBlink: false,
      convertEol: false, scrollback: 100000, lineHeight: 1.22,
    });
    const fit = new window.FitAddon.FitAddon();
    term.loadAddon(fit);
    const host = document.createElement('div');
    host.className = 'history-xterm';
    const rail = document.createElement('div');
    rail.className = 'history-rail';
    rail.setAttribute('aria-label', t('Input and output colours'));
    rail.hidden = true;
    const rowColours = document.createElement('div');
    rowColours.className = 'history-row-colours';
    rowColours.setAttribute('aria-hidden', 'true');
    rowColours.hidden = true;
    element.append(host, rowColours, rail);
    term.open(host);
    instance = { shellID, term, fit, host, rail, rowColours, element, fragment: null,
      next: 0, total: 0, loading: true, error: '', body: null, fetchSequence: 0,
      timer: null, lastFetch: 0, resize: null, subscriptions: [], pendingWrites: new Set() };
    this.instance = instance;
    instance.resize = new ResizeObserver(() => this.fit(instance));
    instance.resize.observe(element);
    instance.subscriptions.push(term.onScroll(() => this.updateRail(instance)));
    instance.subscriptions.push(term.onResize(() => {
      instance.body = null;
      instance.fetchSequence++;
      this.updateRail(instance);
    }));
    const showRow = event => {
      const cell = event.target.closest('.history-rail-cell');
      if (!cell) return;
      const detail = instance.element?.closest('.history-page')?.querySelector('.history-row-detail');
      if (detail) detail.textContent = cell.title;
    };
    rail.addEventListener('mouseover', showRow);
    rail.addEventListener('focusin', showRow);
    rail.addEventListener('click', showRow);
    this.fit(instance);
    this.load();
  }

  fit(instance) {
    if (this.instance !== instance || !instance.element?.isConnected) return;
    try { instance.fit.fit(); } catch { /* A hidden view has no dimensions yet. */ }
    this.updateRail(instance);
  }

  async write(instance, text) {
    if (!text || this.instance !== instance) return;
    await new Promise(resolve => {
      instance.pendingWrites.add(resolve);
      instance.term.write(text, () => { instance.pendingWrites.delete(resolve); resolve(); });
    });
  }

  async load() {
    const instance = this.instance;
    if (!instance || instance.reading) return;
    instance.reading = true;
    instance.loading = true;
    instance.error = '';
    this.showStatus(instance);
    try {
      // Core's row model and xterm must replay the same stream, from byte zero,
      // with the same scrollback limit and no added banners or dropped prefix.
      do {
        const previous = instance.next;
        const response = await this.core.api('GET', `/api/shells/${encodeURIComponent(instance.shellID)}/output-range?start=${previous}&max=${CHUNK_BYTES}`);
        if (this.instance !== instance) return;
        if (response.status >= 400) throw new Error(response.body || `HTTP ${response.status}`);
        const body = response.data;
        const { text, next } = historyChunk(body);
        instance.total = Number(body.total);
        if (Number(body.start) !== previous || !Number.isSafeInteger(next) || !Number.isSafeInteger(instance.total) || next < previous || next > instance.total) throw new Error(t('Invalid history byte range'));
        await this.write(instance, text);
        if (this.instance !== instance) return;
        instance.next = next;
        if (next <= previous) {
          if (next < instance.total) throw new Error(t('History loading stopped before the end of the log'));
          break;
        }
      } while (instance.next < instance.total);
    } catch (error) {
      if (this.instance !== instance) return;
      instance.error = String(error);
    } finally {
      if (this.instance === instance) {
        instance.loading = false;
        instance.reading = false;
        instance.body = null;
        this.showStatus(instance);
        this.updateRail(instance);
      }
    }
  }

  showStatus(instance) {
    if (this.instance !== instance) return;
    this.status({ loading: instance.loading, error: instance.error });
    const page = instance.element?.closest('.history-page');
    const message = page?.querySelector('.history-status');
    if (message) {
      message.hidden = !instance.loading && !instance.error;
      message.textContent = instance.error || t('Loading terminal history…');
      message.classList.toggle('inspector-error', Boolean(instance.error));
    }
  }

  view(instance) {
    const buffer = instance.term.buffer.active;
    return buffer.type === 'alternate' ? null : {
      top: buffer.viewportY, rows: instance.term.rows, cols: instance.term.cols,
    };
  }

  updateRail(instance) {
    if (this.instance !== instance) return;
    this.paint(instance);
    if (instance.loading || !instance.element?.isConnected) return;
    const view = this.view(instance);
    if (!view) return;
    const body = instance.body;
    if (body && body.cols === view.cols && body.height === view.rows &&
        view.top >= body.top && view.top + view.rows <= body.top + body.spans.length) return;
    if (instance.timer !== null) return;
    instance.timer = setTimeout(() => {
      instance.timer = null;
      this.fetchRail(instance);
    }, Math.max(0, 150 - (Date.now() - instance.lastFetch)));
  }

  async fetchRail(instance) {
    if (this.instance !== instance || instance.loading || !instance.element?.isConnected) return;
    const view = this.view(instance);
    if (!view) return;
    const sequence = ++instance.fetchSequence;
    const top = Math.max(0, view.top - 48);
    instance.lastFetch = Date.now();
    try {
      const response = await this.core.api('GET', `/api/shells/${encodeURIComponent(instance.shellID)}/rail?cols=${view.cols}&height=${view.rows}&top=${top}&count=${view.rows + 96}`);
      if (this.instance !== instance || sequence !== instance.fetchSequence) return;
      if (response.status >= 400 || !Array.isArray(response.data?.spans)) throw new Error(response.body || 'Invalid rail response');
      if (Number(response.data.cols) !== instance.term.cols || view.rows !== instance.term.rows) { this.updateRail(instance); return; }
      instance.body = { ...response.data, height: view.rows };
      this.paint(instance);
      const warning = instance.element?.closest('.history-page')?.querySelector('.history-rail-status');
      if (warning) warning.hidden = true;
    } catch (error) {
      if (this.instance !== instance || sequence !== instance.fetchSequence) return;
      const warning = instance.element?.closest('.history-page')?.querySelector('.history-rail-status');
      if (warning) { warning.hidden = false; warning.textContent = t('Colour markers unavailable. Check the Core connection and retry.'); }
      this.core.log('warn', 'History row colours unavailable', String(error));
    }
  }

  paint(instance) {
    if (!instance.element?.isConnected) return;
    const { rail, rowColours, body } = instance;
    rail.replaceChildren();
    rowColours.replaceChildren();
    rowColours.hidden = true;
    const view = this.view(instance);
    if (!view || !body || body.cols !== view.cols || body.height !== view.rows) { rail.hidden = true; return; }
    const spans = Array.from({ length: view.rows }, (_, index) => body.spans[view.top + index - body.top] || null);
    const cells = historyRowCells(body.marks, spans);
    const screen = instance.host.querySelector('.xterm-screen')?.getBoundingClientRect();
    const host = instance.element.getBoundingClientRect();
    if (!screen?.height) { rail.hidden = true; return; }
    rail.style.top = `${screen.top - host.top}px`;
    rail.style.height = `${screen.height}px`;
    rowColours.style.top = `${screen.top - host.top}px`;
    rowColours.style.left = `${screen.left - host.left}px`;
    rowColours.style.width = `${screen.width}px`;
    rowColours.style.height = `${screen.height}px`;
    const fragment = document.createDocumentFragment();
    const rowFragment = document.createDocumentFragment();
    for (let row = 0; row < view.rows; row++) {
      const value = cells?.[row];
      if (!value) continue;
      const cell = document.createElement('button');
      cell.type = 'button';
      cell.className = `history-rail-cell history-colour-${kinds[value.status] || 'output'}`;
      cell.dataset.status = value.status;
      cell.dataset.row = String(view.top + row);
      cell.dataset.start = String(value.start);
      cell.dataset.end = String(value.end);
      cell.style.top = `${row * 100 / view.rows}%`;
      cell.style.height = `${100 / view.rows}%`;
      if (value.cause) {
        cell.dataset.cause = value.cause;
        cell.classList.add(`history-cause-${kinds[value.cause]}`);
        if (cells[row - 1]?.cause !== value.cause) cell.classList.add('history-cause-top');
        if (cells[row + 1]?.cause !== value.cause) cell.classList.add('history-cause-bottom');
      }
      const time = value.time > 0 ? new Date(value.time).toLocaleString() : t('Time unavailable');
      cell.title = `${t(labels[value.status] || 'Terminal output')} · ${time} · ${t('Row')} ${view.top + row + 1} · ${value.start}–${value.end} ${t('bytes')}${value.review ? ` · ${t('Approval decision')}` : ''}`;
      cell.setAttribute('aria-label', cell.title);
      fragment.appendChild(cell);
      if (this.tintRows) {
        const tint = document.createElement('div');
        tint.className = `history-row-colour history-colour-${kinds[value.status] || 'output'}`;
        tint.dataset.row = cell.dataset.row;
        tint.dataset.status = value.status;
        tint.style.top = cell.style.top;
        tint.style.height = cell.style.height;
        rowFragment.appendChild(tint);
      }
    }
    rail.hidden = !fragment.childNodes.length;
    rail.appendChild(fragment);
    rowColours.hidden = !rowFragment.childNodes.length;
    rowColours.appendChild(rowFragment);
  }

  dispose() {
    const instance = this.instance;
    if (!instance) return;
    this.instance = null;
    clearTimeout(instance.timer);
    instance.resize.disconnect();
    instance.subscriptions.forEach(subscription => subscription.dispose());
    instance.pendingWrites.forEach(resolve => resolve());
    instance.term.dispose();
  }
}
