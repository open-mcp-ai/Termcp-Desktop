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
  test(`extension templates and editor fit at ${width}px`, async () => {
    const context = await browser.newContext({ viewport: { width, height: 760 } });
    try {
      const page = await context.newPage();
      await page.goto(origin);
      await page.waitForFunction(async () => !(await import('/src/state.js')).state.loading);
      await page.locator('.rail [data-section="extensions"]').click();
      await page.getByRole('heading', { name: 'Extensions' }).waitFor();
      await page.waitForFunction(async () => (await import('/src/state.js')).state.extensions.length > 0);

      const sizing = await page.evaluate(() => ({
        body: document.body.scrollWidth,
        page: document.querySelector('#main-content').scrollWidth,
        viewport: window.innerWidth,
      }));
      assert.ok(sizing.body <= sizing.viewport + 1, `body overflows: ${JSON.stringify(sizing)}`);
      assert.ok(sizing.page <= sizing.viewport + 1, `content overflows: ${JSON.stringify(sizing)}`);

      await page.getByRole('button', { name: /New template/ }).click();
      await page.getByRole('heading', { name: 'New extension template' }).waitFor();
      const modalBounds = await page.locator('.extension-editor-modal').boundingBox();
      assert.ok(modalBounds.y >= 0, 'editor starts above the viewport');
      assert.ok(modalBounds.y + modalBounds.height <= 761, 'editor extends below the viewport');
      await page.getByRole('button', { name: 'TOML' }).click();
      await page.locator('textarea[name="toml"]').waitFor();
      await page.getByRole('button', { name: 'Cancel' }).click();
    } finally {
      await context.close();
    }
  });
}
