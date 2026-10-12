import test, { before, after } from 'node:test';
import assert from 'node:assert/strict';
import { fileURLToPath } from 'node:url';
import { createServer } from 'vite';
import { chromium } from 'playwright';

let server;
let browser;
let origin;

before(async () => {
  server = await createServer({
    root: fileURLToPath(new URL('../../', import.meta.url)),
    server: { host: '127.0.0.1', port: 0 },
  });
  await server.listen();
  origin = `http://127.0.0.1:${server.httpServer.address().port}`;
  browser = await chromium.launch();
});

after(async () => {
  await browser?.close();
  await server?.close();
});

async function layout(page) {
  return page.evaluate(() => {
    const bounds = selector => {
      const rect = document.querySelector(selector).getBoundingClientRect();
      return { top: rect.top, bottom: rect.bottom, height: rect.height };
    };
    const tree = document.querySelector('.tree-scroll');
    return {
      viewportHeight: window.innerHeight,
      footer: bounds('.shell-tabbar'),
      settings: bounds('.rail-bottom'),
      explorer: bounds('.explorer'),
      treeHeight: tree.clientHeight,
      treeScrollHeight: tree.scrollHeight,
    };
  });
}

function assertFooterVisible(measurement) {
  assert.ok(measurement.footer.height >= 41, 'Shell footer must retain its height');
  assert.ok(measurement.footer.bottom <= measurement.viewportHeight + 1,
    `Shell footer bottom ${measurement.footer.bottom} exceeds viewport ${measurement.viewportHeight}`);
  assert.ok(measurement.settings.bottom <= measurement.viewportHeight + 1,
    'Settings navigation must remain inside the window');
}

for (const width of [1280, 800, 640]) {
  test(`expanded sidebar scrolls without pushing the footer outside a ${width}px window`, async () => {
    const context = await browser.newContext({ viewport: { width, height: 720 } });
    try {
      const page = await context.newPage();
      await page.goto(origin);
      await page.waitForSelector('.shell-tab');
      await page.waitForFunction(async () => !(await import('/src/state.js')).state.loading);
      const initial = await layout(page);
      assertFooterVisible(initial);

      // Populate the real resource tree, then expand through the app's click handler.
      await page.evaluate(async () => {
        const { state } = await import('/src/state.js');
        state.data.sessions[0].shells.push(...Array.from({ length: 30 }, (_, index) => ({
          id: `layout-shell-${index}`, name: `Layout shell ${index}`, status: 'running', mode: 'pty',
        })));
      });
      await page.locator('[data-toggle="session:session-demo-01"]').click();
      const expanded = await layout(page);
      assertFooterVisible(expanded);
      assert.equal(expanded.footer.bottom, initial.footer.bottom);
      assert.ok(expanded.explorer.bottom <= expanded.viewportHeight + 1);
      assert.ok(expanded.treeScrollHeight > expanded.treeHeight, 'Long resource tree must scroll');
      await page.locator('.tree-scroll [data-open-shell="layout-shell-29"]').scrollIntoViewIfNeeded();
      const lastRow = await page.locator('.tree-scroll [data-open-shell="layout-shell-29"]').boundingBox();
      assert.ok(lastRow.y + lastRow.height <= expanded.explorer.bottom + 1,
        'The last shell must be reachable within the sidebar');

      await page.locator('[data-group-toggle="session-list"]').click();
      assertFooterVisible(await layout(page));
      await page.locator('[data-group-toggle="session-list"]').click();
      assertFooterVisible(await layout(page));
      await page.locator('[data-sidebar-pin]').click();
      assertFooterVisible(await layout(page));
      await page.locator('[data-sidebar-pin]').click();
      assertFooterVisible(await layout(page));
      await page.locator('.rail [data-sidebar-toggle]').click();
      assertFooterVisible(await layout(page));
      await page.locator('.rail [data-sidebar-toggle]').click();
      assertFooterVisible(await layout(page));
      await page.setViewportSize({ width, height: 480 });
      assertFooterVisible(await layout(page));
    } finally {
      await context.close();
    }
  });
}

for (const target of [
  { sessionID: 'session-demo-01', shellID: 'shell-demo-02', name: 'logs' },
  { sessionID: 'session-demo-02', shellID: 'shell-demo-03', name: 'zsh' },
]) {
  test(`sidebar context menu closes only ${target.shellID}, including inactive shells`, async () => {
    const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
    try {
      const page = await context.newPage();
      await page.goto(origin);
      await page.waitForSelector('.shell-tab');
      await page.waitForFunction(async () => !(await import('/src/state.js')).state.loading);
      await page.locator(`[data-toggle="session:${target.sessionID}"]`).click();
      await page.locator(`.tree-scroll [data-open-shell="${target.shellID}"] b`).click({ button: 'right' });
      const menu = page.locator('.terminal-context-menu');
      assert.equal(await menu.count(), 1, 'A shell row must open the shell context menu');
      assert.equal(await page.locator('.session-context-menu').count(), 0);
      assert.equal(await menu.getAttribute('data-terminal-menu'), target.shellID);
      assert.equal(await menu.locator('header b').textContent(), target.name);
      await menu.locator('[data-terminal-action="close"]').click();
      const confirm = page.locator('.modal [data-confirm="close-shell"]');
      assert.equal(await confirm.count(), 1, 'Closing a shell must use the shell confirmation');
      assert.equal(await confirm.getAttribute('data-payload'), target.shellID);
      await page.evaluate(async () => {
        const { core } = await import('/src/core.js');
        const originalAPI = core.api.bind(core);
        window.closeRequests = [];
        core.api = (method, path, ...args) => {
          if (method !== 'GET') window.closeRequests.push({ method, path });
          return originalAPI(method, path, ...args);
        };
      });
      await confirm.click();
      await page.waitForFunction(() => window.closeRequests.length > 0);
      assert.deepEqual(await page.evaluate(() => window.closeRequests), [
        { method: 'DELETE', path: `/api/shells/${target.shellID}` },
      ]);
      await page.locator(`[data-session-context="${target.sessionID}"] .tree-row.sub`).click({ button: 'right' });
      assert.equal(await page.locator('.session-context-menu').count(), 1,
        'Session rows must retain their session context menu');
    } finally {
      await context.close();
    }
  });
}
