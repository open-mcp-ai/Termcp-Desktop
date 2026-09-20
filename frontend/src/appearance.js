const STORAGE_KEY = 'termcp-desktop-appearance';

const defaults = {
  theme: 'light',
  fontSize: 14,
  fontFamily: 'system-ui',
};

const legacySizes = { medium: 13, large: 14, 'extra-large': 16 };
const legacyFamilies = { system: 'system-ui', modern: 'Helvetica Neue', mono: 'Menlo' };

function normalise(candidate = {}) {
  const rawSize = legacySizes[candidate.fontSize] || Number(candidate.fontSize);
  const fontSize = Number.isInteger(rawSize) && rawSize >= 12 && rawSize <= 20 ? rawSize : defaults.fontSize;
  const rawFamily = legacyFamilies[candidate.fontFamily] || candidate.fontFamily;
  const fontFamily = typeof rawFamily === 'string' && rawFamily.trim().length > 0 && rawFamily.length <= 120
    ? rawFamily.trim()
    : defaults.fontFamily;
  return {
    theme: candidate.theme === 'dark' ? 'dark' : 'light',
    fontSize,
    fontFamily,
  };
}

function read() {
  try {
    return normalise(JSON.parse(localStorage.getItem(STORAGE_KEY) || '{}'));
  } catch {
    return { ...defaults };
  }
}

function safeFamily(value) {
  if (value === 'system-ui') return '-apple-system, BlinkMacSystemFont, "Segoe UI", "PingFang SC", "Microsoft YaHei", sans-serif';
  return `"${value.replace(/["\\]/g, '')}", -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif`;
}

let appearance = read();

export function getAppearance() {
  return { ...appearance };
}

export function applyAppearance() {
  const root = document.documentElement;
  root.dataset.theme = appearance.theme;
  root.dataset.fontSize = String(appearance.fontSize);
  root.dataset.fontFamily = appearance.fontFamily === 'system-ui' ? 'system' : 'custom';
  root.style.colorScheme = appearance.theme;
  root.style.setProperty('--app-font-size', `${appearance.fontSize}px`);
  root.style.setProperty('--app-font-family', safeFamily(appearance.fontFamily));
}

export function setAppearance(patch) {
  appearance = normalise({ ...appearance, ...patch });
  localStorage.setItem(STORAGE_KEY, JSON.stringify(appearance));
  applyAppearance();
  return getAppearance();
}

export function terminalAppearance() {
  const selected = appearance.fontFamily === 'system-ui' ? 'SFMono-Regular' : appearance.fontFamily.replace(/["\\]/g, '');
  const lightTheme = {
    background: '#fbfcfe',
    foreground: '#273444',
    cursor: '#3568c8',
    cursorAccent: '#fbfcfe',
    selectionBackground: '#9bbcff66',
    black: '#273444',
    red: '#b93645',
    green: '#277a4a',
    yellow: '#8c6500',
    blue: '#3568c8',
    magenta: '#844bb5',
    cyan: '#147681',
    white: '#e4e9f0',
    brightBlack: '#667386',
    brightRed: '#d34b59',
    brightGreen: '#35945d',
    brightYellow: '#a97900',
    brightBlue: '#5787e0',
    brightMagenta: '#9a63c7',
    brightCyan: '#26929e',
    brightWhite: '#ffffff',
  };
  const darkTheme = {
    background: '#0d1219',
    foreground: '#d6dee8',
    cursor: '#79a6ff',
    cursorAccent: '#0d1219',
    selectionBackground: '#4770a866',
    black: '#151c25',
    red: '#f06b75',
    green: '#71c58a',
    yellow: '#d8ae59',
    blue: '#79a6ff',
    magenta: '#bc88e6',
    cyan: '#65c4ce',
    white: '#d6dee8',
    brightBlack: '#748194',
    brightRed: '#ff8790',
    brightGreen: '#92d5a4',
    brightYellow: '#edc873',
    brightBlue: '#9bbdff',
    brightMagenta: '#d2a5f0',
    brightCyan: '#88d8df',
    brightWhite: '#ffffff',
  };
  return {
    fontSize: appearance.fontSize,
    fontFamily: `"${selected}", "Cascadia Mono", Menlo, Monaco, Consolas, monospace`,
    theme: appearance.theme === 'dark' ? darkTheme : lightTheme,
  };
}

applyAppearance();
