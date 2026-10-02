export class SessionCreationCoordinator {
  constructor({ create, snapshot, connected, timeoutMs = 3000 }) {
    this.createRequest = create;
    this.snapshot = snapshot;
    this.connected = connected;
    this.timeoutMs = timeoutMs;
    this.pendingProfiles = new Set();
    this.visibleSessions = new Set();
    this.waiters = new Map();
  }

  pending(connection) { return this.pendingProfiles.has(String(connection)); }

  observe(message) {
    if (message?.type !== 'sessions' || !Array.isArray(message.sessions)) return;
    this.visibleSessions = new Set(message.sessions.map(session => session.id).filter(Boolean));
    for (const id of this.visibleSessions) this.waiters.get(id)?.();
  }

  waitForNotification(id) {
    if (this.visibleSessions.has(id)) return Promise.resolve(true);
    return new Promise(resolve => {
      const timer = setTimeout(() => {
        this.waiters.delete(id);
        resolve(false);
      }, this.timeoutMs);
      this.waiters.set(id, () => {
        clearTimeout(timer);
        this.waiters.delete(id);
        resolve(true);
      });
    });
  }

  async confirmSnapshot(id) {
    const current = await this.snapshot();
    if (![...(current.sessions || []), ...(current.history || [])].some(session => session.id === id)) {
      throw new Error(`Core did not confirm session ${id}; refresh the resource list before retrying.`);
    }
  }

  async create(connection, payload) {
    const key = String(connection);
    if (this.pendingProfiles.has(key)) return null;
    this.pendingProfiles.add(key);
    try {
      const response = await this.createRequest(payload);
      const session = response?.data || response;
      if (!session?.session_id || !session.shell_id) throw new Error('Core did not return a session and shell ID.');
      if (!(this.connected() && await this.waitForNotification(session.session_id))) {
        await this.confirmSnapshot(session.session_id);
      }
      return session;
    } finally {
      this.pendingProfiles.delete(key);
    }
  }
}
