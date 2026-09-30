// Core executes an executable plus argv; the form accepts a typed line.
export function commandLinePayload(line) {
  const words = [];
  let current = '';
  let quote = '';
  let started = false;
  for (const char of String(line || '').trim()) {
    if (quote) {
      if (char === quote) quote = '';
      else current += char;
    } else if (char === '"' || char === "'") {
      quote = char;
      started = true;
    } else if (/\s/.test(char)) {
      if (started) { words.push(current); current = ''; started = false; }
    } else {
      current += char;
      started = true;
    }
  }
  if (quote) throw new Error('Close the quote in the startup command');
  if (started) words.push(current);
  return { command: words.shift() || '', args: words };
}
