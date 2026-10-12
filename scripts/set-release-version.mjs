import { readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const checkOnly = process.argv[2] === '--check';
const raw = String(process.argv[checkOnly ? 3 : 2] || '').trim();
const match = /^v(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.exec(raw);
const root = process.cwd();
const goMod = await readFile(resolve(root, 'go.mod'), 'utf8');
const coreMatch = /^\s*github\.com\/open-mcp-ai\/termcp v(\d+\.\d+\.\d+)\s*$/m.exec(goMod);

if (!coreMatch) {
  console.error('go.mod must pin a released termcp Core version.');
  process.exit(1);
}

const paths = {
  package: resolve(root, 'package.json'),
  frontend: resolve(root, 'frontend/package.json'),
  lock: resolve(root, 'frontend/package-lock.json'),
  wails: resolve(root, 'wails.json'),
  product: resolve(root, 'internal/config/product.go'),
  frontendBuild: resolve(root, 'frontend/src/build-info.js'),
};
const files = Object.fromEntries(await Promise.all(Object.entries(paths).map(async ([name, path]) => [name, await readFile(path, 'utf8')])));
const pkg = JSON.parse(files.package);
const frontend = JSON.parse(files.frontend);
const lock = JSON.parse(files.lock);
const config = JSON.parse(files.wails);
const productMatch = /var ProductVersion = "(\d+\.\d+\.\d+)"/.exec(files.product);
if (!productMatch) throw new Error('internal/config/product.go must define ProductVersion.');
const bundledCoreMatch = /var CoreVersion = "(\d+\.\d+\.\d+)"/.exec(files.product);
if (!bundledCoreMatch) throw new Error('internal/config/product.go must define CoreVersion.');
const frontendVersionMatch = /export const desktopVersion = '(\d+\.\d+\.\d+)'/.exec(files.frontendBuild);
const frontendCoreMatch = /export const bundledCoreVersion = '(\d+\.\d+\.\d+)'/.exec(files.frontendBuild);
if (!frontendVersionMatch || !frontendCoreMatch) throw new Error('frontend/src/build-info.js must define both versions.');

if ((!checkOnly || raw) && !match) {
  console.error(`Invalid release tag "${raw}". Expected vX.Y.Z.`);
  process.exit(1);
}

const taggedVersion = match ? `${match[1]}.${match[2]}.${match[3]}` : '';

const version = checkOnly ? pkg.version : taggedVersion;
if (!/^(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(version)) {
  console.error(`Invalid GUI version "${version}" in package.json. Expected X.Y.Z.`);
  process.exit(1);
}
if (checkOnly && match && version !== taggedVersion) {
  console.error(`Release tag GUI v${taggedVersion} does not match package.json v${version}.`);
  process.exit(1);
}

if (checkOnly) {
  if (bundledCoreMatch[1] !== coreMatch[1] || frontendCoreMatch[1] !== coreMatch[1]) {
    console.error(`Bundled Core versions must match go.mod v${coreMatch[1]}: backend v${bundledCoreMatch[1]}, frontend v${frontendCoreMatch[1]}.`);
    process.exit(1);
  }
  const actual = {
    'package.json': pkg.version,
    'frontend/package.json': frontend.version,
    'frontend/package-lock.json': lock.version,
    'frontend/package-lock.json root package': lock.packages?.['']?.version,
    'wails.json': config.info?.productVersion,
    'internal/config/product.go': productMatch[1],
    'frontend/src/build-info.js': frontendVersionMatch[1],
  };
  const mismatches = Object.entries(actual).filter(([, value]) => value !== version);
  if (mismatches.length) {
    console.error(`GUI version fields must match v${version}: ${mismatches.map(([name, value]) => `${name}=${value}`).join(', ')}`);
    process.exit(1);
  }
  if (match) {
    console.log(`version=${version}`);
    console.log(`core_version=${coreMatch[1]}`);
    console.log(`tag=${raw}`);
  } else {
    console.log(`Desktop version fields match: v${version} (bundled Core v${coreMatch[1]}).`);
  }
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
  writeFile(paths.product, files.product.replace(productMatch[0], `var ProductVersion = "${version}"`).replace(bundledCoreMatch[0], `var CoreVersion = "${coreMatch[1]}"`), 'utf8'),
  writeFile(paths.frontendBuild, files.frontendBuild.replace(frontendVersionMatch[0], `export const desktopVersion = '${version}'`).replace(frontendCoreMatch[0], `export const bundledCoreVersion = '${coreMatch[1]}'`), 'utf8'),
]);

// The output format can be appended directly to GitHub Actions' GITHUB_OUTPUT.
console.log(`version=${version}`);
console.log(`core_version=${coreMatch[1]}`);
console.log(`tag=${raw}`);
