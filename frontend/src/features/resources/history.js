// Only output spans own bytes in Core's terminal log. An input mark at the same
// offset does not identify which command (if any) produced the later output.
export function historyPreviewSpan(spans, index) {
  const selected = spans[index];
  return selected?.status === 'o' && selected.end > selected.start ? selected : null;
}

// The byte log is a terminal stream, not plain text. Render a readable text
// excerpt for the timeline while keeping the raw log available for export.
export function readableTerminalText(raw) {
  const lines = [[]];
  let line = lines[0];
  let column = 0;
  let mode = 'text';
  let sequence = '';
  const write = character => { line[column] = character; column += 1; };
  for (const character of String(raw || '')) {
    if (mode === 'csi') {
      if (/[@-~]/.test(character)) {
        if (character === 'K') {
          if (sequence === '2') { line.length = 0; column = 0; }
          else line.length = column;
        }
        mode = 'text';
      } else sequence += character;
      continue;
    }
    if (mode === 'osc' || mode === 'string') {
      if (character === '\x07' && mode === 'osc') mode = 'text';
      else if (character === '\x1b') mode = `${mode}-escape`;
      continue;
    }
    if (mode === 'osc-escape' || mode === 'string-escape') {
      mode = character === '\\' ? 'text' : mode.slice(0, -7);
      continue;
    }
    if (mode === 'escape-argument') { mode = 'text'; continue; }
    if (mode === 'escape') {
      if (character === '[') { mode = 'csi'; sequence = ''; }
      else if (character === ']') mode = 'osc';
      else if ('P^_X'.includes(character)) mode = 'string';
      else if ('()#%'.includes(character)) mode = 'escape-argument';
      else mode = 'text';
      continue;
    }
    if (character === '\x1b') { mode = 'escape'; continue; }
    if (character === '\r') { column = 0; continue; }
    if (character === '\n') { line = []; lines.push(line); column = 0; continue; }
    if (character === '\b') { column = Math.max(0, column - 1); continue; }
    if (character === '\t') {
      do { write(' '); } while (column % 8);
      continue;
    }
    const code = character.codePointAt(0);
    if (code < 32 || code === 127) continue;
    write(character);
  }
  return lines.map(value => value.join('')).join('\n');
}
