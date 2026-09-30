const KEY = 'termcp-desktop-resource-tags';

function loadTags() {
  try {
    const value = JSON.parse(localStorage.getItem(KEY) || '{}');
    if (value && typeof value === 'object' && !Array.isArray(value)) return value;
  } catch { /* Ignore damaged local preferences. */ }
  return {};
}

const saved = loadTags();
const keyFor = (type, id) => `${type === 'history' ? 'session' : type}:${id}`;

export function tagsFor(type, id) {
  const value = saved[keyFor(type, id)];
  return Array.isArray(value) ? value.filter(tag => typeof tag === 'string') : [];
}

export function setTags(type, id, raw) {
  const tags = [...new Set((Array.isArray(raw) ? raw : String(raw).split(',')).map(tag => String(tag).trim()).filter(Boolean))];
  const key = keyFor(type, id);
  if (tags.length) saved[key] = tags;
  else delete saved[key];
  localStorage.setItem(KEY, JSON.stringify(saved));
  return tags;
}

export function moveTags(type, oldID, newID) {
  if (oldID === newID) return;
  const oldKey = keyFor(type, oldID);
  const newKey = keyFor(type, newID);
  if (saved[oldKey]) saved[newKey] = saved[oldKey];
  delete saved[oldKey];
  localStorage.setItem(KEY, JSON.stringify(saved));
}

export function knownTags(resources) {
  return [...new Set(resources.flatMap(({ type, id }) => tagsFor(type, id)))].sort((a, b) => a.localeCompare(b));
}
