import test, { before, after } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let server, browser, origin;
before(async () => {
  server = await createServer({ root: fileURLToPath(new URL('../../', import.meta.url)), server: { host: '127.0.0.1', port: 0 } });
  await server.listen();
  origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch();
});
after(async () => { await browser?.close(); await server?.close(); });

async function openHistory(page, prepare = async () => {}) {
  await page.goto(origin);
  await page.waitForFunction(async () => !(await import('/src/state.js')).state.loading);
  await page.evaluate(async () => {
    const OriginalTerminal = window.Terminal;
    window.historyTerminals = [];
    window.Terminal = class extends OriginalTerminal {
      constructor(options) { super(options); window.historyTerminals.push(this); }
    };
    window.historyRequests = [];
    const { core } = await import('/src/core.js');
    const api = core.api.bind(core);
    core.api = (method, path, ...args) => { window.historyRequests.push({ method, path }); return api(method, path, ...args); };
  });
  await prepare();
  await page.click('[data-section="history"]');
  await page.click('[data-select="history:history-demo-01"]');
  await page.waitForFunction(async () => !(await import('/src/state.js')).state.historyLoading);
}

test('archived history replays commands and output in one read-only terminal with row colours', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  try {
    const page = await context.newPage();
    await openHistory(page);
    assert.equal(await page.locator('.history-terminal .xterm').count(), 1,
      'History must replay the terminal stream instead of displaying offset-indexed excerpts');
    await page.waitForSelector('.history-rail-cell[data-status="i"]');
    await page.waitForSelector('.history-rail-cell[data-status="a"]');
    await page.waitForSelector('.history-rail-cell[data-status="o"]');
    const colours = await page.locator('.history-legend i').evaluateAll(items => items.map(item => getComputedStyle(item).backgroundColor));
    assert.equal(new Set(colours).size, 3);
    await page.locator('.history-rail-cell[data-status="i"]').first().click();
    assert.match(await page.locator('.history-row-detail').innerText(), /Human input/);
    assert.equal(await page.locator('.history-timeline').count(), 0);
    const content = await page.evaluate(() => {
      const term = window.historyTerminals[0];
      const buffer = term.buffer.active;
      return { readOnly: term.options.disableStdin,
        lines: Array.from({ length: buffer.length }, (_, row) => buffer.getLine(row).translateToString(true)).join('\n') };
    });
    assert.equal(content.readOnly, true);
    assert.match(content.lines, /\$ npm test\n✓ all tests passed\n\$ go version/);
    assert.ok((await page.evaluate(() => window.historyRequests)).every(request => request.method === 'GET'));
    const toggle = page.locator('[data-action="history-tint-rows"]');
    assert.equal(await toggle.getAttribute('aria-pressed'), 'false');
    assert.equal(await page.locator('.history-row-colours').isVisible(), false);
    await toggle.click();
    assert.equal(await toggle.getAttribute('aria-pressed'), 'true');
    const tinted = await page.locator('.history-row-colour').evaluateAll(rows => rows.map(row => {
      const screen = document.querySelector('.history-xterm .xterm-screen').getBoundingClientRect();
      const rect = row.getBoundingClientRect();
      return { status: row.dataset.status, width: rect.width, screenWidth: screen.width,
        transparentToClicks: getComputedStyle(row).pointerEvents === 'none', colour: getComputedStyle(row).backgroundColor };
    }));
    assert.deepEqual(tinted.map(row => row.status), ['i', 'o', 'a', 'o', 'o']);
    assert.equal(new Set(tinted.map(row => row.colour)).size, 3);
    assert.ok(tinted.every(row => row.width === row.screenWidth && row.transparentToClicks));
    await toggle.click();
    assert.equal(await page.locator('.history-row-colours').isVisible(), false);
    assert.equal(await page.evaluate(() => window.historyTerminals.length), 1, 'Toggling row tint must not reload the terminal');
  } finally { await context.close(); }
});

test('rail geometry stays on terminal rows after narrow layouts, font changes and app rerenders', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  try {
    const page = await context.newPage();
    await openHistory(page);
    await page.click('[data-action="history-tint-rows"]');
    for (const [width, height] of [[1280, 720], [800, 480], [640, 480], [1280, 720]]) {
      await page.setViewportSize({ width, height });
      await page.waitForFunction(() => {
        const term = window.historyTerminals[0];
        const screen = document.querySelector('.history-terminal .xterm-screen').getBoundingClientRect();
        const cell = document.querySelector('.history-rail-cell[data-row="0"]');
        if (!cell) return false;
        const bounds = cell.getBoundingClientRect();
        const tint = document.querySelector('.history-row-colour[data-row="0"]')?.getBoundingClientRect();
        return tint && Math.abs(bounds.top - screen.top) < 1 && Math.abs(bounds.height - screen.height / term.rows) < 1 &&
          Math.abs(tint.top - bounds.top) < 1 && Math.abs(tint.height - bounds.height) < 1 && Math.abs(tint.width - screen.width) < 1;
      });
      const measurement = await page.evaluate(() => {
        const workspace = document.querySelector('.history-workspace').getBoundingClientRect();
        const term = document.querySelector('.history-terminal').getBoundingClientRect();
        return { bottom: workspace.bottom, height: term.height, right: workspace.right, width: innerWidth, viewport: innerHeight,
          overflow: document.documentElement.scrollWidth > innerWidth };
      });
      assert.ok(measurement.bottom <= measurement.viewport);
      assert.ok(measurement.right <= measurement.width);
      assert.ok(measurement.height >= 100, `History needs a usable viewport at ${width}x${height}`);
      assert.equal(measurement.overflow, false);
    }
    await page.evaluate(async () => { (await import('/src/appearance.js')).setAppearance({ fontSize: 20, theme: 'dark' }); });
    await page.click('[data-sidebar-pin]');
    await page.waitForSelector('.history-rail-cell[data-status="i"]');
    assert.equal(await page.evaluate(() => window.historyTerminals.length), 1, 'App rerenders must preserve the history terminal');
    assert.equal(await page.evaluate(() => window.historyRequests.filter(request => request.path.includes('output-range')).length), 1,
      'App rerenders must not replay the log or reset scrollback');
    assert.equal(await page.evaluate(() => window.historyTerminals[0].options.fontSize), 20);
    assert.equal(await page.locator('[data-action="history-tint-rows"]').getAttribute('aria-pressed'), 'true');
    assert.equal(await page.evaluate(() => localStorage.getItem('termcp-desktop-history-tint-rows')), '1');
  } finally { await context.close(); }
});

test('long logs use Core byte cursors and row layouts through scrolling and reflow', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  try {
    const page = await context.newPage();
    await openHistory(page, () => page.evaluate(async () => {
      const { core } = await import('/src/core.js');
      const api = core.api.bind(core);
      const lines = Array.from({ length: 240 }, (_, index) => `${String(index).padStart(3, '0')} ${'x'.repeat(96)}\r\n`);
      const bytes = new TextEncoder().encode(lines.join(''));
      core.api = async (method, path, ...args) => {
        const url = new URL(path, location.origin);
        if (url.pathname.endsWith('/output-range')) {
          const start = Number(url.searchParams.get('start'));
          const end = Math.min(start + 113, bytes.length);
          window.historyRequests.push({ method, path });
          return { data: { start, end, total: bytes.length, d: new TextDecoder().decode(bytes.slice(start, end)) } };
        }
        if (url.pathname.endsWith('/rail')) {
          const cols = Number(url.searchParams.get('cols'));
          const top = Number(url.searchParams.get('top'));
          const count = Number(url.searchParams.get('count'));
          const rows = [];
          let offset = 0;
          for (const line of lines) {
            for (let at = 0; at < line.length - 2; at += cols) {
              rows.push({ start: offset + at, end: offset + Math.min(at + cols, line.length - 2) + (at + cols >= line.length - 2 ? 2 : 0) });
            }
            offset += line.length;
          }
          window.historyRequests.push({ method, path });
          return { data: { cols, top, spans: Array.from({ length: count }, (_, row) => rows[top + row] || null),
            total_rows: rows.length, marks: [{ status: 'o', time: 1, start: 0, end: bytes.length }] } };
        }
        return api(method, path, ...args);
      };
    }));
    await page.waitForSelector('.history-rail-cell');
    await page.evaluate(() => window.historyTerminals[0].scrollToTop());
    await page.waitForSelector('.history-rail-cell[data-row="0"]');
    await page.evaluate(() => window.historyTerminals[0].scrollToLine(100));
    await page.waitForSelector('.history-rail-cell[data-row="100"]');
    const before = await page.evaluate(() => window.historyTerminals[0].buffer.active.viewportY);
    await page.click('[data-sidebar-pin]');
    assert.equal(await page.evaluate(() => window.historyTerminals[0].buffer.active.viewportY), before);
    const previousCols = await page.evaluate(() => window.historyTerminals[0].cols);
    await page.setViewportSize({ width: 640, height: 480 });
    await page.waitForFunction(previous => window.historyTerminals[0].cols < previous, previousCols);
    await page.evaluate(() => window.historyTerminals[0].scrollToTop());
    await page.waitForFunction(() => {
      const cols = window.historyTerminals[0].cols;
      const cell = document.querySelector('.history-rail-cell[data-row="0"]');
      return cell && Number(cell.dataset.end) === cols;
    });
    assert.equal(await page.evaluate(() => window.historyTerminals[0].buffer.active.getLine(0).translateToString(true).slice(0, 4)), '000 ');
    const requests = await page.evaluate(() => window.historyRequests.filter(request => request.path.includes('output-range')));
    assert.ok(requests.length > 2, 'The fixture forces multiple byte windows');
    assert.equal(new URL(requests[1].path, origin).searchParams.get('start'), '113', 'Use Core end, not JS string or row counts');
  } finally { await context.close(); }
});

test('switching shells discards delayed row-layout responses for the old shell', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  try {
    const page = await context.newPage();
    await openHistory(page, () => page.evaluate(async () => {
      const { state } = await import('/src/state.js');
      const { core } = await import('/src/core.js');
      state.data.history[0].shells.push({ id: 'other-history-shell', name: 'Other', mode: 'pipe' });
      const api = core.api.bind(core);
      core.api = async (method, path, ...args) => {
        const url = new URL(path, location.origin);
        if (url.pathname.includes('history-shell-01/rail')) {
          const result = await api(method, path, ...args);
          await new Promise(resolve => { window.releaseOldRail = resolve; });
          return result;
        }
        if (url.pathname.includes('other-history-shell/output-range')) return { data: { start: 0, end: 13, total: 13, d: 'other history' } };
        if (url.pathname.includes('other-history-shell/rail')) return { data: {
          cols: Number(url.searchParams.get('cols')), top: 0, total_rows: 1,
          spans: Array.from({ length: Number(url.searchParams.get('count')) }, (_, index) => index === 0 ? { start: 0, end: 13 } : null),
          marks: [{ status: 'a', time: 1, start: 0, end: 0 }, { status: 'o', time: 2, start: 0, end: 13 }],
        } };
        return api(method, path, ...args);
      };
    }));
    await page.waitForFunction(() => Boolean(window.releaseOldRail));
    await page.click('[data-history-shell="other-history-shell"]');
    await page.waitForSelector('.history-rail-cell[data-status="a"]');
    await page.evaluate(() => window.releaseOldRail());
    await page.waitForTimeout(200);
    assert.equal(await page.locator('.history-rail-cell[data-status="i"]').count(), 0);
    assert.equal(await page.evaluate(() => window.historyTerminals.at(-1).buffer.active.getLine(0).translateToString(true)), 'other history');
    assert.equal(await page.locator('.history-rail-cell').count(), 1);
  } finally { await context.close(); }
});

test('replay preserves ANSI, cursor edits and UTF-8 characters split between byte windows', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  try {
    const page = await context.newPage();
    await openHistory(page, () => page.evaluate(async () => {
      const { core } = await import('/src/core.js');
      const api = core.api.bind(core);
      const lines = ['progress 1\rprogress 2\r\n', '\x1b[32m中🙂\x1b[0m\r\n', 'abc\bD\r\n'];
      const bytes = new TextEncoder().encode(lines.join(''));
      let offset = 0;
      const rows = lines.map(line => { const start = offset; offset += new TextEncoder().encode(line).length; return { start, end: offset }; });
      core.api = async (method, path, ...args) => {
        const url = new URL(path, location.origin);
        if (url.pathname.endsWith('/output-range')) {
          const start = Number(url.searchParams.get('start'));
          const end = Math.min(start + 5, bytes.length);
          return { data: { start, end, total: bytes.length, d: new TextDecoder().decode(bytes.slice(start, end)) } };
        }
        if (url.pathname.endsWith('/rail')) return { data: {
          cols: Number(url.searchParams.get('cols')), top: 0, total_rows: 3,
          spans: Array.from({ length: Number(url.searchParams.get('count')) }, (_, index) => rows[index] || null),
          marks: [{ status: 'o', time: 1, start: 0, end: bytes.length }],
        } };
        return api(method, path, ...args);
      };
    }));
    await page.waitForSelector('.history-rail-cell');
    const lines = await page.evaluate(() => {
      const buffer = window.historyTerminals[0].buffer.active;
      return [0, 1, 2].map(row => buffer.getLine(row).translateToString(true));
    });
    assert.deepEqual(lines, ['progress 2', '中🙂', 'abD']);
    assert.equal(await page.locator('.history-rail-cell').count(), 3, 'Blank rows must have no colour marker');
  } finally { await context.close(); }
});

test('a failed archive load displays an error and can be reloaded', async () => {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  try {
    const page = await context.newPage();
    await openHistory(page, () => page.evaluate(async () => {
      const { core } = await import('/src/core.js');
      const api = core.api.bind(core);
      let failed = false;
      core.api = async (method, path, ...args) => {
        if (path.includes('output-range') && !failed) { failed = true; throw new Error('Archive read failed'); }
        return api(method, path, ...args);
      };
    }));
    assert.match(await page.locator('.history-status').innerText(), /Archive read failed/);
    await page.click('[data-action="history-reload"]');
    await page.waitForSelector('.history-rail-cell[data-status="i"]');
    assert.equal(await page.locator('.history-status').isVisible(), false);
  } finally { await context.close(); }
});
