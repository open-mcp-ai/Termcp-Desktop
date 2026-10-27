export const EXTENSION_KINDS = ['mcp', 'skill', 'prompt', 'custom'];

export function isExtensionNamespace(namespace) {
  const value = String(namespace || '').trim();
  return value === 'ext' || value.startsWith('ext:');
}

export function userSessions(items = []) {
  return (Array.isArray(items) ? items : []).filter(item => !isExtensionNamespace(item?.namespace));
}

export function emptyExtensionForm() {
  return {
    name: '',
    kind: 'custom',
    target: '',
    description: '',
    enabled: true,
    mode: '',
    command: '',
    args: '',
    url: '',
    headers: '',
    allow: '',
    paths: '',
    idleTimeout: '',
    notes: '',
    forwards: '',
  };
}

export function formFromExtensionDetail(detail = {}, document = '') {
  const extension = detail.extension || {};
  const file = parseFileFields(document);
  return {
    name: extension.name || '',
    kind: detail.kind || extension.kind || 'custom',
    target: extension.target || '',
    description: extension.description || '',
    enabled: extension.enabled !== false,
    mode: detail.mode || '',
    command: detail.command || '',
    args: joinLines(detail.args),
    url: extension.url || '',
    headers: formatObject(detail.headers || file.headers),
    allow: joinLines(detail.allow),
    paths: joinLines(detail.paths),
    idleTimeout: detail.idle_timeout || file.idleTimeout,
    notes: detail.notes || '',
    forwards: formatObject(detail.forward_decls, true),
  };
}

export function formValues(form) {
  const data = {};
  for (const [key, value] of new FormData(form).entries()) data[key] = String(value);
  const enabled = form.elements?.enabled;
  data.enabled = enabled?.type === 'checkbox'
    ? Boolean(enabled.checked)
    : String(enabled?.value ?? 'true') !== 'false';
  return {
    name: data.name || '',
    kind: data.kind || 'custom',
    target: data.target || '',
    description: data.description || '',
    enabled: data.enabled,
    mode: data.mode || '',
    command: data.command || '',
    args: data.args || '',
    url: data.url || '',
    headers: data.headers || '',
    allow: data.allow || '',
    paths: data.paths || '',
    idleTimeout: data.idle_timeout || '',
    notes: data.notes || '',
    forwards: data.forwards || '',
  };
}

export function extensionPayload(form, { create = false } = {}) {
  const args = splitLines(form.args);
  const allow = splitLines(form.allow);
  const paths = splitLines(form.paths);
  const headers = parseObject(form.headers, 'Headers');
  const forwards = parseArray(form.forwards, 'Forwards');
  const payload = {
    kind: form.kind,
    target: twoWay(form.target),
    enabled: Boolean(form.enabled),
  };
  if (create) payload.name = twoWay(form.name);

  setText(payload, 'description', form.description, { create, clear: false });
  setText(payload, 'mode', form.mode, { create, clear: false });
  setText(payload, 'command', form.command, { create, clear: false });
  setText(payload, 'url', form.url, { create, clear: false });
  setText(payload, 'idle_timeout', form.idleTimeout, { create, clear: false });
  setText(payload, 'notes', form.notes, { create, clear: false });
  setList(payload, 'args', args, form.args, create);
  setList(payload, 'allow', allow, form.allow, create);
  setList(payload, 'paths', paths, form.paths, create);
  setMap(payload, 'headers', headers, form.headers, create);
  setList(payload, 'forwards', forwards, form.forwards, create);
  return payload;
}

export function extensionNamesFromDocument(document) {
  const names = [];
  let inExtensions = false;
  for (const rawLine of String(document || '').split(/\r?\n/)) {
    const line = rawLine.trim();
    if (/^\[\[extensions\]\]$/.test(line)) {
      inExtensions = true;
      continue;
    }
    if (/^\[/.test(line)) {
      inExtensions = false;
      continue;
    }
    if (!inExtensions) continue;
    const match = line.match(/^name\s*=\s*(?:"((?:\\.|[^"\\])*)"|'([^']*)')\s*(?:#.*)?$/);
    if (!match) continue;
    if (match[1] !== undefined) {
      try { names.push(JSON.parse(`"${match[1]}"`)); }
      catch { names.push(match[1]); }
    } else {
      names.push(match[2]);
    }
  }
  return names;
}

export function importReport(document, result = {}) {
  if (Array.isArray(result.results)) {
    return result.results.map(item => ({
      name: item.name || item.from || item.id || '',
      importedAs: item.to || item.name || item.id || '',
      status: item.ok === false ? 'failed' : (item.status || (item.skipped ? 'skipped' : 'created')),
      error: item.error || '',
    }));
  }
  const names = extensionNamesFromDocument(document);
  const renamed = Array.isArray(result.renamed) ? [...result.renamed] : [];
  const importedCount = Number(result.imported);
  if (!names.length && importedCount > 0) {
    return Array.from({ length: importedCount }, (_, index) => ({
      name: `Template ${index + 1}`,
      importedAs: renamed[index]?.to || `Template ${index + 1}`,
      status: 'created',
      error: '',
    }));
  }
  const rows = names.map((name, index) => {
    const renameIndex = renamed.findIndex(item => item.from === name);
    const rename = renameIndex >= 0 ? renamed.splice(renameIndex, 1)[0] : null;
    const created = !Number.isFinite(importedCount) || index < importedCount;
    return {
      name,
      importedAs: rename?.to || name,
      status: created ? 'created' : 'skipped',
      error: '',
    };
  });
  for (const rename of renamed) {
    rows.push({ name: rename.from, importedAs: rename.to, status: 'created', error: '' });
  }
  return rows;
}

export function importFailureReport(error) {
  return [{
    name: 'No templates imported',
    importedAs: '',
    status: 'failed',
    error: String(error?.message || error || 'Import failed'),
  }];
}

function twoWay(value) {
  return String(value || '').trim();
}

function splitLines(value) {
  return String(value || '').split(/\r?\n/).map(item => item.trim()).filter(Boolean);
}

function joinLines(value) {
  return Array.isArray(value) ? value.join('\n') : '';
}

function parseObject(value, label) {
  const raw = String(value || '').trim();
  if (!raw) return {};
  let parsed;
  try { parsed = JSON.parse(raw); } catch (error) { throw new Error(`${label} must be valid JSON: ${error.message}`); }
  if (!parsed || Array.isArray(parsed) || typeof parsed !== 'object') throw new Error(`${label} must be a JSON object`);
  return parsed;
}

function parseArray(value, label) {
  const raw = String(value || '').trim();
  if (!raw) return [];
  let parsed;
  try { parsed = JSON.parse(raw); } catch (error) { throw new Error(`${label} must be valid JSON: ${error.message}`); }
  if (!Array.isArray(parsed)) throw new Error(`${label} must be a JSON array`);
  return parsed;
}

function formatObject(value, array = false) {
  if (array) return Array.isArray(value) && value.length ? JSON.stringify(value, null, 2) : '';
  return value && Object.keys(value).length ? JSON.stringify(value, null, 2) : '';
}

function parseFileFields(document) {
  const source = String(document || '');
  const idle = source.match(/^\s*idle_timeout\s*=\s*(?:"((?:\\.|[^"\\])*)"|'([^']*)')\s*(?:#.*)?$/m);
  const headers = {};
  let inHeaders = false;
  for (const rawLine of source.split(/\r?\n/)) {
    const line = rawLine.trim();
    if (/^\[headers\]$/.test(line)) { inHeaders = true; continue; }
    if (/^\[/.test(line)) { inHeaders = false; continue; }
    if (!inHeaders || !line || line.startsWith('#')) continue;
    const match = line.match(/^("((?:\\.|[^"\\])*)"|'([^']*)'|[A-Za-z0-9_-]+)\s*=\s*(?:"((?:\\.|[^"\\])*)"|'([^']*)')\s*(?:#.*)?$/);
    if (!match) continue;
    const key = match[2] ?? match[3] ?? match[1];
    const value = match[4] ?? match[5] ?? '';
    headers[key] = value;
  }
  return {
    idleTimeout: idle?.[1] ?? idle?.[2] ?? '',
    headers,
  };
}

function setText(payload, key, value, { create, clear }) {
  const text = String(value || '');
  if (!create || clear || text.trim()) payload[key] = text;
}

function setList(payload, key, values, raw, create) {
  if (!create || String(raw || '').trim()) payload[key] = values;
}

function setMap(payload, key, value, raw, create) {
  if (!create || String(raw || '').trim()) payload[key] = value;
}
