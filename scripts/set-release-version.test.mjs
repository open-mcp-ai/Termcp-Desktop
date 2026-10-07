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
    'package.json': JSON.stringify({ name: 'termcp-desktop', version: '0.2.5' }),
    'frontend/package.json': JSON.stringify({ name: 'termcp-frontend', version: '0.2.5' }),
    'frontend/package-lock.json': JSON.stringify({ version: '0.2.5', packages: { '': { version: '0.2.5' } } }),
    'wails.json': JSON.stringify({ info: { productVersion: '0.2.5' } }),
    'internal/config/product.go': 'package config\nvar ProductVersion = "0.2.5"\n',
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

test('GUI patch release can advance while Core stays pinned', async t => {
  const root = await fixture(t);
  const release = run(root, 'v0.2.6+core.0.2.5');
  assert.equal(release.status, 0, release.stderr);
  assert.match(release.stdout, /version=0\.2\.6\ncore_version=0\.2\.5\ntag=v0\.2\.6\+core\.0\.2\.5/);
  assert.equal(JSON.parse(await readFile(join(root, 'package.json'))).version, '0.2.6');
  assert.equal(JSON.parse(await readFile(join(root, 'frontend/package.json'))).version, '0.2.6');
  assert.equal(JSON.parse(await readFile(join(root, 'frontend/package-lock.json'))).packages[''].version, '0.2.6');
  assert.equal(JSON.parse(await readFile(join(root, 'wails.json'))).info.productVersion, '0.2.6');
  assert.match(await readFile(join(root, 'internal/config/product.go'), 'utf8'), /ProductVersion = "0\.2\.6"/);
  const check = run(root, '--check');
  assert.equal(check.status, 0, check.stderr);
  assert.match(check.stdout, /termcp Core v0\.2\.5/);
  const releaseCheck = run(root, '--check', 'v0.2.6+core.0.2.5');
  assert.equal(releaseCheck.status, 0, releaseCheck.stderr);
  assert.equal(releaseCheck.stdout, 'version=0.2.6\ncore_version=0.2.5\ntag=v0.2.6+core.0.2.5\n');
});

test('release tag must use the composite three-part format', async t => {
  const root = await fixture(t);
  for (const tag of ['v0.2.5.1', 'v0.2.6', 'v0.2.6+core.0.2.5.1']) {
    const release = run(root, tag);
    assert.equal(release.status, 1);
    assert.match(release.stderr, /Expected vX\.Y\.Z\+core\.A\.B\.C/);
  }
  assert.equal(JSON.parse(await readFile(join(root, 'package.json'))).version, '0.2.5');
});

test('release tag must match both the Core pin and checked-in GUI version', async t => {
  const root = await fixture(t);
  const wrongCore = run(root, 'v0.2.6+core.0.2.6');
  assert.equal(wrongCore.status, 1);
  assert.match(wrongCore.stderr, /does not match go\.mod Core v0\.2\.5/);
  const wrongGUI = run(root, '--check', 'v0.2.6+core.0.2.5');
  assert.equal(wrongGUI.status, 1);
  assert.match(wrongGUI.stderr, /does not match package\.json v0\.2\.5/);
  assert.equal(JSON.parse(await readFile(join(root, 'package.json'))).version, '0.2.5');
});
