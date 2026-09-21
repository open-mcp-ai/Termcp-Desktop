import chinese from './zh-CN.js';

const STORAGE_KEY = 'termcp-desktop-language';
const resources = { 'zh-CN': chinese };
const supported = ['en', ...Object.keys(resources)];
const stored = localStorage.getItem(STORAGE_KEY);
let language = supported.includes(stored)
  ? stored
  : (navigator.language || '').toLowerCase().startsWith('zh') ? 'zh-CN' : 'en';
let replacements = [];

function updateReplacements() {
  replacements = Object.entries(resources[language] || {}).sort((a, b) => b[0].length - a[0].length);
}

export function getLanguage() { return language; }

export function setLanguage(next) {
  language = supported.includes(next) ? next : 'en';
  localStorage.setItem(STORAGE_KEY, language);
  document.documentElement.lang = language;
  updateReplacements();
  return language;
}

export function t(value) {
  const source = String(value ?? '');
  if (language === 'en') return source;
  let translated = source;
  for (const [from, to] of replacements) translated = translated.split(from).join(to);
  return translated;
}

function shouldSkip(node) {
  const parent = node.nodeType === Node.ELEMENT_NODE ? node : node.parentElement;
  return Boolean(parent?.closest('code, pre, textarea, [data-i18n-ignore]'));
}

export function localizeDOM(root = document) {
  document.documentElement.lang = language;
  if (language === 'en' || !root) return;
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  const nodes = [];
  while (walker.nextNode()) nodes.push(walker.currentNode);
  for (const node of nodes) if (!shouldSkip(node)) node.nodeValue = t(node.nodeValue);
  const elements = root.querySelectorAll?.('[title], [aria-label], [placeholder]') || [];
  for (const element of elements) {
    if (shouldSkip(element)) continue;
    for (const attribute of ['title', 'aria-label', 'placeholder']) {
      if (element.hasAttribute(attribute)) element.setAttribute(attribute, t(element.getAttribute(attribute)));
    }
  }
}

setLanguage(language);
