const icons = {
  resources: '<circle cx="7" cy="7" r="3"/><circle cx="17" cy="17" r="3"/><path d="m9.2 9.2 5.6 5.6M14 7h6v6"/>',
  sessions: '<rect x="4" y="4" width="14" height="12" rx="2"/><path d="M8 20h12V8M8 9l2 2-2 2m5 0h2"/>',
  history: '<circle cx="12" cy="12" r="8"/><path d="M12 8v5l3 2M4 5v4h4"/>',
  terminal: '<path d="m5 7 4 5-4 5m7 0h7"/>',
  core: '<path d="M8 3h8v4h4v10h-4v4H8v-4H4V7h4z"/><circle cx="12" cy="12" r="3"/>',
  service: '<rect x="4" y="4" width="16" height="6" rx="2"/><rect x="4" y="14" width="16" height="6" rx="2"/><path d="M8 7h.01M8 17h.01M12 7h5M12 17h5"/>',
  settings: '<path d="M12.2 2h-.4a2 2 0 0 0-2 2v.2a2 2 0 0 1-1 1.7l-.4.2a2 2 0 0 1-2 0L6.2 6a2 2 0 0 0-2.7.7l-.2.4A2 2 0 0 0 4 9.8l.2.1a2 2 0 0 1 1 1.7v.5a2 2 0 0 1-1 1.7l-.2.1a2 2 0 0 0-.7 2.7l.2.4a2 2 0 0 0 2.7.7l.2-.1a2 2 0 0 1 2 0l.4.2a2 2 0 0 1 1 1.7v.2a2 2 0 0 0 2 2h.4a2 2 0 0 0 2-2v-.2a2 2 0 0 1 1-1.7l.4-.2a2 2 0 0 1 2 0l.2.1a2 2 0 0 0 2.7-.7l.2-.4a2 2 0 0 0-.7-2.7l-.2-.1a2 2 0 0 1-1-1.7v-.5a2 2 0 0 1 1-1.7l.2-.1a2 2 0 0 0 .7-2.7l-.2-.4a2 2 0 0 0-2.7-.7l-.2.1a2 2 0 0 1-2 0l-.4-.2a2 2 0 0 1-1-1.7V4a2 2 0 0 0-2-2z"/><circle cx="12" cy="12" r="3"/>',
  refresh: '<path d="M20 7v5h-5M4 17v-5h5M6 7a7 7 0 0 1 12-2l2 2M18 17a7 7 0 0 1-12 2l-2-2"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  search: '<circle cx="11" cy="11" r="6"/><path d="m16 16 4 4"/>',
  chevron: '<path d="m9 6 6 6-6 6"/>',
  edit: '<path d="m4 20 4-1 11-11-3-3L5 16z"/><path d="m14 6 3 3"/>',
  trash: '<path d="M5 7h14M9 7V4h6v3m-8 0 1 13h8l1-13"/>',
  split: '<rect x="3" y="4" width="18" height="16" rx="2"/><path d="M12 4v16"/>',
  folder: '<path d="M3 6h7l2 2h9v11H3z"/>',
  forward: '<path d="M5 7h11m0 0-3-3m3 3-3 3M19 17H8m0 0 3-3m-3 3 3 3"/>',
  bell: '<path d="M6 16h12l-2-3V9a4 4 0 0 0-8 0v4z"/><path d="M10 19h4"/>',
  download: '<path d="M12 3v12m0 0 4-4m-4 4-4-4M5 20h14"/>',
  collapse: '<path d="m14 7-5 5 5 5"/>',
  expand: '<path d="m10 7 5 5-5 5"/>',
  copy: '<rect x="8" y="8" width="11" height="11" rx="2"/><path d="M16 8V5a2 2 0 0 0-2-2H5a2 2 0 0 0-2 2v9a2 2 0 0 0 2 2h3"/>',
  external: '<path d="M14 4h6v6m0-6-9 9"/><path d="M18 13v6a1 1 0 0 1-1 1H5a1 1 0 0 1-1-1V7a1 1 0 0 1 1-1h6"/>',
  eye: '<path d="M2 12s3.5-6 10-6 10 6 10 6-3.5 6-10 6S2 12 2 12z"/><circle cx="12" cy="12" r="2.5"/>',
  paste: '<path d="M9 5h6v3H9z"/><path d="M7 6H5v15h14V6h-2"/>',
  select: '<path d="M5 3H3v2M19 3h2v2M5 21H3v-2M19 21h2v-2"/><path d="M8 8h8M8 12h8M8 16h5"/>',
  clear: '<path d="m4 15 7-7 6 6-7 7H4z"/><path d="m14 5 5 5"/>',
};

export const icon = (name, size = 17) => `<svg viewBox="0 0 24 24" width="${size}" height="${size}" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${icons[name] || ''}</svg>`;
export const esc = (value = '') => String(value).replace(/[&<>"']/g, char => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[char]);
export const dot = (status = 'running') => `<i class="status-dot ${esc(status)}"></i>`;
export const encode = value => encodeURIComponent(String(value));

export function fmtSize(bytes) {
  let size = Number(bytes || 0); const units = ['B', 'KB', 'MB', 'GB', 'TB']; let index = 0;
  while (size >= 1024 && index < units.length - 1) { size /= 1024; index += 1; }
  return `${index ? size.toFixed(1) : size} ${units[index]}`;
}

export function parentPath(value) {
  const clean = String(value || '/').replace(/\/+$/, '') || '/';
  if (clean === '/') return '/';
  const parts = clean.split('/'); parts.pop(); return parts.join('/') || '/';
}

export const joinPath = (base, name) => `${String(base || '/').replace(/\/+$/, '')}/${name}`.replace(/^\/+/, '/');
