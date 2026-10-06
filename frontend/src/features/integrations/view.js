import { esc, icon } from '../../ui/render.js';
import { button, header } from '../../ui/components.js';
import { state } from '../../state.js';

function endpoint(label, url) {
  return `<div class="integration-endpoint"><span>${label}</span><code>${esc(url)}</code><button data-copy="${esc(url)}">Copy</button></div>`;
}

function snippet(label, value) {
  return `<div class="integration-snippet"><div><b>${label}</b><button data-copy="${esc(value)}">Copy</button></div><pre>${esc(value)}</pre></div>`;
}

function stdioArgs(base) {
  const args = ['daemon', 'stdio'];
  try {
    const url = new URL(base);
    const host = url.hostname.replace(/^\[|\]$/g, '');
    if (host && !['127.0.0.1', 'localhost', '::1'].includes(host)) args.push('--host', host);
    if (url.port && url.port !== '18765') args.push('--port', url.port);
  } catch { /* The default local address still produces a usable snippet. */ }
  return args;
}

export function integrationPage() {
  const base = String(state.data.core.address || 'http://127.0.0.1:18765').replace(/\/+$/, '');
  const sse = `${base}/sse`;
  const stream = `${base}/stream`;
  const apiDocs = `${base}/api.md`;
  const skillDocs = `${base}/skills.md`;
  const args = stdioArgs(base);
  const clientJSON = type => JSON.stringify({ mcpServers: { termcp: { type, url: type === 'sse' ? sse : stream } } }, null, 2);
  const stdioJSON = JSON.stringify({ mcpServers: { termcp: { command: 'termcp', args } } }, null, 2);
  const installSkill = `DIR=~/.claude/skills && mkdir -p "$DIR/termcp" && curl -fsS ${skillDocs} -o "$DIR/termcp/SKILL.md"`;
  return `<div class="integration-page">
    ${header('API / MCP / SKILLS', 'TERMCP INTEGRATIONS', button(`${icon('collapse', 14)}Back to previous view`, 'close-integrations'))}
    <p class="integration-lead">Copy connection addresses and client configuration for this Core instance.</p>
    <nav class="integration-jump" aria-label="Integration sections"><a href="#integration-mcp">MCP endpoints</a><a href="#integration-skill">Agent docs &amp; skill</a><a href="#integration-clients">Client config</a></nav>

    <section id="integration-mcp" class="integration-section"><h2>1. MCP endpoints</h2><div class="integration-grid">
      <article class="integration-card"><h3>SSE</h3><p>Use the SSE transport. JSON-RPC messages automatically use <code>/message</code>.</p>${endpoint('MCP · /sse', sse)}</article>
      <article class="integration-card"><h3>Streamable HTTP</h3><p>Use the HTTP transport for clients such as Open WebUI.</p>${endpoint('MCP · /stream', stream)}</article>
    </div><p class="integration-note">Both transports expose the same tools. Choose the endpoint that matches your client's transport.</p></section>

    <section id="integration-skill" class="integration-section"><h2>2. Agent docs &amp; skill</h2><div class="integration-grid">
      <article class="integration-card"><h3>Documentation</h3><p>Read the REST and WebSocket reference, or install the curl skill without an MCP client.</p>${endpoint('API reference · api.md', apiDocs)}${endpoint('Agent skill · skills.md', skillDocs)}</article>
      <article class="integration-card"><h3>Install the HTTP skill</h3><p>Save the skill as <code>termcp/SKILL.md</code>. Agents following the shared convention use <code>~/.agents/skills</code>.</p>${snippet('Claude Code', installSkill)}</article>
    </div></section>

    <section id="integration-clients" class="integration-section"><h2>3. Client config</h2><div class="integration-grid">
      <article class="integration-card"><h3>Claude Code CLI</h3>${snippet('SSE', `claude mcp add --transport sse termcp ${sse}`)}${snippet('Streamable HTTP', `claude mcp add --transport http termcp ${stream}`)}${snippet('Local stdio bridge', `claude mcp add termcp -- termcp ${args.join(' ')}`)}</article>
      <article class="integration-card"><h3>JSON · mcpServers</h3>${snippet('SSE', clientJSON('sse'))}${snippet('Streamable HTTP', clientJSON('http'))}${snippet('Local stdio bridge', stdioJSON)}</article>
    </div><p class="integration-note">If Core requires authentication, send the token in an Authorization header or client environment variable. Keep tokens out of URLs.</p></section>

  </div>`;
}
