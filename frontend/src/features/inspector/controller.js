function ensureMaps(inspector) {
  inspector.paths ||= {};
  inspector.shellDirectories ||= {};
}

function cleanDirectory(value) {
  const directory = String(value || '').trim();
  if (!directory || directory.includes('\0')) return '';
  return directory;
}

export async function performFileUpload({ upload, sessionID, directory, onSuccess, refresh }) {
  const result = await upload(sessionID, directory);
  if (result?.cancelled) return result;
  await onSuccess?.(result);
  await refresh?.();
  return result;
}

export function activateInspectorShell(inspector, shellID, detectedDirectory = '') {
  ensureMaps(inspector);
  const changed = inspector.shellID !== shellID;
  inspector.shellID = shellID || '';
  if (!shellID) {
    inspector.path = '';
    return changed;
  }
  const detected = cleanDirectory(detectedDirectory);
  if (detected) inspector.shellDirectories[shellID] = detected;
  inspector.path = inspector.paths[shellID] || detected || inspector.shellDirectories[shellID] || '';
  return changed;
}

export function rememberInspectorPath(inspector, shellID, directory) {
  ensureMaps(inspector);
  const path = cleanDirectory(directory);
  if (!shellID || !path) return false;
  inspector.paths[shellID] = path;
  if (inspector.shellID === shellID) inspector.path = path;
  return true;
}

export function followShellDirectory(inspector, shellID, directory) {
  ensureMaps(inspector);
  const path = cleanDirectory(directory);
  if (!shellID || !path) return false;
  const previous = inspector.shellDirectories[shellID] || '';
  const selected = inspector.paths[shellID] || '';
  const followsShell = !selected || selected === previous;
  inspector.shellDirectories[shellID] = path;
  if (!followsShell) return false;
  inspector.paths[shellID] = path;
  if (inspector.shellID === shellID) inspector.path = path;
  return previous !== path;
}
