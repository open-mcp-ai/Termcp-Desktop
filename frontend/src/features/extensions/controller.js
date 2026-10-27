export class ExtensionController {
  constructor(api) {
    this.api = api;
  }

  async list() {
    const response = await this.api('GET', '/api/extensions');
    return Array.isArray(response.data?.extensions) ? response.data.extensions : [];
  }

  async detail(name) {
    const response = await this.api('GET', `/api/extensions/${encodeURIComponent(name)}`);
    return response.data || {};
  }

  async toml(name) {
    const response = await this.api('GET', `/api/extensions/${encodeURIComponent(name)}?format=toml`);
    return response.data ?? response.body ?? '';
  }

  async editor(name) {
    if (!name) return { detail: {}, toml: '' };
    const [detail, toml] = await Promise.all([this.detail(name), this.toml(name)]);
    return { detail, toml };
  }

  async saveForm(original, name, payload) {
    const path = original ? `/api/extensions/${encodeURIComponent(original)}` : '/api/extensions';
    const method = original ? 'PUT' : 'POST';
    return this.api(method, path, payload, 'application/json');
  }

  async saveTOML(original, name, document) {
    const suffix = original ? '' : `?name=${encodeURIComponent(name)}`;
    const path = `/api/extensions${original ? `/${encodeURIComponent(original)}` : ''}${suffix}`;
    return this.api(original ? 'PUT' : 'POST', path, String(document || ''), 'application/toml; charset=utf-8');
  }

  async remove(name) {
    return this.api('DELETE', `/api/extensions/${encodeURIComponent(name)}`);
  }

  async retry(name) {
    const response = await this.api('POST', `/api/extensions/${encodeURIComponent(name)}/retry`, {});
    if (response.data?.ok === false) throw new Error(response.data.error || 'Retry failed');
    return response.data;
  }

  async importDocument(document) {
    const response = await this.api('POST', '/api/extensions/batch', String(document || ''), 'application/toml; charset=utf-8');
    return response.data || {};
  }

  async exportDocument(names = []) {
    const query = Array.isArray(names) && names.length ? `?names=${encodeURIComponent(names.join(','))}` : '';
    const response = await this.api('GET', `/api/extensions/batch${query}`);
    return response.data ?? response.body ?? '';
  }
}

export const EXTENSION_IMPORT_LIMIT = 16 << 20;

export async function readExtensionImport(file) {
  if (!file) return { cancelled: true };
  if (Number(file.size) > EXTENSION_IMPORT_LIMIT) {
    throw new Error(`Import file is larger than ${EXTENSION_IMPORT_LIMIT} bytes`);
  }
  const text = typeof file.text === 'function' ? await file.text() : await readAsText(file);
  if (!String(text || '').trim()) throw new Error('Import file is empty');
  return { cancelled: false, document: String(text) };
}

function readAsText(file) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error || new Error('Import file could not be read'));
    reader.onload = () => resolve(String(reader.result || ''));
    reader.readAsText(file);
  });
}
