export function normalizeActivity(events) {
  return Array.isArray(events)
    ? events.filter(event => event && event.session_id && event.shell_id)
    : [];
}

export function normalizeClients(value = {}) {
  value ||= {};
  return {
    webui: Array.isArray(value.webui) ? value.webui : [],
    mcp: Array.isArray(value.mcp) ? value.mcp : [],
  };
}

export function clientsFromFrame(message = {}) {
  if (!Array.isArray(message.clients) && !Array.isArray(message.mcp_clients)) return null;
  return normalizeClients({ webui: message.clients, mcp: message.mcp_clients });
}

export function activityMatches(event, sessionID = '') {
  return !sessionID || event?.session_id === sessionID;
}

export function activityLabel(event = {}) {
  const source = event.source === 'ai' ? 'AI' : event.source === 'human' ? 'Human' : 'Review';
  const action = event.verdict === 'review-requested'
    ? 'requested approval'
    : event.verdict === 'review-granted'
      ? 'approved an action'
      : 'submitted input';
  return `${source} ${action}`;
}

export function activitySessionLabel(event, sessions = []) {
  const session = sessions.find(item => item.id === event.session_id);
  return session?.name || event.session_name || event.session_id || '';
}

export function activityTime(value) {
  const time = Date.parse(value || '');
  return Number.isFinite(time) && time > 0 ? new Date(time).toLocaleString() : 'Time unavailable';
}
