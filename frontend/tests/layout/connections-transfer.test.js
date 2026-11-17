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

async function openPage() {
  const context = await browser.newContext({ viewport: { width: 1280, height: 720 } });
  const page = await context.newPage();
  await page.goto(origin);
  await page.waitForFunction(async () => !(await import('/src/state.js')).state.loading);
  return { context, page };
}

test('host transfer dialogs fit and never offer the built-in profile', async () => {
  const { context, page } = await openPage();
  try {
    await page.click('[data-action="export-connections"]');
    assert.equal(await page.locator('.transfer-modal').count(), 1);
    assert.equal(await page.locator('input[name="connections"]').count(), 2);
    assert.equal(await page.locator('input[name="connections"]').evaluateAll(items => items.some(item => item.value === 'internal')), false);
    for (const [width, height] of [[1280, 720], [640, 480]]) {
      await page.setViewportSize({ width, height });
      const measurement = await page.locator('.transfer-modal').evaluate(element => {
        const box = element.getBoundingClientRect();
        return { bottom: box.bottom, right: box.right, viewport: innerHeight, width: innerWidth, overflow: element.scrollWidth > element.clientWidth + 1 };
      });
      assert.ok(measurement.bottom <= measurement.viewport + 1);
      assert.ok(measurement.right <= measurement.width + 1);
      assert.equal(measurement.overflow, false);
    }
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.click('.transfer-modal [data-close]');
    await page.click('[data-action="import-connections"]');
    await page.evaluate(async () => {
      const { core } = await import('/src/core.js');
      const snapshot = core.snapshot.bind(core);
      window.importSnapshotCalls = 0;
      core.snapshot = async () => { window.importSnapshotCalls += 1; return snapshot(); };
    });
    await page.fill('input[name="password"]', 'preview-password');
    await page.click('.transfer-modal button[type="submit"]');
    await page.waitForSelector('.transfer-result-row');
    assert.match(await page.locator('.transfer-result-summary').innerText(), /2 imported · 1 renamed/);
    assert.equal(await page.locator('.transfer-result-row').count(), 2);
    assert.match(await page.locator('.transfer-result-row').nth(1).innerText(), /staging-2/);
    assert.ok(await page.evaluate(() => window.importSnapshotCalls > 0), 'Import must refresh the resource tree from Core');
  } finally {
    await context.close();
  }
});
