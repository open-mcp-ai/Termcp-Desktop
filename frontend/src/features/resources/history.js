// Row attribution follows Termcp WebUI timeline.js. Core supplies byte ranges
// for the terminal rows; input markers outrank their echo, then plain output.
// Output cause is the nearest preceding input in log order, not proven causality.
function historyRank(status) {
  let s = String(status || '');
  if (s === 'a') return 3;
  if (s === 'i') return 2;
  if (s === 'o' || s === '') return 1;
  return 0;
}
function historyCauses(marks) {
  let causes = new Array(marks.length);
  let last = '';
  for (let i = 0; i < marks.length; i++) {
    causes[i] = last;
    let s = String((marks[i] || {}).status || '');
    if (s === 'i' || s === 'a') last = s;
  }
  return causes;
}
export function historyRowCells(marks, spans) {
  if (!spans || !marks || !marks.length) return null;
  let rows = spans.length;
  let n = marks.length;
  let causes = historyCauses(marks);
  let cells = new Array(rows);
  let floor = Infinity;
  for (let f = 0; f < rows; f++) {
    let sp0 = spans[f];
    if (!sp0 || !(sp0.end > sp0.start) || sp0.start >= floor) continue;
    floor = Number(sp0.start);
  }
  let k = 0;
  if (floor !== Infinity) {
    let lo = 0;
    let hi = n;
    while (lo < hi) {
      let mid = (lo + hi) >> 1;
      let m0 = marks[mid] || {};
      let e0 = Number(m0.end);
      if (!isFinite(e0)) e0 = Number(m0.start);
      if (isFinite(e0) && e0 < floor) lo = mid + 1;
      else hi = mid;
    }
    k = lo;
  }
  let carry = null;
  let carryIdx = -1;
  for (let r = 0; r < rows; r++) {
    let sp = spans[r];
    if (!sp) continue;
    let s = Number(sp.start);
    let e = Number(sp.end);
    if (!isFinite(s) || !isFinite(e) || e <= s) continue;
    let hit = null;
    let hitIdx = -1;
    let review = false;
    if (carry) {
      if (carry.end <= s) carry = null;
      else if (historyRank(carry.status) > 0) { hit = carry; hitIdx = carryIdx; }
      else review = true;
    }
    while (k < n) {
      let m = marks[k];
      let ms = Number(m.start);
      if (!isFinite(ms)) { k++; continue; }
      let me = Number(m.end);
      if (!isFinite(me) || me < ms) me = ms;
      if (ms >= e) break;
      let rank = historyRank(m.status);
      let point = me <= ms;
      if (!(me > s || (point && ms >= s))) { k++; continue; }
      if (rank > 0) {
        if (!hit || rank > historyRank(hit.status)) {
          hit = m;
          hitIdx = k;
        }
      } else {
        review = true;
      }
      if (me > e) {
        carry = m;
        carryIdx = k;
        break;
      }
      k++;
    }
    if (!hit) continue;
    let time = Number(hit.time);
    if (!isFinite(time)) time = 0;
    let status = String(hit.status || '');
    cells[r] = {
      status: status,
      time: time,
      start: s,
      end: e,
      review: review,
      cause: (status === 'o' || status === '') ? String(causes[hitIdx] || '') : ''
    };
  }
  return cells;
}
