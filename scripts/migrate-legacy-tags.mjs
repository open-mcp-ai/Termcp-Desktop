// One-time tag reference migration. Run only from release.yml with GITHUB_TOKEN,
// so creating historical tags does not start their outdated release workflows.
const token = process.env.GH_TOKEN;
const repo = process.env.GITHUB_REPOSITORY;
const phase = process.env.TAG_MIGRATION_PHASE;
if (!token || repo !== 'open-mcp-ai/Termcp-Desktop' || !['create', 'delete'].includes(phase)) {
  throw new Error('Unexpected repository or migration phase.');
}

const releases = [
  {
    old: 'v0.1.0',
    next: 'v0.1.0+core.0.1.16',
    oldRef: '4b08ce27e834a466f8e154792b88b69906e94602',
    commit: '959aa65489296bd68d834e369ac428d1098666d6',
    releaseId: 392810159,
  },
  {
    old: 'v0.1.1',
    next: 'v0.1.1+core.0.1.16',
    oldRef: '0904827dc406eb284daca109aefd7e5b145dc287',
    commit: '0904827dc406eb284daca109aefd7e5b145dc287',
    releaseId: 394660197,
  },
  {
    old: 'v0.2.5',
    next: 'v0.2.5+core.0.2.5',
    oldRef: 'ef60b80427bc85ad119bff010ba78e5b8e6c3dbf',
    commit: 'ef60b80427bc85ad119bff010ba78e5b8e6c3dbf',
    releaseId: 401922294,
  },
];
const invalid = {
  name: 'v0.2.5.1',
  ref: '0dbf6b460c173107092be349ec813b8e08c834bb',
};

async function api(method, path, body, allowMissing = false) {
  const response = await fetch(`https://api.github.com/repos/${repo}/${path}`, {
    method,
    headers: {
      Accept: 'application/vnd.github+json',
      Authorization: `Bearer ${token}`,
      'Content-Type': 'application/json',
      'X-GitHub-Api-Version': '2026-03-10',
    },
    body: body ? JSON.stringify(body) : undefined,
  });
  if (allowMissing && response.status === 404) return null;
  if (!response.ok) throw new Error(`${method} ${path}: HTTP ${response.status} ${await response.text()}`);
  return response.status === 204 ? null : response.json();
}

const ref = name => api('GET', `git/ref/tags/${encodeURIComponent(name)}`, undefined, true);
const release = id => api('GET', `releases/${id}`);
const assert = (condition, message) => {
  if (!condition) throw new Error(message);
};

// Check every release and reference before changing any of them.
for (const item of releases) {
  const [oldRef, newRef, published] = await Promise.all([ref(item.old), ref(item.next), release(item.releaseId)]);
  assert(oldRef?.object.sha === item.oldRef, `${item.old} changed unexpectedly`);
  assert(!newRef || (newRef.object.type === 'commit' && newRef.object.sha === item.commit), `${item.next} changed unexpectedly`);
  assert(published.tag_name === (phase === 'create' ? item.old : item.next), `Release ${item.releaseId} has an unexpected tag`);
  assert(published.assets.length >= 8, `Release ${item.releaseId} is missing assets`);
  if (phase === 'delete') assert(newRef, `${item.next} has not been created`);
}
const invalidRef = await ref(invalid.name);
assert(!invalidRef || invalidRef.object.sha === invalid.ref, `${invalid.name} changed unexpectedly`);
const invalidRelease = await api('GET', `releases/tags/${invalid.name}`, undefined, true);
assert(!invalidRelease, `${invalid.name} unexpectedly has a release`);

if (phase === 'create') {
  for (const item of releases) {
    if (await ref(item.next)) continue;
    await api('POST', 'git/refs', { ref: `refs/tags/${item.next}`, sha: item.commit });
    console.log(`Created ${item.next} at ${item.commit}`);
  }
} else {
  for (const item of releases) {
    await api('DELETE', `git/refs/tags/${encodeURIComponent(item.old)}`);
    console.log(`Deleted ${item.old}`);
  }
  if (invalidRef) {
    await api('DELETE', `git/refs/tags/${encodeURIComponent(invalid.name)}`);
    console.log(`Deleted unpublished ${invalid.name}`);
  }
}
