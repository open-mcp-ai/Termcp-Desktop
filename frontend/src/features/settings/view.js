import { dot, esc, icon } from '../../ui/render.js';
import { coreMode, shellCount, state } from '../../state.js';
import { button, header } from '../../ui/components.js';
import { getLanguage } from '../../i18n/index.js';

function coreSettingsCard() {
  const status = state.data.core;
  const runtimeAction = status.running
    ? `${button('Stop Core', 'stop-core')}${button(`${icon('refresh', 14)}Restart Core`, 'restart-core', 'primary')}`
    : button('Start Core', 'start-core', 'primary');
  const metrics = [
    ['Connections', state.data.connections.length],
    ['Active sessions', state.data.sessions.length],
    ['Running shells', shellCount()],
    ['Port forwards', state.data.forwards.length],
    ['History entries', state.data.history.length],
  ];
  return `<section class="settings-card core-settings-card" id="core-settings"><header><div><span>LOCAL CORE</span><h2>Core management</h2></div><div class="core-settings-actions">${button(`${icon('plus', 14)}New connection`, 'new-connection')}${runtimeAction}</div></header><div class="core-settings-runtime"><div class="core-runtime-state">${dot(status.running ? 'running' : 'error')}<span><b>${status.running ? 'Running' : 'Stopped'}</b><small>${coreMode()}</small></span></div><dl><div><dt>Local service address</dt><dd><code>${esc(status.address || 'http://127.0.0.1:18765')}</code></dd></div><div><dt>Run mode</dt><dd>${coreMode()}</dd></div><div><dt>Event channel</dt><dd>${esc(state.wsStatus)}</dd></div><div><dt>System service</dt><dd><button class="setting-link" data-section="service">${state.service.installed ? 'Registered' : 'Not registered'} ${icon('chevron', 14)}</button></dd></div></dl></div><div class="core-settings-metrics">${metrics.map(([label, value]) => `<article><strong>${value}</strong><span>${label}</span></article>`).join('')}</div></section>`;
}

export function settingsPage() {
  const base = state.data.core.address || 'http://127.0.0.1:18765';
  const mcp = JSON.stringify({ mcpServers: { termcp: { url: `${base}/stream` } } }, null, 2);
  const appearance = state.appearance;
  const fonts = [...new Set(['system-ui', ...state.systemFonts.items])];
  const fontStatus = state.systemFonts.loading ? 'Reading installed fonts…' : state.systemFonts.error ? 'Installed fonts could not be read. Enter a font name directly.' : `Found ${Math.max(0, fonts.length - 1)}  installed fonts`;
  const appearanceCard = `<section class="settings-card appearance-card"><header><div><span>APPEARANCE</span><h2>Interface appearance</h2></div><small>Applies immediately</small></header><div class="setting-row"><label><b>Theme</b><small>Light and dark use the same neutral colour scale.</small></label><select data-appearance="theme"><option value="light" ${appearance.theme === 'light' ? 'selected' : ''}>Light</option><option value="dark" ${appearance.theme === 'dark' ? 'selected' : ''}>Dark</option></select></div><div class="setting-row"><label><b>Font size</b><small>Uses an explicit pixel value and also applies it to terminals.</small></label><div class="font-size-control"><select data-appearance="fontSize">${Array.from({ length: 9 }, (_, index) => index + 12).map(size => `<option value="${size}" ${appearance.fontSize === size ? 'selected' : ''}>${size}</option>`).join('')}</select><span>px</span></div></div><div class="setting-row font-setting"><label><b>System font</b><small>${fontStatus}</small></label><input list="system-font-families" value="${esc(appearance.fontFamily)}" data-appearance="fontFamily" data-font-family placeholder="Search or enter a font name"><datalist id="system-font-families">${fonts.map(font => `<option value="${esc(font)}">`).join('')}</datalist></div><div class="font-preview" data-font-preview><span>Aa 文</span><p>Termcp keeps the local Core, SSH sessions and file management in one clear workflow.</p><code>debian@host:~$ termcp</code></div></section>`;
  const applicationCard = `<section class="settings-card"><header><div><span>APPLICATION</span><h2>Application settings</h2></div></header><div class="setting-row"><label><b>Interface language</b><small>Updates the application interface and system tray.</small></label><select data-language data-i18n-ignore><option value="zh-CN" ${getLanguage() === 'zh-CN' ? 'selected' : ''}>简体中文</option><option value="en" ${getLanguage() === 'en' ? 'selected' : ''}>English</option></select></div><div class="setting-row"><label><b>Core runtime</b><small>Runtime mode for the termcp Core on this computer.</small></label><em>${coreMode()}</em></div><div class="setting-row"><label><b>System service</b><small>Registration, autostart and background operation.</small></label><button class="setting-link" data-section="service">${state.service.installed ? 'Registered' : 'Not registered'} ${icon('chevron', 14)}</button></div><div class="setting-row"><label><b>Terminal event channel</b><small>Terminal output, input and resize share one WebSocket.</small></label><em>${esc(state.wsStatus)}</em></div><div class="setting-row"><label><b>Rendering engine</b><small>Wails system WebView.</small></label><em>Native WebView</em></div></section>`;
  const integrationCard = `<section class="settings-card developer-card"><header><div><span>INTEGRATIONS</span><h2>MCP access</h2></div><small>Streamable HTTP</small></header><div class="mcp-endpoint"><div><span>Local service address</span><code>${esc(base)}/stream</code></div><button data-copy="${esc(`${base}/stream`)}">Copy address</button></div><pre>${esc(mcp)}</pre><button class="button" data-copy="${esc(mcp)}">Copy MCP config</button></section>`;
  return `${header('Settings', 'TERMCP DESKTOP', button(`${icon('refresh', 14)}Refresh`, 'refresh'))}<div class="settings-grid">${coreSettingsCard()}${appearanceCard}${applicationCard}${integrationCard}</div>`;
}
