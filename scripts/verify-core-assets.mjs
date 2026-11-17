import { spawnSync } from 'node:child_process';

const targetOS = process.env.GOOS || process.platform;
const tags = ['no_webui'];
if (targetOS === 'linux') tags.push('webkit2_41');

const result = spawnSync('go', [
  'list',
  `-tags=${tags.join(',')}`,
  '-json',
  'github.com/open-mcp-ai/termcp/internal/webui',
], { encoding: 'utf8' });
if (result.error) {
  console.error(result.error.message);
  process.exit(1);
}
if (result.status !== 0) {
  process.stderr.write(result.stderr || 'go list failed\n');
  process.exit(result.status ?? 1);
}

const pkg = JSON.parse(result.stdout);
const embedded = Array.isArray(pkg.EmbedFiles) ? pkg.EmbedFiles : [];
const expected = ['assets/api.md'];
if (JSON.stringify(embedded) !== JSON.stringify(expected)) {
  console.error(`no_webui Core assets = ${JSON.stringify(embedded)}, want ${JSON.stringify(expected)}`);
  process.exit(1);
}
console.log(`No-WebUI Core assets verified: ${embedded.join(', ')}`);
