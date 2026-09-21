import { esc } from './render.js';

export const header = (title, eyebrow, actions = '') => `<header class="content-head"><div><div class="eyebrow">${eyebrow}</div><h1>${esc(title)}</h1></div><div class="head-actions">${actions}</div></header>`;

export const button = (label, action, kind = '', extra = '') => `<button class="button ${kind}" data-action="${action}" ${extra}>${label}</button>`;

export const emptyState = (message, large = false) => `<div class="empty-state${large ? ' large' : ''}">${esc(message)}</div>`;

export const emptyPage = () => `${header('Select a resource', 'RESOURCE EXPLORER')}${emptyState('Select a connection, session, shell or history entry from the left.', true)}`;
