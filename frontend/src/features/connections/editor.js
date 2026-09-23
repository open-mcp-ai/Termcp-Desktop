import { esc, icon } from '../../ui/render.js';

function authSelector(scope, value) {
  return `<div class="auth-method" role="group" aria-label="Authentication"><input type="hidden" name="${scope === 'main' ? 'auth' : 'jump_auth'}" value="${esc(value)}" data-auth-value="${scope}"><button type="button" class="${value === 'password' ? 'active' : ''}" data-auth-mode="password" data-auth-scope="${scope}" aria-pressed="${value === 'password'}">Password login</button><button type="button" class="${value === 'key' ? 'active' : ''}" data-auth-mode="key" data-auth-scope="${scope}" aria-pressed="${value === 'key'}">Private key login</button></div>`;
}

function secretInput(name, value, placeholder = '', required = false) {
  return `<div class="secret-input"><input type="password" name="${name}" value="${esc(value)}" placeholder="${esc(placeholder)}" autocomplete="new-password" ${required ? 'required' : ''}><button type="button" data-toggle-secret aria-label="Show password" title="Show password">${icon('eye', 15)}</button></div>`;
}

export function connectionEditor(profile, loading = false) {
  const disabled = loading ? 'disabled' : '';
  return `<div class="connection-editor-body ${loading ? 'loading' : ''}">
    <textarea name="nested_jump_config" hidden data-i18n-ignore>${esc(profile.nestedJumpConfig || '')}</textarea>
    <nav class="connection-editor-tabs" role="tablist" aria-label="Connection settings sections">
      <button type="button" class="active" data-connection-tab="basic" role="tab" aria-selected="true">General</button>
      <button type="button" data-connection-tab="jump" role="tab" aria-selected="false">Jump host</button>
      <button type="button" data-connection-tab="proxy" role="tab" aria-selected="false">Proxy</button>
      <button type="button" data-connection-tab="advanced" role="tab" aria-selected="false">Advanced</button>
    </nav>
    <section class="connection-editor-panel" data-connection-panel="basic" role="tabpanel">
      <div class="connection-form-grid">
        <label>Configuration name<input name="name" value="${esc(profile.name)}" pattern="[A-Za-z0-9][A-Za-z0-9_-]{0,63}" maxlength="64" placeholder="development-server" required ${disabled} ${profile.name ? '' : 'autofocus'}><small>Used in sessions and resource lists. Use letters, numbers, underscores and hyphens only.</small></label>
        <label>Notes<input name="description" value="${esc(profile.description)}" placeholder="For example: Development web server" ${disabled}></label>
        <label class="span-2">Host<input name="host" value="${esc(profile.host)}" placeholder="192.168.1.10 or ssh.example.com" autocomplete="off" required ${disabled}></label>
        <label>Port<input name="port" type="number" min="1" max="65535" value="${esc(profile.port || 22)}" required ${disabled}></label>
        <label>Username<input name="user" value="${esc(profile.user)}" placeholder="root" autocomplete="username" required ${disabled}></label>
      </div>
      <div class="connection-section-head"><div><b>Authentication</b><small>Choose the SSH authentication method used by this server.</small></div>${authSelector('main', profile.auth)}</div>
      <div class="auth-panel" data-auth-panel="main:password" ${profile.auth === 'password' ? '' : 'hidden'}>
        <label>Password${secretInput('password', profile.password, 'Enter the SSH password', profile.auth === 'password')}</label>
      </div>
      <div class="auth-panel key-panel" data-auth-panel="main:key" ${profile.auth === 'key' ? '' : 'hidden'}>
        <label>Private key<textarea name="private_key" rows="7" spellcheck="false" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" ${profile.auth === 'key' ? 'required' : ''}>${esc(profile.privateKey)}</textarea><small>Paste an OpenSSH or PEM private key. The key is stored in the connection profile under ~/.termcp.</small></label>
        <label>Key passphrase (optional)${secretInput('key_passphrase', profile.keyPassphrase, 'Leave blank when the key is not encrypted')}</label>
      </div>
    </section>
    <section class="connection-editor-panel" data-connection-panel="jump" role="tabpanel" hidden>
      <div class="connection-option-card">
        <label class="check-row"><input type="checkbox" name="jump_enabled" value="true" data-jump-enabled ${profile.jumpEnabled ? 'checked' : ''} ${disabled}><span><b>Connect through a jump host</b><small>Sign in to the bastion first, then access the target host from it.</small></span></label>
      </div>
      <div data-jump-fields ${profile.jumpEnabled ? '' : 'hidden'}>
        ${profile.nestedJumpCount ? `<div class="connection-preserved-note">Detected ${profile.nestedJumpCount} additional jump hosts. This form edits the first hop; the remaining chain is preserved when saved.</div>` : ''}
        <div class="connection-form-grid">
          <label class="span-2">Jump host address<input name="jump_host" value="${esc(profile.jumpHost)}" placeholder="bastion.example.com" ${profile.jumpEnabled ? 'required' : ''} ${disabled}></label>
          <label>Port<input name="jump_port" type="number" min="1" max="65535" value="${esc(profile.jumpPort || 22)}" ${disabled}></label>
          <label>Username<input name="jump_user" value="${esc(profile.jumpUser)}" placeholder="ops" ${profile.jumpEnabled ? 'required' : ''} ${disabled}></label>
        </div>
        <div class="connection-section-head"><div><b>Jump host authentication</b><small>The target and jump host can use different authentication methods.</small></div>${authSelector('jump', profile.jumpAuth)}</div>
        <div class="auth-panel" data-auth-panel="jump:password" ${profile.jumpAuth === 'password' ? '' : 'hidden'}><label>Jump host password${secretInput('jump_password', profile.jumpPassword, 'Enter the jump host password', profile.jumpEnabled && profile.jumpAuth === 'password')}</label></div>
        <div class="auth-panel key-panel" data-auth-panel="jump:key" ${profile.jumpAuth === 'key' ? '' : 'hidden'}><label>Jump host private key<textarea name="jump_private_key" rows="6" spellcheck="false" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" ${profile.jumpEnabled && profile.jumpAuth === 'key' ? 'required' : ''}>${esc(profile.jumpPrivateKey)}</textarea></label><label>Key passphrase (optional)${secretInput('jump_key_passphrase', profile.jumpKeyPassphrase, 'Leave blank when the key is not encrypted')}</label></div>
      </div>
    </section>
    <section class="connection-editor-panel" data-connection-panel="proxy" role="tabpanel" hidden>
      <div class="connection-panel-intro"><b>SOCKS5 proxy</b><p>Route SSH traffic through a SOCKS5 proxy reachable from this computer. Leave blank when no proxy is needed.</p></div>
      <label>Proxy URL<input name="proxy" value="${esc(profile.proxy)}" placeholder="socks5://user:password@127.0.0.1:1080" ${disabled}><small>Supports socks5://host:port and URLs with a username and password.</small></label>
      <div class="connection-option-card jump-proxy-option" data-jump-proxy-option ${profile.jumpEnabled ? '' : 'hidden'}>
        <label>Jump host proxy URL<input name="jump_proxy" value="${esc(profile.jumpProxy)}" placeholder="socks5://127.0.0.1:1080" ${disabled}><small>Used only when connecting to the jump host.</small></label>
      </div>
    </section>
    <section class="connection-editor-panel" data-connection-panel="advanced" role="tabpanel" hidden>
      <div class="connection-form-grid">
        <label>Default shell<input name="default_shell" value="${esc(profile.defaultShell)}" placeholder="Auto-detect, for example /bin/zsh" ${disabled}></label>
        <label>Default terminal mode<select name="default_mode" ${disabled}><option value="pty" ${profile.defaultMode === 'pty' ? 'selected' : ''}>PTY interactive terminal</option><option value="pipe" ${profile.defaultMode === 'pipe' ? 'selected' : ''}>Pipe non-interactive</option></select></label>
        <label>Connection timeout<input name="dial_timeout_seconds" type="number" min="1" max="600" value="${esc(profile.dialTimeout || 30)}" ${disabled}><small>Seconds</small></label>
      </div>
      <div class="connection-option-card">
        <label class="check-row"><input type="checkbox" name="trust_unknown_host" value="true" data-trust-toggle="main" ${profile.trustUnknownHost ? 'checked' : ''} ${disabled}><span><b>Trust unknown host keys automatically</b><small>When disabled, known_hosts content is required for strict host verification.</small></span></label>
        <label data-known-hosts="main" ${profile.trustUnknownHost ? 'hidden' : ''}>known_hosts content<textarea name="known_hosts" rows="5" spellcheck="false" placeholder="ssh.example.com ssh-ed25519 AAAA…" ${profile.trustUnknownHost ? '' : 'required'}>${esc(profile.knownHosts)}</textarea></label>
      </div>
      <div class="connection-option-card" data-jump-advanced ${profile.jumpEnabled ? '' : 'hidden'}>
        <b>Jump host advanced settings</b>
        <div class="connection-form-grid"><label>Connection timeout<input name="jump_dial_timeout_seconds" type="number" min="1" max="600" value="${esc(profile.jumpDialTimeout || 30)}" ${disabled}><small>Seconds</small></label></div>
        <label class="check-row"><input type="checkbox" name="jump_trust_unknown_host" value="true" data-trust-toggle="jump" ${profile.jumpTrustUnknownHost ? 'checked' : ''} ${disabled}><span><b>Trust unknown jump host keys automatically</b><small>When disabled, verify the jump host with the known_hosts content below.</small></span></label>
        <label data-known-hosts="jump" ${profile.jumpTrustUnknownHost ? 'hidden' : ''}>Jump host known_hosts content<textarea name="jump_known_hosts" rows="4" spellcheck="false" ${profile.jumpEnabled && !profile.jumpTrustUnknownHost ? 'required' : ''}>${esc(profile.jumpKnownHosts)}</textarea></label>
      </div>
    </section>
  </div>`;
}

export function setConnectionAuthMode(form, scope, mode) {
  const value = form.querySelector(`[data-auth-value="${scope}"]`);
  if (value) value.value = mode;
  form.querySelectorAll(`[data-auth-scope="${scope}"]`).forEach(button => {
    const active = button.dataset.authMode === mode;
    button.classList.toggle('active', active); button.setAttribute('aria-pressed', String(active));
  });
  form.querySelectorAll(`[data-auth-panel^="${scope}:"]`).forEach(panel => { panel.hidden = panel.dataset.authPanel !== `${scope}:${mode}`; });
  const enabled = scope === 'main' || Boolean(form.querySelector('[data-jump-enabled]')?.checked);
  const password = form.elements.namedItem(scope === 'main' ? 'password' : 'jump_password');
  const privateKey = form.elements.namedItem(scope === 'main' ? 'private_key' : 'jump_private_key');
  if (password) password.required = enabled && mode === 'password';
  if (privateKey) privateKey.required = enabled && mode === 'key';
}

export function activateConnectionTab(form, tab) {
  form.querySelectorAll('[data-connection-tab]').forEach(button => { const active = button.dataset.connectionTab === tab; button.classList.toggle('active', active); button.setAttribute('aria-selected', String(active)); });
  form.querySelectorAll('[data-connection-panel]').forEach(panel => { panel.hidden = panel.dataset.connectionPanel !== tab; });
}

export function validateConnectionForm(form) {
  const invalid = form.querySelector('input:invalid, select:invalid, textarea:invalid');
  if (!invalid) return true;
  const panel = invalid.closest('[data-connection-panel]');
  if (panel) activateConnectionTab(form, panel.dataset.connectionPanel);
  invalid.reportValidity();
  return false;
}

export function setJumpEnabled(form, enabled) {
  const fields = form.querySelector('[data-jump-fields]'); if (fields) fields.hidden = !enabled;
  form.querySelectorAll('[data-jump-proxy-option], [data-jump-advanced]').forEach(element => { element.hidden = !enabled; });
  const host = form.elements.namedItem('jump_host'); const user = form.elements.namedItem('jump_user');
  if (host) host.required = enabled; if (user) user.required = enabled;
  const mode = form.elements.namedItem('jump_auth')?.value || 'password';
  setConnectionAuthMode(form, 'jump', mode);
  const trust = form.querySelector('[data-trust-toggle="jump"]')?.checked ?? true;
  setTrustUnknownHost(form, 'jump', !enabled || trust);
}

export function setTrustUnknownHost(form, scope, trusted) {
  const field = form.querySelector(`[data-known-hosts="${scope}"]`); if (!field) return;
  field.hidden = trusted;
  const textarea = form.elements.namedItem(scope === 'main' ? 'known_hosts' : 'jump_known_hosts');
  const enabled = scope === 'main' || Boolean(form.querySelector('[data-jump-enabled]')?.checked);
  if (textarea) textarea.required = enabled && !trusted;
}
