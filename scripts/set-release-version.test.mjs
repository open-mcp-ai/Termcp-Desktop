import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import { mkdtemp, mkdir, readFile, rm, writeFile } from 'node:fs/promises';
import { tmpdir } from 'node:os';
import { dirname, join } from 'node:path';
import { test } from 'node:test';
import { fileURLToPath } from 'node:url';

const script = fileURLToPath(new URL('./set-release-version.mjs', import.meta.url));

async function fixture(t) {
  const root = await mkdtemp(join(tmpdir(), 'termcp-release-'));
  t.after(() => rm(root, { recursive: true, force: true }));
  const contents = {
    'go.mod': 'module example.test/desktop\n\nrequire (\n  github.com/open-mcp-ai/termcp v0.2.5\n)\n',
    'package.json': JSON.stringify({ name: 'termcp-desktop', version: '0.1.1' }),
    'frontend/package.json': JSON.stringify({ name: 'termcp-frontend', version: '0.1.1' }),
    'frontend/package-lock.json': JSON.stringify({ version: '0.1.1', packages: { '': { version: '0.1.1' } } }),
    'wails.json': JSON.stringify({ info: { productVersion: '0.1.1' } }),
    'internal/config/product.go': 'package config\nvar ProductVersion = "0.1.1"\nvar CoreVersion = "0.2.5"\n',
    'frontend/src/build-info.js': "export const desktopVersion = '0.1.1';\nexport const bundledCoreVersion = '0.2.5';\n",
  };
  for (const [path, content] of Object.entries(contents)) {
    const target = join(root, path);
    await mkdir(dirname(target), { recursive: true });
    await writeFile(target, content);
  }
  return root;
}

function run(root, ...args) {
  return spawnSync(process.execPath, [script, ...args], { cwd: root, encoding: 'utf8' });
}

test('Desktop patch release advances independently of the bundled Core', async t => {
  const root = await fixture(t);
  const release = run(root, 'v0.1.2');
  assert.equal(release.status, 0, release.stderr);
  assert.equal(release.stdout, 'version=0.1.2\ncore_version=0.2.5\ntag=v0.1.2\n');
  assert.equal(JSON.parse(await readFile(join(root, 'package.json'))).version, '0.1.2');
  assert.equal(JSON.parse(await readFile(join(root, 'frontend/package.json'))).version, '0.1.2');
  assert.equal(JSON.parse(await readFile(join(root, 'frontend/package-lock.json'))).packages[''].version, '0.1.2');
  assert.equal(JSON.parse(await readFile(join(root, 'wails.json'))).info.productVersion, '0.1.2');
  assert.match(await readFile(join(root, 'internal/config/product.go'), 'utf8'), /ProductVersion = "0\.1\.2"\nvar CoreVersion = "0\.2\.5"/);
  assert.match(await readFile(join(root, 'frontend/src/build-info.js'), 'utf8'), /desktopVersion = '0\.1\.2';\nexport const bundledCoreVersion = '0\.2\.5'/);
  const check = run(root, '--check');
  assert.equal(check.status, 0, check.stderr);
  assert.match(check.stdout, /bundled Core v0\.2\.5/);
  const releaseCheck = run(root, '--check', 'v0.1.2');
  assert.equal(releaseCheck.status, 0, releaseCheck.stderr);
  assert.equal(releaseCheck.stdout, 'version=0.1.2\ncore_version=0.2.5\ntag=v0.1.2\n');
});

test('release tag contains only the three-part Desktop version', async t => {
  const root = await fixture(t);
  for (const tag of ['v0.1.2.1', '0.1.2', 'v0.1.2+core.0.2.5']) {
    const release = run(root, tag);
    assert.equal(release.status, 1);
    assert.match(release.stderr, /Expected vX\.Y\.Z/);
  }
  assert.equal(JSON.parse(await readFile(join(root, 'package.json'))).version, '0.1.1');
});

test('release tag and displayed Core version must match source metadata', async t => {
  const root = await fixture(t);
  const wrongGUI = run(root, '--check', 'v0.1.2');
  assert.equal(wrongGUI.status, 1);
  assert.match(wrongGUI.stderr, /does not match package\.json v0\.1\.1/);
  await writeFile(join(root, 'frontend/src/build-info.js'), "export const desktopVersion = '0.1.1';\nexport const bundledCoreVersion = '0.2.4';\n");
  const wrongCore = run(root, '--check');
  assert.equal(wrongCore.status, 1);
  assert.match(wrongCore.stderr, /frontend v0\.2\.4/);
});

test('Core upgrade updates the app labels without changing the Desktop version scheme', async t => {
  const root = await fixture(t);
  await writeFile(join(root, 'go.mod'), 'module example.test/desktop\n\nrequire (\n  github.com/open-mcp-ai/termcp v0.2.6\n)\n');
  const release = run(root, 'v0.1.2');
  assert.equal(release.status, 0, release.stderr);
  assert.match(await readFile(join(root, 'internal/config/product.go'), 'utf8'), /CoreVersion = "0\.2\.6"/);
  assert.match(await readFile(join(root, 'frontend/src/build-info.js'), 'utf8'), /bundledCoreVersion = '0\.2\.6'/);
});
