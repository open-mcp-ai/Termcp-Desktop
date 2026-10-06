import { dot, esc, icon } from '../../ui/render.js';
import { coreMode, shellCount, state } from '../../state.js';
import { button, header } from '../../ui/components.js';
import { getLanguage } from '../../i18n/index.js';
import { serviceDetailsPage } from './service.js';

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
  return `<section class="settings-card core-settings-card" id="core-settings"><header><div><span>LOCAL CORE</span><h2>Core management</h2></div><div class="core-settings-actions">${button(`${icon('plus', 14)}New connection`, 'new-connection')}${runtimeAction}</div></header><div class="core-settings-runtime"><div class="core-runtime-state">${dot(status.running ? 'running' : 'error')}<span><b>${status.running ? 'Running' : 'Stopped'}</b><small>${coreMode()}</small></span></div><dl><div><dt>Local service address</dt><dd><code>${esc(status.address || 'http://127.0.0.1:18765')}</code></dd></div><div><dt>Core version</dt><dd><code>${esc(status.version || 'Unavailable')}</code></dd></div><div><dt>System service</dt><dd><button class="setting-link" data-action="open-service-details" aria-label="Open service details">${state.service.installed ? 'Registered' : 'Not registered'} ${icon('chevron', 14)}</button></dd></div></dl></div><div class="core-settings-metrics">${metrics.map(([label, value]) => `<article><strong>${value}</strong><span>${label}</span></article>`).join('')}</div></section>`;
}

export function settingsPage() {
  if (state.settingsView === 'service') return serviceDetailsPage();
  const base = state.data.core.address || 'http://127.0.0.1:18765';
  const mcp = JSON.stringify({ mcpServers: { termcp: { url: `${base}/stream` } } }, null, 2);
  const appearance = state.appearance;
  const fonts = [...new Set(['system-ui', ...state.systemFonts.items])];
  const fontStatus = state.systemFonts.loading ? 'Reading installed fonts…' : state.systemFonts.error ? 'Installed fonts could not be read. Enter a font name directly.' : `Found ${Math.max(0, fonts.length - 1)}  installed fonts`;
  const appearanceCard = `<section class="settings-card appearance-card"><header><div><span>APPEARANCE</span><h2>Interface appearance</h2></div><small>Applies immediately</small></header><div class="setting-row"><label><b>Theme</b><small>Light and dark use the same neutral colour scale.</small></label><select data-appearance="theme"><option value="light" ${appearance.theme === 'light' ? 'selected' : ''}>Light</option><option value="dark" ${appearance.theme === 'dark' ? 'selected' : ''}>Dark</option></select></div><div class="setting-row"><label><b>Font size</b><small>Uses an explicit pixel value and also applies it to terminals.</small></label><div class="font-size-control"><select data-appearance="fontSize">${Array.from({ length: 9 }, (_, index) => index + 12).map(size => `<option value="${size}" ${appearance.fontSize === size ? 'selected' : ''}>${size}</option>`).join('')}</select><span>px</span></div></div><div class="setting-row font-setting"><label><b>System font</b><small>${fontStatus}</small></label><input list="system-font-families" value="${esc(appearance.fontFamily)}" data-appearance="fontFamily" data-font-family placeholder="Search or enter a font name"><datalist id="system-font-families">${fonts.map(font => `<option value="${esc(font)}">`).join('')}</datalist></div><div class="font-preview" data-font-preview><span>Aa 文</span><p>Termcp keeps the local Core, SSH sessions and file management in one clear workflow.</p><code>debian@host:~$ termcp</code></div></section>`;
  const applicationCard = `<section class="settings-card"><header><div><span>APPLICATION</span><h2>Application settings</h2></div></header><div class="setting-row"><label><b>Interface language</b><small>Updates the application interface and system tray.</small></label><select data-language data-i18n-ignore><option value="zh-CN" ${getLanguage() === 'zh-CN' ? 'selected' : ''}>简体中文</option><option value="en" ${getLanguage() === 'en' ? 'selected' : ''}>English</option></select></div><div class="setting-row"><label><b>System service</b><small>Registration, autostart and background operation.</small></label><button class="setting-link" data-action="open-service-details" aria-label="Open service details">${state.service.installed ? 'Registered' : 'Not registered'} ${icon('chevron', 14)}</button></div></section>`;
  const approvalCard = `<section class="settings-card"><header><div><span>AI REVIEW</span><h2>Approval and notifications</h2></div></header><div class="setting-row"><label><b>Global default approval</b><small>New sessions inherit this setting unless their connection overrides it. Approval is off by default.</small></label><select data-global-approval><option value="off" ${state.approvalSettings.defaultEnabled ? '' : 'selected'}>Off</option><option value="on" ${state.approvalSettings.defaultEnabled ? 'selected' : ''}>On</option></select></div><div class="setting-row"><label><b>Desktop notifications</b><small>Show a system notification when an AI action needs approval. Pending requests always appear inside the app.</small></label><select data-approval-notifications><option value="off" ${state.approvalSettings.desktopNotifications ? '' : 'selected'}>Off</option><option value="on" ${state.approvalSettings.desktopNotifications ? 'selected' : ''}>On</option></select></div><div class="setting-row"><label><b>Pending approvals</b><small>Review Shell commands, file changes and port forwards.</small></label><button class="setting-link" data-section="approvals">Open review queue ${icon('chevron', 14)}</button></div></section>`;
  const integrationCard = `<section class="settings-card developer-card"><header><div><span>INTEGRATIONS</span><h2>MCP access</h2></div><small>Streamable HTTP</small></header><div class="mcp-endpoint"><div><span>Local service address</span><code>${esc(base)}/stream</code></div><button data-copy="${esc(`${base}/stream`)}">Copy address</button></div><pre>${esc(mcp)}</pre><button class="button" data-copy="${esc(mcp)}">Copy MCP config</button></section>`;
  return `${header('Settings', 'TERMCP DESKTOP', button(`${icon('refresh', 14)}Refresh`, 'refresh'))}<div class="settings-grid">${coreSettingsCard()}${appearanceCard}${approvalCard}${applicationCard}${integrationCard}</div>`;
}
