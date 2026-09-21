import { dot, esc, icon } from '../../ui/render.js';
import { state } from '../../state.js';
import { button, header } from '../../ui/components.js';

export function servicePage() {
  const service = state.service;
  const registered = service.installed;
  const actions = !service.supported ? '' : registered
    ? `${service.running ? button('Stop', 'stop-core') : button('Start', 'start-core', 'primary')}${button(`${icon('refresh', 14)}Restart`, 'restart-core')}${button('Uninstall service', 'uninstall-service', 'danger')}`
    : `${button('Register service', 'install-service')}${button(`${icon('plus', 14)}Register and enable autostart`, 'install-service-autostart', 'primary')}`;
  const platformNames = { darwin: 'macOS · LaunchAgent', linux: 'Linux · systemd --user', windows: 'Windows · Service Control Manager' };
  const platform = platformNames[service.platform] || service.platform || 'Current platform';
  const supportNotice = service.supported ? '' : '<div class="error-banner">System service management is unavailable on this platform.</div>';
  const autostartScope = service.platform === 'windows' ? 'Starts with the system' : 'Starts when this user signs in';
  return `${header('Core system service', 'LOCAL SERVICE', actions)}${supportNotice}<div class="service-metrics"><article><span>Registration</span><b>${registered ? 'Registered' : 'Not registered'}</b><small>${esc(platform)}</small></article><article><span>Process</span><b>${dot(service.running ? 'running' : 'error')}${service.running ? 'Running' : 'Stopped'}</b><small>${service.pid ? `PID ${service.pid}` : 'No service process'}</small></article><article><span>Autostart</span><b>${service.autostart ? 'Enabled' : 'Disabled'}</b><small>${autostartScope}</small></article></div><section class="panel service-control"><div class="service-control-main"><span class="service-symbol">${icon('service', 25)}</span><div><b>Local termcp Core</b><small>Listens on 127.0.0.1:18765 and runs from the same Termcp executable.</small></div>${registered ? `<label class="switch"><input type="checkbox" data-service-autostart ${service.autostart ? 'checked' : ''}><span></span><em>Autostart</em></label>` : ''}</div><dl><div><dt>Service ID</dt><dd><code>${esc(service.label || '—')}</code></dd></div><div><dt>Manager</dt><dd>${esc(service.description || platform)}</dd></div><div><dt>Data directory</dt><dd><code>${esc(service.data_dir || '~/.termcp')}</code></dd></div><div><dt>Definition</dt><dd><code>${esc(service.definition || '—')}</code></dd></div><div><dt>Executable</dt><dd><code>${esc(service.executable || '—')}</code></dd></div><div><dt>Logs</dt><dd><code>${esc(service.log_path || (service.platform === 'windows' ? 'Windows Event Log' : '—'))}</code></dd></div></dl></section><div class="service-note"><b>Local management boundary</b><p>Termcp manages only the local Core. After registration, closing the desktop window does not stop Core. Uninstalling the service switches back to the in-app process. SSH hosts remain connection resources managed by the local Core.</p></div>`;
}
