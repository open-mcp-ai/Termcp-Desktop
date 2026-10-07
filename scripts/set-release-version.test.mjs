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

function run(root, version) {
  return spawnSync(process.execPath, [script, version], { cwd: root, encoding: 'utf8' });
}

test('GUI patch release can advance while Core stays pinned', async t => {
  const root = await fixture(t);
  const release = run(root, 'v0.2.6');
  assert.equal(release.status, 0, release.stderr);
  assert.match(release.stdout, /version=0\.2\.6\ntag=v0\.2\.6/);
  assert.equal(JSON.parse(await readFile(join(root, 'package.json'))).version, '0.2.6');
  assert.equal(JSON.parse(await readFile(join(root, 'frontend/package.json'))).version, '0.2.6');
  assert.equal(JSON.parse(await readFile(join(root, 'frontend/package-lock.json'))).packages[''].version, '0.2.6');
  assert.equal(JSON.parse(await readFile(join(root, 'wails.json'))).info.productVersion, '0.2.6');
  assert.match(await readFile(join(root, 'internal/config/product.go'), 'utf8'), /ProductVersion = "0\.2\.6"/);
  const check = run(root, '--check');
  assert.equal(check.status, 0, check.stderr);
  assert.match(check.stdout, /termcp Core v0\.2\.5/);
});

test('four-part release tag fails with guidance and leaves versions unchanged', async t => {
  const root = await fixture(t);
  const release = run(root, 'v0.2.5.1');
  assert.equal(release.status, 1);
  assert.match(release.stderr, /Expected vX\.Y\.Z or X\.Y\.Z/);
  assert.match(release.stderr, /increment the patch component/);
  assert.equal(JSON.parse(await readFile(join(root, 'package.json'))).version, '0.2.5');
});
