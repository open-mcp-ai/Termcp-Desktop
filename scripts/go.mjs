import { spawnSync } from 'node:child_process';

const command = String(process.argv[2] || '').trim();
if (!['test', 'vet'].includes(command)) {
  console.error('usage: node scripts/go.mjs test|vet [go arguments]');
  process.exit(2);
}

const targetOS = process.env.GOOS || process.platform;
const tags = ['no_webui'];
if (targetOS === 'linux') tags.push('webkit2_41');

const args = [command, `-tags=${tags.join(',')}`, ...process.argv.slice(3)];
const result = spawnSync('go', args, { stdio: 'inherit' });
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
process.exit(result.status ?? 1);
