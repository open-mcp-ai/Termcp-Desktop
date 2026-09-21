import { readFile, writeFile } from 'node:fs/promises';
import { resolve } from 'node:path';

const raw = String(process.argv[2] || '').trim();
const match = /^v?(\d+)\.(\d+)\.(\d+)$/.exec(raw);

if (!match) {
  console.error(`Invalid release version "${raw}". Expected vX.Y.Z or X.Y.Z.`);
  process.exit(1);
}

const version = `${match[1]}.${match[2]}.${match[3]}`;
const configPath = resolve(process.cwd(), 'wails.json');
const config = JSON.parse(await readFile(configPath, 'utf8'));

config.name = 'Termcp';
config.outputfilename = 'Termcp';
config.info ||= {};
config.info.productName = 'Termcp';
config.info.productVersion = version;

await writeFile(configPath, `${JSON.stringify(config, null, 2)}\n`, 'utf8');

// The output format can be appended directly to GitHub Actions' GITHUB_OUTPUT.
console.log(`version=${version}`);
console.log(`tag=v${version}`);
