import { spawnSync } from 'node:child_process';

const command = String(process.argv[2] || '').trim();
if (!['dev', 'build'].includes(command)) {
  console.error('usage: node scripts/wails.mjs dev|build [wails arguments]');
  process.exit(2);
}

const platformIndex = process.argv.findIndex(value => value === '-platform' || value === '--platform');
const explicitPlatform = platformIndex >= 0 ? String(process.argv[platformIndex + 1] || '') : '';
const targetOS = (explicitPlatform.split('/')[0] || process.env.GOOS || process.platform);
const tags = ['no_webui'];
if (targetOS === 'linux') tags.push('webkit2_41');

const args = [
  'run',
  'github.com/wailsapp/wails/v2/cmd/wails@v2.15.0',
  command,
  '-tags',
  tags.join(','),
  ...process.argv.slice(3),
];
const result = spawnSync('go', args, { stdio: 'inherit' });
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
process.exit(result.status ?? 1);
