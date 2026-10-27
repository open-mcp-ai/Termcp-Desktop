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

for (const width of [1280, 640]) {
  test(`runtime observability fits and links activity to history at ${width}px`, async () => {
    const context = await browser.newContext({ viewport: { width, height: 760 } });
    try {
      const page = await context.newPage();
      await page.goto(origin);
      await page.waitForFunction(async () => !(await import('/src/state.js')).state.loading);
      await page.locator('.rail [data-section="observability"]').click();
      await page.getByRole('heading', { name: 'Runtime observability' }).waitFor();
      await page.waitForFunction(async () => {
        const { state } = await import('/src/state.js');
        return !state.observability.loading && state.observability.activity.length > 0;
      });
      const sizing = await page.evaluate(() => ({
        body: document.body.scrollWidth,
        page: document.querySelector('#main-content').scrollWidth,
        viewport: window.innerWidth,
      }));
      assert.ok(sizing.body <= sizing.viewport + 1, `body overflows: ${JSON.stringify(sizing)}`);
      assert.ok(sizing.page <= sizing.viewport + 1, `content overflows: ${JSON.stringify(sizing)}`);

      await page.locator('[data-activity-open]').first().click();
      await page.waitForSelector('.history-page');
      await page.waitForFunction(() => /Located event at byte/.test(document.querySelector('.history-row-detail')?.textContent || ''));
      await page.waitForFunction(() => document.querySelectorAll('.history-focus-cell').length > 0);
    } finally {
      await context.close();
    }
  });
}
