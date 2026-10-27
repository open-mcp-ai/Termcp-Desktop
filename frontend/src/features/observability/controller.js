import { clientsFromFrame, normalizeActivity, normalizeClients } from './model.js';

export class ObservabilityController {
  constructor(core, state, { render } = {}) {
    this.core = core;
    this.state = state;
    this.render = render || (() => {});
    this.sequence = 0;
    this.refreshTimer = null;
  }

  async load({ quiet = false } = {}) {
    const sequence = ++this.sequence;
    const data = this.state.observability;
    if (!quiet) {
      data.loading = true;
      data.error = '';
      this.render();
    }
    const [activityResult, clientResult] = await Promise.allSettled([
      this.core.activity(data.sessionID, 200),
      this.core.clients(),
    ]);
    if (sequence !== this.sequence) return;
    if (activityResult.status === 'fulfilled') {
      data.activity = normalizeActivity(activityResult.value);
    } else {
      data.activity = [];
      data.error = String(activityResult.reason);
    }
    if (clientResult.status === 'fulfilled') {
      const clients = normalizeClients(clientResult.value);
      if (!data.clients.webui.length && !data.clients.mcp.length) data.clients = clients;
    }
    data.loading = false;
    this.render();
  }

  applySessionFrame(message) {
    const clients = clientsFromFrame(message);
    if (!clients) return false;
    this.state.observability.clients = clients;
    if (this.state.section === 'observability') this.render();
    return true;
  }

  activityEvent() {
    if (this.state.section !== 'observability') return;
    clearTimeout(this.refreshTimer);
    this.refreshTimer = setTimeout(() => this.load({ quiet: true }), 220);
  }

  async selectSession(sessionID) {
    this.state.observability.sessionID = sessionID || '';
    await this.load();
  }

  dispose() {
    clearTimeout(this.refreshTimer);
  }
}
