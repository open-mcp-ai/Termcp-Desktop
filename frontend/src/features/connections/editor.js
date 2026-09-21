import { esc, icon } from '../../ui/render.js';

function authSelector(scope, value) {
  return `<div class="auth-method" role="group" aria-label="认证方式"><input type="hidden" name="${scope === 'main' ? 'auth' : 'jump_auth'}" value="${esc(value)}" data-auth-value="${scope}"><button type="button" class="${value === 'password' ? 'active' : ''}" data-auth-mode="password" data-auth-scope="${scope}" aria-pressed="${value === 'password'}">密码登录</button><button type="button" class="${value === 'key' ? 'active' : ''}" data-auth-mode="key" data-auth-scope="${scope}" aria-pressed="${value === 'key'}">私钥登录</button></div>`;
}

function secretInput(name, value, placeholder = '', required = false) {
  return `<div class="secret-input"><input type="password" name="${name}" value="${esc(value)}" placeholder="${esc(placeholder)}" autocomplete="new-password" ${required ? 'required' : ''}><button type="button" data-toggle-secret aria-label="显示密码" title="显示密码">${icon('eye', 15)}</button></div>`;
}

export function connectionEditor(profile, loading = false) {
  const disabled = loading ? 'disabled' : '';
  return `<div class="connection-editor-body ${loading ? 'loading' : ''}">
    <textarea name="nested_jump_config" hidden data-i18n-ignore>${esc(profile.nestedJumpConfig || '')}</textarea>
    <nav class="connection-editor-tabs" role="tablist" aria-label="连接配置分区">
      <button type="button" class="active" data-connection-tab="basic" role="tab" aria-selected="true">基本信息</button>
      <button type="button" data-connection-tab="jump" role="tab" aria-selected="false">跳板机</button>
      <button type="button" data-connection-tab="proxy" role="tab" aria-selected="false">代理</button>
      <button type="button" data-connection-tab="advanced" role="tab" aria-selected="false">高级</button>
    </nav>
    <section class="connection-editor-panel" data-connection-panel="basic" role="tabpanel">
      <div class="connection-form-grid">
        <label>配置名称<input name="name" value="${esc(profile.name)}" pattern="[A-Za-z0-9][A-Za-z0-9_-]{0,63}" maxlength="64" placeholder="production-server" required ${disabled} ${profile.name ? '' : 'autofocus'}><small>用于会话和资源列表，只能包含字母、数字、下划线和连字符。</small></label>
        <label>备注<input name="description" value="${esc(profile.description)}" placeholder="例如：生产环境 Web 服务器" ${disabled}></label>
        <label class="span-2">主机地址<input name="host" value="${esc(profile.host)}" placeholder="192.168.1.10 或 ssh.example.com" autocomplete="off" required ${disabled}></label>
        <label>端口<input name="port" type="number" min="1" max="65535" value="${esc(profile.port || 22)}" required ${disabled}></label>
        <label>用户名<input name="user" value="${esc(profile.user)}" placeholder="root" autocomplete="username" required ${disabled}></label>
      </div>
      <div class="connection-section-head"><div><b>认证方式</b><small>选择与服务器匹配的 SSH 登录方式。</small></div>${authSelector('main', profile.auth)}</div>
      <div class="auth-panel" data-auth-panel="main:password" ${profile.auth === 'password' ? '' : 'hidden'}>
        <label>密码${secretInput('password', profile.password, '输入 SSH 登录密码', profile.auth === 'password')}</label>
      </div>
      <div class="auth-panel key-panel" data-auth-panel="main:key" ${profile.auth === 'key' ? '' : 'hidden'}>
        <label>私钥内容<textarea name="private_key" rows="7" spellcheck="false" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" ${profile.auth === 'key' ? 'required' : ''}>${esc(profile.privateKey)}</textarea><small>粘贴 OpenSSH 或 PEM 私钥正文。密钥保存在 ~/.termcp 的连接配置中。</small></label>
        <label>私钥口令（可选）${secretInput('key_passphrase', profile.keyPassphrase, '私钥未加密时留空')}</label>
      </div>
    </section>
    <section class="connection-editor-panel" data-connection-panel="jump" role="tabpanel" hidden>
      <div class="connection-option-card">
        <label class="check-row"><input type="checkbox" name="jump_enabled" value="true" data-jump-enabled ${profile.jumpEnabled ? 'checked' : ''} ${disabled}><span><b>通过跳板机连接</b><small>先登录堡垒机，再从堡垒机访问目标主机。</small></span></label>
      </div>
      <div data-jump-fields ${profile.jumpEnabled ? '' : 'hidden'}>
        ${profile.nestedJumpCount ? `<div class="connection-preserved-note">已检测到额外 ${profile.nestedJumpCount} 层跳板机。当前表单编辑第一跳，其余链路会在保存时原样保留。</div>` : ''}
        <div class="connection-form-grid">
          <label class="span-2">跳板机地址<input name="jump_host" value="${esc(profile.jumpHost)}" placeholder="bastion.example.com" ${profile.jumpEnabled ? 'required' : ''} ${disabled}></label>
          <label>端口<input name="jump_port" type="number" min="1" max="65535" value="${esc(profile.jumpPort || 22)}" ${disabled}></label>
          <label>用户名<input name="jump_user" value="${esc(profile.jumpUser)}" placeholder="ops" ${profile.jumpEnabled ? 'required' : ''} ${disabled}></label>
        </div>
        <div class="connection-section-head"><div><b>跳板机认证</b><small>目标主机和跳板机可以使用不同的认证方式。</small></div>${authSelector('jump', profile.jumpAuth)}</div>
        <div class="auth-panel" data-auth-panel="jump:password" ${profile.jumpAuth === 'password' ? '' : 'hidden'}><label>跳板机密码${secretInput('jump_password', profile.jumpPassword, '输入跳板机登录密码', profile.jumpEnabled && profile.jumpAuth === 'password')}</label></div>
        <div class="auth-panel key-panel" data-auth-panel="jump:key" ${profile.jumpAuth === 'key' ? '' : 'hidden'}><label>跳板机私钥内容<textarea name="jump_private_key" rows="6" spellcheck="false" placeholder="-----BEGIN OPENSSH PRIVATE KEY-----" ${profile.jumpEnabled && profile.jumpAuth === 'key' ? 'required' : ''}>${esc(profile.jumpPrivateKey)}</textarea></label><label>私钥口令（可选）${secretInput('jump_key_passphrase', profile.jumpKeyPassphrase, '私钥未加密时留空')}</label></div>
      </div>
    </section>
    <section class="connection-editor-panel" data-connection-panel="proxy" role="tabpanel" hidden>
      <div class="connection-panel-intro"><b>SOCKS5 代理</b><p>SSH 流量通过本机可访问的 SOCKS5 代理转发。无需代理时保持为空。</p></div>
      <label>代理地址<input name="proxy" value="${esc(profile.proxy)}" placeholder="socks5://user:password@127.0.0.1:1080" ${disabled}><small>支持 socks5://host:port 和带用户名、密码的 URL。</small></label>
      <div class="connection-option-card jump-proxy-option" data-jump-proxy-option ${profile.jumpEnabled ? '' : 'hidden'}>
        <label>跳板机代理地址<input name="jump_proxy" value="${esc(profile.jumpProxy)}" placeholder="socks5://127.0.0.1:1080" ${disabled}><small>仅用于跳板机本身的连接。</small></label>
      </div>
    </section>
    <section class="connection-editor-panel" data-connection-panel="advanced" role="tabpanel" hidden>
      <div class="connection-form-grid">
        <label>默认 Shell<input name="default_shell" value="${esc(profile.defaultShell)}" placeholder="自动检测，例如 /bin/zsh" ${disabled}></label>
        <label>默认终端模式<select name="default_mode" ${disabled}><option value="pty" ${profile.defaultMode === 'pty' ? 'selected' : ''}>PTY 交互终端</option><option value="pipe" ${profile.defaultMode === 'pipe' ? 'selected' : ''}>Pipe 非交互</option></select></label>
        <label>连接超时<input name="dial_timeout_seconds" type="number" min="1" max="600" value="${esc(profile.dialTimeout || 30)}" ${disabled}><small>单位：秒</small></label>
      </div>
      <div class="connection-option-card">
        <label class="check-row"><input type="checkbox" name="trust_unknown_host" value="true" data-trust-toggle="main" ${profile.trustUnknownHost ? 'checked' : ''} ${disabled}><span><b>自动信任未知主机密钥</b><small>关闭后必须提供 known_hosts 内容，适合需要严格主机校验的环境。</small></span></label>
        <label data-known-hosts="main" ${profile.trustUnknownHost ? 'hidden' : ''}>known_hosts 内容<textarea name="known_hosts" rows="5" spellcheck="false" placeholder="ssh.example.com ssh-ed25519 AAAA…" ${profile.trustUnknownHost ? '' : 'required'}>${esc(profile.knownHosts)}</textarea></label>
      </div>
      <div class="connection-option-card" data-jump-advanced ${profile.jumpEnabled ? '' : 'hidden'}>
        <b>跳板机高级设置</b>
        <div class="connection-form-grid"><label>连接超时<input name="jump_dial_timeout_seconds" type="number" min="1" max="600" value="${esc(profile.jumpDialTimeout || 30)}" ${disabled}><small>单位：秒</small></label></div>
        <label class="check-row"><input type="checkbox" name="jump_trust_unknown_host" value="true" data-trust-toggle="jump" ${profile.jumpTrustUnknownHost ? 'checked' : ''} ${disabled}><span><b>自动信任跳板机主机密钥</b><small>关闭后使用下方 known_hosts 校验跳板机。</small></span></label>
        <label data-known-hosts="jump" ${profile.jumpTrustUnknownHost ? 'hidden' : ''}>跳板机 known_hosts 内容<textarea name="jump_known_hosts" rows="4" spellcheck="false" ${profile.jumpEnabled && !profile.jumpTrustUnknownHost ? 'required' : ''}>${esc(profile.jumpKnownHosts)}</textarea></label>
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

