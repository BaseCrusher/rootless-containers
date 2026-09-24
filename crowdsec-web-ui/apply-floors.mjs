// Apply the security floors in packages_override.json to pnpm-workspace.yaml's `overrides`.
// A floor is applied only when the lowest version already in the tree is below
// `min`; otherwise it is inert and reported. Run from the app root after an
// install. Exit 10 = a floor was applied (reinstall), 0 = nothing to do.
// `node apply-floors.mjs --selftest` runs the version-compare self-check.

import { readFileSync, writeFileSync, readdirSync } from 'node:fs';
import { createRequire } from 'node:module';

// Compare dotted numeric versions (pre-release/build metadata ignored).
export function cmp(a, b) {
  const p = (v) => v.split(/[-+]/)[0].split('.').map(Number);
  const [pa, pb] = [p(a), p(b)];
  for (let i = 0; i < 3; i++) {
    const d = (pa[i] || 0) - (pb[i] || 0);
    if (d) return d < 0 ? -1 : 1;
  }
  return 0;
}

function lowestInstalled(pkg) {
  const prefix = pkg.replace(/\//g, '+') + '@';
  let versions;
  try {
    versions = readdirSync('node_modules/.pnpm')
      .filter((d) => d.startsWith(prefix))
      .map((d) => d.slice(prefix.length).split('_')[0]);
  } catch {
    versions = [];
  }
  return versions.sort(cmp)[0];
}

function main() {
  const { parse, stringify } = createRequire('/app/node_modules/')('yaml');
  const floors = JSON.parse(readFileSync('/packages_override.json', 'utf8'));
  const ws = parse(readFileSync('pnpm-workspace.yaml', 'utf8')) ?? {};
  ws.overrides ??= {};

  let changed = false;
  for (const { package: pkg, min, advisory } of floors) {
    const cur = lowestInstalled(pkg);
    if (!cur) {
      console.log(`${pkg}: not in the tree, floor skipped (${advisory})`);
    } else if (cmp(cur, min) >= 0) {
      console.log(`${pkg} ${cur} >= ${min}, floor no longer needed (${advisory})`);
    } else {
      console.log(`${pkg} ${cur} < ${min}, applying floor (${advisory})`);
      ws.overrides[pkg] = min;
      changed = true;
    }
  }
  if (changed) writeFileSync('pnpm-workspace.yaml', stringify(ws));
  process.exit(changed ? 10 : 0);
}

if (process.argv[2] === '--selftest') {
  const assert = (c, m) => { if (!c) { console.error('FAIL', m); process.exit(1); } };
  assert(cmp('8.5.0', '8.9.0') < 0, '8.5.0 < 8.9.0');
  assert(cmp('10.3.1', '10.3.1') === 0, 'equal');
  assert(cmp('10.10.0', '10.9.0') > 0, '10.10 > 10.9');
  assert(cmp('8.9.0-rc.1', '8.9.0') === 0, 'prerelease stripped');
  console.log('selftest ok');
} else {
  main();
}
