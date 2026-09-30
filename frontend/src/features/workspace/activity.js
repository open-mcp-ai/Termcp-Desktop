const IDLE_MS = 3000;

// Core reports the source and whether a line was submitted. Terminal output
// cannot close a pending line because the shell also echoes each keystroke.
export class ShellActivityTracker {
  constructor(onChange = () => {}, idleMs = IDLE_MS) {
    this.onChange = onChange;
    this.idleMs = idleMs;
    this.pending = new Map();
    this.timers = new Map();
  }

  status(shellID) { return this.pending.get(shellID) || ''; }

  input({ shell_id: shellID, src, submit }) {
    if (!shellID || !['api', 'ai'].includes(src) || typeof submit !== 'boolean') return;
    if (submit) {
      this.set(shellID, '');
    } else {
      const current = this.status(shellID);
      this.set(shellID, src === 'ai' || current === 'ai' ? 'ai' : 'human');
    }
    this.touch(shellID);
  }

  output(shellID, bytes) {
    if (shellID && bytes && this.status(shellID)) this.touch(shellID);
  }

  end(shellID) {
    clearTimeout(this.timers.get(shellID));
    this.timers.delete(shellID);
    this.set(shellID, '');
  }

  clear() {
    for (const shellID of new Set([...this.pending.keys(), ...this.timers.keys()])) this.end(shellID);
  }

  set(shellID, status) {
    if (this.status(shellID) === status) return;
    if (status) this.pending.set(shellID, status);
    else this.pending.delete(shellID);
    this.onChange(shellID, status);
  }

  touch(shellID) {
    clearTimeout(this.timers.get(shellID));
    this.timers.set(shellID, setTimeout(() => this.end(shellID), this.idleMs));
  }
}
