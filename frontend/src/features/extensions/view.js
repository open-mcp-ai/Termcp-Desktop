import { esc, icon } from '../../ui/render.js';
import { button, emptyState, header } from '../../ui/components.js';
import { state } from '../../state.js';
import { emptyExtensionForm } from './model.js';

const STATUS_LABELS = {
  available: 'Available',
  idle: 'Idle · starts on demand',
  probing: 'Starting',
  unavailable: 'Unavailable',
  disabled: 'Disabled',
};

function formatRetry(value) {
  const time = Number(value || 0);
  return time > 0 ? new Date(time).toLocaleString() : '';
}

function statusRow(extension) {
  const status = extension.status || 'unavailable';
  const failure = extension.failure ? `${extension.failure.stage ? `${extension.failure.stage}: ` : ''}${extension.failure.reason || ''}` : '';
  const retry = formatRetry(extension.next_retry_ms);
  return `<span class="extension-status status-${esc(status)}"><i></i>${esc(STATUS_LABELS[status] || status)}</span>${failure ? `<small class="extension-failure" title="${esc(failure)}">${esc(failure)}</small>` : ''}${retry ? `<small>Next retry ${esc(retry)}</small>` : ''}`;
}

function shapeLabel(extension) {
  if (extension.url) return 'URL MCP';
  if (extension.host_count > 1) return 'Multi-host';
  if (extension.host_count === 1) return 'Template';
  if (extension.needs?.length) return 'Needs values';
  if (extension.enabled === false && extension.status === 'disabled') return 'Disabled template';
  if (extension.status === 'unavailable') return 'Template unavailable';
  return 'Template';
}

function targetLabel(extension) {
  if (extension.target) return extension.target;
  if (extension.host_count > 1) return `${extension.host_count} hosts`;
  if (extension.host_count === 1) return '1 host';
  if (extension.needs?.length) return `Waiting for ${extension.needs.join(', ')}`;
  return 'Targets resolved by template';
}

function targetRows(extension) {
  const targets = Array.isArray(extension.targets) ? extension.targets : [];
  return targets.map(target => `<small class="extension-target"><b>${esc(target.target || 'host')}</b> · ${esc(STATUS_LABELS[target.status] || target.status || 'unknown')}${target.next_retry ? ` · retry ${esc(formatRetry(target.next_retry))}` : ''}</small>`).join('');
}

function extensionRow(extension) {
  const name = extension.name || '';
  const enabled = extension.enabled !== false;
  const busy = state.extensionBusy === name;
  const canOpen = extension.status === 'available' && extension.session_id && extension.shell_id;
  const needs = Array.isArray(extension.needs) ? extension.needs : [];
  const templateInvalid = !needs.length && extension.status === 'unavailable' && !extension.url && !extension.target && !extension.host_count && !extension.failure;
  const actions = [
    canOpen ? button(`${icon('terminal', 14)}Open`, 'open-extension', '', `data-name="${esc(name)}" data-session="${esc(extension.session_id)}" data-shell="${esc(extension.shell_id)}" ${busy ? 'disabled' : ''}`) : '',
    enabled ? button(`${icon('refresh', 14)}Retry`, 'retry-extension', '', `data-name="${esc(name)}" ${busy || extension.status === 'probing' ? 'disabled' : ''}`) : '',
    button(`${icon('edit', 14)}Edit`, 'edit-extension', '', `data-name="${esc(name)}" ${busy ? 'disabled' : ''}`),
    button(`${icon('trash', 14)}Delete`, 'delete-extension', 'danger', `data-name="${esc(name)}" ${busy ? 'disabled' : ''}`),
  ].join('');
  const forwards = Array.isArray(extension.forwards) ? extension.forwards : [];
  const forwardLabel = forwards.map(forward => [forward.kind, forward.listen_addr, forward.target_addr].filter(Boolean).join(' · ')).join('; ') || (extension.forward_count ? `${extension.forward_count} declared forwards` : 'No forwards');
  const handleNote = extension.status === 'available' && !canOpen
    ? `<small class="extension-handle-note">${extension.url ? `MCP peer ${esc(name)} · no terminal shell` : 'Available without a terminal shell'}</small>`
    : '';
  const holes = Array.isArray(extension.holes) ? extension.holes : [];
  const holeBadge = needs.length ? `<small class="extension-hole-note">Needs ${esc(needs.join(', '))}</small>` : '';
  return `<article class="extension-row">
    <div class="extension-identity"><span class="extension-mark">${icon('core', 19)}</span><span><b>${esc(name)}</b><small>${esc(extension.description || 'No description')}</small></span></div>
    <div class="extension-shape"><span class="extension-kind ${templateInvalid ? 'extension-invalid' : ''}">${esc(shapeLabel(extension))}</span><code>${esc(targetLabel(extension))}</code>${targetRows(extension)}</div>
    <div class="extension-state">${statusRow(extension)}${handleNote}${holeBadge}</div>
    <div class="extension-meta"><span>${enabled ? 'Enabled' : 'Disabled'}</span><small title="${esc(forwardLabel)}">${esc(forwardLabel)}</small>${holes.length ? `<small>${holes.filter(hole => hole.set).length}/${holes.length} values set</small>` : ''}</div>
    <div class="extension-actions">${actions}</div>
  </article>`;
}

function importReport() {
  const report = state.extensionImport;
  if (!report) return '';
  const rows = Array.isArray(report.rows) ? report.rows : [];
  return `<section class="extension-import-report" aria-live="polite"><header><div><span>IMPORT RESULT</span><b>${esc(report.title || 'Template import')}</b></div><button class="icon-btn" data-action="dismiss-extension-import" aria-label="Dismiss import result">×</button></header>
    <div class="extension-import-list">${rows.map(row => `<div><span class="import-result import-${esc(row.status)}">${esc(row.status)}</span><span><b>${esc(row.name)}</b>${row.importedAs && row.importedAs !== row.name ? `<small>Created as ${esc(row.importedAs)}</small>` : ''}${row.error ? `<small class="import-error">${esc(row.error)}</small>` : ''}</span></div>`).join('') || '<p>No change report was returned by Core.</p>'}</div>
  </section>`;
}

export function extensionsPage() {
  const unavailable = !state.data.core.running;
  const extensions = Array.isArray(state.extensions) ? state.extensions : [];
  const counts = extensions.reduce((result, extension) => {
    result.total += 1;
    if (extension.status === 'available') result.available += 1;
    if (extension.status === 'unavailable' || extension.status === 'probing') result.unavailable += 1;
    if (Array.isArray(extension.needs) && extension.needs.length) result.needs += 1;
    return result;
  }, { total: 0, available: 0, unavailable: 0, needs: 0 });
  const actions = `${button(`${icon('plus', 14)}New template`, 'new-extension', 'primary', unavailable ? 'disabled' : '')}${button(`${icon('download', 14)}Import`, 'import-extensions', '', unavailable ? 'disabled' : '')}${button(`${icon('external', 14)}Export all`, 'export-extensions', '', extensions.length && !unavailable ? '' : 'disabled')}`;
  let body = '';
  if (unavailable) body = emptyState('Start Core to read extension templates.', true);
  else if (state.extensionsLoading) body = emptyState('Loading extension templates…', true);
  else if (state.extensionError) body = `<div class="extension-error" role="alert"><b>Extensions could not be read</b><p>${esc(state.extensionError)}</p><button class="button" data-action="refresh-extensions">Retry</button></div>`;
  else if (!extensions.length) body = emptyState('No extension templates are installed. Create one or import a TOML document.', true);
  else body = `<div class="extension-list">${extensions.map(extensionRow).join('')}</div>`;

  return `<div class="extension-page">
    ${header('Extensions', 'CAPABILITY TEMPLATES', actions)}
    <p class="extension-lead">Templates and installation values are stored and reconciled by Core. Status changes arrive through the Core event stream.</p>
    <div class="extension-summary"><div><span>Installed</span><strong>${counts.total}</strong></div><div><span>Available</span><strong>${counts.available}</strong></div><div><span>Needs attention</span><strong>${counts.unavailable + counts.needs}</strong></div><div><span>Status source</span><b>${state.wsStatus === 'connected' ? 'Core push connected' : 'Core read only'}</b></div></div>
    <input type="file" accept=".toml,application/toml,text/plain" data-extension-import-file hidden>
    ${importReport()}
    ${body}
  </div>`;
}

function holesPanel(dialog) {
  const holes = Array.isArray(dialog.holes) ? dialog.holes : [];
  if (!holes.length) return '<p class="extension-values-help">This template has no installation values.</p>';
  return `<div class="extension-holes">${holes.map(hole => `<div class="${hole.set ? 'is-set' : 'is-missing'}"><span>${esc(hole.name)}</span><code>${esc(hole.type || 'string')}</code><p>${esc(hole.help || '')}</p><b>${hole.set ? 'Set for this installation' : 'Needs a value'}</b></div>`).join('')}</div>`;
}

export function extensionEditorBody(dialog) {
  const form = { ...emptyExtensionForm(), ...(dialog.form || {}) };
  const mode = dialog.mode || 'form';
  const disabled = dialog.loading || dialog.saving ? 'disabled' : '';
  const existing = Boolean(dialog.original);
  const formDisabled = existing ? 'disabled' : '';
  const formView = `<div class="extension-editor-panel" data-extension-editor-panel="form" ${mode === 'form' ? '' : 'hidden'}>
    ${existing ? '<p class="extension-values-help">Existing templates are edited as lossless TOML so comments, typed holes, targets and fields Desktop does not render are preserved.</p>' : ''}
    <div class="extension-form-grid">
      <label>Single target<input name="target" value="${esc(form.target)}" list="extension-targets" ${formDisabled} placeholder="internal or SSH profile"><datalist id="extension-targets">${state.data.connections.map(connection => `<option value="${esc(connection.name)}"></option>`).join('')}</datalist></label>
      <label class="span-2">Multiple targets · JSON array<input name="targets" value="${esc(form.targets)}" ${formDisabled} placeholder='[{"host":"node-a","forwards":[]}]'></label>
      <label>Enabled<select name="enabled" ${formDisabled}><option value="true" ${form.enabled ? 'selected' : ''}>Enabled</option><option value="false" ${!form.enabled ? 'selected' : ''}>Disabled</option></select></label>
      <label>Resident<select name="resident" ${formDisabled}><option value="false" ${!form.resident ? 'selected' : ''}>On demand</option><option value="true" ${form.resident ? 'selected' : ''}>Always running</option></select></label>
      <label class="span-2">Description<input name="description" value="${esc(form.description)}" ${formDisabled}></label>
      <label>Shell mode<select name="mode" ${formDisabled}><option value="" ${!form.mode ? 'selected' : ''}>No shell</option><option value="pty" ${form.mode === 'pty' ? 'selected' : ''}>PTY</option><option value="pipe" ${form.mode === 'pipe' ? 'selected' : ''}>Pipe</option></select></label>
      <label>Idle timeout<input name="idle_timeout" value="${esc(form.idleTimeout)}" ${formDisabled} placeholder="10m; 0 disables collection"></label>
      <label class="span-2">Command<input name="command" value="${esc(form.command)}" ${formDisabled}></label>
      <label class="span-2">Arguments · one per line<textarea name="args" rows="3" ${formDisabled}>${esc(form.args)}</textarea></label>
      <label class="span-2">HTTP MCP URL<input name="url" value="${esc(form.url)}" ${formDisabled} placeholder="Optional; mutually exclusive with command"></label>
      <label class="span-2">TERM/1 downstream tool<input name="termcp_tool" value="${esc(form.termcpTool)}" ${formDisabled} placeholder="Optional tool name declared by the URL peer"></label>
      <label class="span-2">Headers · JSON object<textarea name="headers" rows="3" ${formDisabled} placeholder='{"Authorization": "Bearer …"}'>${esc(form.headers)}</textarea></label>
      <label>Allowed capabilities · one per line<textarea name="allow" rows="4" ${formDisabled}>${esc(form.allow)}</textarea></label>
      <label>Allowed paths · one per line<textarea name="paths" rows="4" ${formDisabled}>${esc(form.paths)}</textarea></label>
      <label class="span-2">Forwards · JSON array<textarea name="forwards" rows="4" ${formDisabled} placeholder='[{"kind":"local","local_port":8080,"remote_host":"localhost","remote_port":80}]'>${esc(form.forwards)}</textarea></label>
      <label class="span-2">Agent notes<textarea name="notes" rows="7" ${formDisabled}>${esc(form.notes)}</textarea></label>
    </div>
  </div>`;
  const tomlView = `<div class="extension-editor-panel" data-extension-editor-panel="toml" ${mode === 'toml' ? '' : 'hidden'}>
    <label>Template source · edited losslessly<textarea class="extension-toml-editor" name="toml" spellcheck="false">${esc(dialog.toml || '')}</textarea></label>
  </div>`;
  const valuesView = `<div class="extension-editor-panel" data-extension-editor-panel="values" ${mode === 'values' ? '' : 'hidden'}>
    ${holesPanel(dialog)}
    <label>Installation values · JSON object<textarea class="extension-values-editor" name="values" spellcheck="false" ${existing ? '' : 'disabled'}>${esc(dialog.values || '{}')}</textarea></label>
    <p class="extension-values-help">Values stay in <code>config.local.toml</code>. Export includes templates only; preview renders the current template with saved values.</p>
  </div>`;
  const failures = Array.isArray(dialog.failures) ? dialog.failures : [];
  const history = dialog.original ? `<section class="extension-history"><header><span>FAILURE HISTORY</span><b>${failures.length ? `${failures.length} recorded` : 'No recorded failures'}</b></header>${failures.length ? `<ol>${failures.map(failure => `<li><span>${esc(failure.stage || 'failure')}</span><time>${esc(failure.at ? new Date(failure.at).toLocaleString() : '')}</time><p>${esc(failure.reason || '')}</p></li>`).join('')}</ol>` : ''}</section>` : '';
  const preview = dialog.preview ? `<section class="extension-preview"><header><span>PREVIEW</span><button class="icon-btn" data-action="dismiss-extension-preview" aria-label="Dismiss preview">×</button></header><pre>${esc(dialog.preview)}</pre></section>` : '';
  return `<div class="extension-editor">
    <label class="extension-name-field">Template name<input name="name" value="${esc(form.name)}" ${dialog.original ? 'readonly' : ''} required pattern="[A-Za-z0-9._-]+" autofocus></label>
    <div class="extension-editor-tabs" role="tablist" aria-label="Template editor"><button type="button" class="${mode === 'form' ? 'active' : ''}" data-extension-editor-view="form" aria-pressed="${mode === 'form'}" ${disabled}>Fields</button><button type="button" class="${mode === 'toml' ? 'active' : ''}" data-extension-editor-view="toml" aria-pressed="${mode === 'toml'}" ${disabled}>Template TOML</button>${existing ? `<button type="button" class="${mode === 'values' ? 'active' : ''}" data-extension-editor-view="values" aria-pressed="${mode === 'values'}" ${disabled}>Values</button>` : ''}</div>
    <div class="extension-editor-status">${dialog.loading ? '<span>Loading template from Core…</span>' : dialog.status ? statusRow(dialog.status) : '<span>New template</span>'}</div>
    ${formView}${tomlView}${valuesView}${preview}${history}
  </div>`;
}

export function extensionEditorFooter(dialog) {
  const disabled = dialog.loading || dialog.saving ? 'disabled' : '';
  const preview = dialog.original ? `<button type="button" class="button" data-action="preview-extension" ${disabled}>Preview</button>` : '';
  return `<div class="modal-result extension-editor-result" role="status">${esc(dialog.result || '')}</div><span class="modal-spacer"></span>${preview}<button type="button" class="button" data-close ${disabled}>Cancel</button><button class="button primary" type="submit" ${disabled}>${dialog.saving ? 'Saving…' : (dialog.original ? 'Save template' : 'Create template')}</button>`;
}

export function extensionEditorOpen(name = '') {
  return {
    type: 'extension',
    original: name,
    mode: name ? 'toml' : 'form',
    loading: Boolean(name),
    saving: false,
    result: '',
    status: null,
    failures: [],
    holes: [],
    values: '{}',
    valuesDirty: false,
    preview: '',
    form: emptyExtensionForm(),
    toml: '',
  };
}
