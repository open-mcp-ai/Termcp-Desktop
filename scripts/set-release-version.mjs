import { readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const raw = String(process.argv[2] || '').trim();
const checkOnly = raw === '--check';
const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(raw);
const root = process.cwd();
const goMod = await readFile(resolve(root, 'go.mod'), 'utf8');
const coreMatch = /^\s*github\.com\/open-mcp-ai\/termcp v(\d+\.\d+\.\d+)\s*$/m.exec(goMod);

if (!coreMatch) {
  console.error('go.mod must pin a released termcp Core version.');
  process.exit(1);
}

if (!checkOnly && !match) {
  console.error(`Invalid release version "${raw}". Expected vX.Y.Z or X.Y.Z.`);
  process.exit(1);
}

const version = checkOnly ? coreMatch[1] : `${match[1]}.${match[2]}.${match[3]}`;
if (version !== coreMatch[1]) {
  console.error(`GUI v${version} must match the pinned termcp Core v${coreMatch[1]}.`);
  process.exit(1);
}

const paths = {
  package: resolve(root, 'package.json'),
  frontend: resolve(root, 'frontend/package.json'),
  lock: resolve(root, 'frontend/package-lock.json'),
  wails: resolve(root, 'wails.json'),
  product: resolve(root, 'internal/config/product.go'),
};
const files = Object.fromEntries(await Promise.all(Object.entries(paths).map(async ([name, path]) => [name, await readFile(path, 'utf8')])));
const pkg = JSON.parse(files.package);
const frontend = JSON.parse(files.frontend);
const lock = JSON.parse(files.lock);
const config = JSON.parse(files.wails);
const productMatch = /var ProductVersion = "(\d+\.\d+\.\d+)"/.exec(files.product);
if (!productMatch) throw new Error('internal/config/product.go must define ProductVersion.');

if (checkOnly) {
  const actual = {
    'package.json': pkg.version,
    'frontend/package.json': frontend.version,
    'frontend/package-lock.json': lock.version,
    'frontend/package-lock.json root package': lock.packages?.['']?.version,
    'wails.json': config.info?.productVersion,
    'internal/config/product.go': productMatch[1],
  };
  const mismatches = Object.entries(actual).filter(([, value]) => value !== version);
  if (mismatches.length) {
    console.error(`GUI version must match termcp Core v${version}: ${mismatches.map(([name, value]) => `${name}=${value}`).join(', ')}`);
    process.exit(1);
  }
  console.log(`GUI and termcp Core versions match: v${version}`);
  process.exit(0);
}

pkg.version = version;
frontend.version = version;
lock.version = version;
lock.packages[''].version = version;
config.name = 'Termcp';
config.outputfilename = 'Termcp';
config.info ||= {};
config.info.productName = 'Termcp';
config.info.productVersion = version;

await Promise.all([
  writeFile(paths.package, `${JSON.stringify(pkg, null, 2)}\n`, 'utf8'),
  writeFile(paths.frontend, `${JSON.stringify(frontend, null, 2)}\n`, 'utf8'),
  writeFile(paths.lock, `${JSON.stringify(lock, null, 2)}\n`, 'utf8'),
  writeFile(paths.wails, `${JSON.stringify(config, null, 2)}\n`, 'utf8'),
  writeFile(paths.product, files.product.replace(productMatch[0], `var ProductVersion = "${version}"`), 'utf8'),
]);

// The output format can be appended directly to GitHub Actions' GITHUB_OUTPUT.
console.log(`version=${version}`);
console.log(`tag=v${version}`);
