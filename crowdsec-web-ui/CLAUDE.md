# crowdsec-web-ui — build internals

The [CrowdSec Web UI](https://github.com/TheDuffman85/crowdsec-web-ui) built from
source and run rootless on distroless Node.js.

## Why build from source

Unlike the other images, nothing is lifted out of an upstream image — there is no
static binary to lift. It is a Node app (React/Vite front end, Hono/Node back
end, a `better-sqlite3` native module), so the git tree is cloned at
`CROWDSECWEBUI_VERSION` and built with upstream's own steps (`vite build`,
`tsup`, the cached GeoNames dataset). `CROWDSECWEBUI_VERSION` in
`docker-bake.hcl` pins an upstream release tag (CalVer, e.g. `2026.8.3`, no `v`
prefix); Renovate tracks the release channel via
`datasource=github-releases`.

The builder is `node:24.20.0-trixie-slim` — the same Node major and glibc
(Debian 13) as the `gcr.io/distroless/nodejs24-debian13` base — so the compiled
`better-sqlite3` module loads on the final image unchanged. Keeping the builder
on Debian 13 is what removes the usual native-module glibc-mismatch risk; do not
move it to bookworm while the base is debian13.

## No arm/v7

`amd64` and `arm64` only. Neither `node:24-trixie-slim` nor
`distroless/nodejs24-debian13` publishes a 32-bit `linux/arm/v7` variant (Node
dropped 32-bit arm), so that platform cannot be produced on this base — it is a
missing base layer, not a compile failure. Adding it back means abandoning the
distroless base for a self-built Node.

## The entrypoint script is gone

Upstream's `docker-entrypoint.sh` exists mainly to start as root, `chown
/app/data`, and `gosu`-drop to `node`. This image is `USER nonroot` (uid 65532)
from the start, so that is dead code: PID 1 is `node dist/server/index.js`
directly (the distroless entrypoint is `node`, the `CMD` is the script).
`pnpm start`'s `prestart` native-dependency rebuild goes with it — the module is
already built for the target platform. Consequence, documented in the README:
`/app/data` is not `chown`ed at runtime, so it must arrive writable by uid 65532
(a named volume, or a `chown 65532:65532`'d host dir).

## Package floors (`packages_override.json`)

The npm analogue of `coredns/modules.json`: a list of `{package, min, advisory}`
security floors for dependencies upstream's lockfile still pins below a fix.
`apply-floors.mjs` applies them between the initial `pnpm install
--frozen-lockfile` and the build.

Overrides go into **`pnpm-workspace.yaml`**, not `package.json`. pnpm 11 no
longer reads the `pnpm.overrides` field from `package.json` (it warns and ignores
it); the overrides home is `pnpm-workspace.yaml`, where upstream already keeps
its own `overrides:` block (it pins `ip-address` and `ws` there). The helper
merges into that map with the `yaml` package already in the tree — editing YAML
with sed would be fragile.

Each entry is a **floor, not a pin**, and self-inerting like coredns':

- After the frozen install, `apply-floors.mjs` reads the lowest installed version
  of `package` from the `node_modules/.pnpm/<name>@<version>/` directory names
  (`/` in a scoped name becomes `+`, and a `_peer` suffix is stripped, matching
  pnpm's on-disk layout).
- Only when that is **below** `min` does it write `overrides.<package> = <min>`
  into `pnpm-workspace.yaml` and exit `10`; the Dockerfile then reinstalls with
  `--no-frozen-lockfile`. A satisfied floor logs `floor no longer needed`; a
  package no longer in the tree logs `floor skipped`. Exit `0` (nothing applied)
  skips the reinstall, so the common case keeps the reproducible frozen install.
- The override is exact `min`, so a higher instance elsewhere in the tree is
  levelled to `min` — never below the fix. The check keys on the *lowest*
  installed version: once every instance is at or above `min`, the entry goes
  inert and can be deleted.

Version comparison is a small dotted-numeric `cmp` in the script (no `semver`
dependency); `node apply-floors.mjs --selftest` asserts it.

Not Renovate-tracked, by design (as with `modules.json`): a floor is set to a
CVE's fixed version once and left until upstream passes it — the daily Trivy scan
is what surfaces the next advisory to add.

## Health check

Reuses the shared static `healthcheck` binary (`_shared/healthcheck`, a stdlib
Go `GET` that exits non-zero on any non-`200`), built in its own stage and given
the app's `/api/health` endpoint. The context lives outside the folder, so a
local `docker buildx bake` needs `--allow=fs.read=../_shared/healthcheck`.
