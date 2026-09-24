# crowdsec-web-ui

[CrowdSec Web UI](https://github.com/TheDuffman85/crowdsec-web-ui) on distroless
— a self-hosted dashboard for investigating alerts, managing decisions and
watching runtime metrics from one place. React/Vite front end, a Hono/Node.js
back end and a `better-sqlite3` database, all built from source and dropped onto
a distroless Node.js base. No shell, no package manager, no
`docker-entrypoint.sh`.

## How it differs from `theduffman85/crowdsec-web-ui`

| | official image | this image |
| --- | --- | --- |
| Base | `node:24-trixie-slim` | `gcr.io/distroless/nodejs24-debian13` |
| User | starts `root`, entrypoint `gosu`-drops to `node` | uid/gid `65532` (`nonroot`) by construction, never root |
| PID 1 | `docker-entrypoint.sh pnpm start` | `node dist/server/index.js` |
| `/app/data` permissions | fixed at every start by the root entrypoint | must be writable by uid `65532` up front |
| `gosu`, `pnpm`, entrypoint script | present | none |
| Shell, `apt` | present | none |

The application is the upstream one, unmodified — cloned at
`$CROWDSECWEBUI_VERSION`, built with the same steps as upstream (`vite build`,
`tsup`, the cached GeoNames dataset), then the built `dist`, the pruned
production `node_modules` and `geonames` are copied onto the distroless base. The
builder is `node:24.20.0-trixie-slim` — the same Node major and glibc (Debian 13)
as the `nodejs24-debian13` base — so the compiled `better-sqlite3` native module
loads as-is.

### The entrypoint script is gone

Upstream's `docker-entrypoint.sh` exists mainly to start as root, `chown
/app/data`, and `gosu`-drop to the `node` user. This image is `USER nonroot` from
the start, so that whole dance is dead code. The server runs directly as PID 1;
`pnpm start`'s `prestart` native-dependency rebuild is dropped with it (the module
is already built for the target platform).

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| crowdsec-web-ui | [TheDuffman85/crowdsec-web-ui](https://github.com/TheDuffman85/crowdsec-web-ui) | `CROWDSECWEBUI_VERSION` — the git ref cloned and built |

`CROWDSECWEBUI_VERSION` pins an upstream release tag (CalVer, e.g. `2026.8.3`);
Renovate tracks the release channel.

### Package floors

Because the app is built from upstream's pinned lockfile, a CVE in one of its
(often transitive) npm dependencies can't be fixed by bumping the app — the fix
has to force a newer version into the tree. `packages_override.json` is a list of
security floors, mirroring `coredns/modules.json`:

```json
[
  { "package": "undici", "min": "8.9.0", "advisory": "CVE-2026-13697" },
  { "package": "ip-address", "min": "10.3.1", "advisory": "CVE-2026-69192" }
]
```

Each entry is a **floor, not a pin**. After the initial install
`apply-floors.mjs` reads the lowest version of each package already in the tree;
only when it is below `min` does it write an entry into `pnpm-workspace.yaml`'s
`overrides:` (the pnpm 11 home for overrides, where upstream already pins some of
its own) and reinstall — otherwise it logs `floor no longer needed` and moves on.
Stale entries are therefore safe to leave (they never downgrade below the fix);
delete one once the build logs it as inert, i.e. upstream's own lockfile has
caught up. Add an object to cover a new advisory — no Dockerfile change.

## Usage

```sh
docker run --rm -p 3000:3000 \
  -v cwuidata:/app/data \
  ghcr.io/basecrusher/rootless-containers/crowdsec-web-ui:latest
```

The UI listens on `3000/tcp`. On first start it writes a `config.yaml` into
`/app/data`; connect it to your CrowdSec LAPI there or through the environment
variables below. Put it behind an HTTPS reverse proxy for anything but local use.

### Persist `/app/data`

`/app/data` holds the SQLite database and `config.yaml`, and is owned by uid
`65532`. Use a **named volume** — Docker seeds its ownership from the image, so
the mount stays writable by the container's user. An empty bind mount arrives
`root`-owned and the container cannot write to it; unlike the official image
there is no root entrypoint to `chown` it, so `chown 65532:65532` the host
directory yourself, or use a named volume.

### Configuration

Configure the LAPI connection through `CONFIG_*` environment variables (or edit
the generated `/app/data/config.yaml`):

```sh
docker run ... \
  -e CONFIG_INSTANCE_LAPI_URL=http://crowdsec:8080 \
  -e CONFIG_INSTANCE_LAPI_AUTH_USERNAME=webui \
  -e CONFIG_INSTANCE_LAPI_AUTH_PASSWORD=... \
  crowdsec-web-ui:latest
```

mTLS instead of a password uses the `CONFIG_INSTANCE_LAPI_AUTH_CERT_FILE` /
`CONFIG_INSTANCE_LAPI_AUTH_KEY_FILE` pair. See upstream for the full variable
list, OIDC SSO and TOTP options.

## Distroless caveats

- **No shell.** Shell-form and shell health checks do not work; use the `-debug`
  image below when you need a shell to `docker exec`. The image ships an opt-in
  binary `HEALTHCHECK` (the shared static `healthcheck` — a plain `GET`, non-`200`
  is unhealthy) against `/api/health`:

  ```
  HEALTHCHECK CMD ["/usr/local/bin/healthcheck", "http://localhost:3000/api/health"]
  ```

- **No `/tmp` writes as `nonroot`.** Mount a `tmpfs` if a feature needs one.

CA certificates and the timezone database come with the distroless base, so TLS
verification and `TZ` work.

### Ports

- `3000/tcp` — the web UI

Unprivileged, so no capability is needed on the binary.

## Images

| Tag | Base | Notes |
| --- | --- | --- |
| `:v<version>-1.1`, `:v<version>-1`, `:latest` | `nodejs24-debian13:nonroot` | no shell, no package manager |
| `:v<version>-1.1-debug`, `:v<version>-1-debug`, `:latest-debug` | `nodejs24-debian13:debug-nonroot` | identical, plus busybox at `/busybox/sh` |

All under `ghcr.io/basecrusher/rootless-containers/crowdsec-web-ui`. Tags are
`v<version>-Y.Z`: `<version>` is `CROWDSECWEBUI_VERSION` (the upstream release tag
built, e.g. `2026.8.3`), `v`-prefixed on the image tag for consistency with the
other images even though upstream's tag has no `v`. `Y.Z` is `IMAGE_REVISION` —
`Y` for breaking repackaging of the same version, `Z` for fixes that don't.
`v<version>-Y` is a rolling tag that always points at the newest `Z` of that
revision. `latest` follows `main`.

## Cross-platform builds

```sh
cd crowdsec-web-ui && docker buildx bake
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `crowdsec-web-ui` | `${REGISTRY}/crowdsec-web-ui:v${CROWDSECWEBUI_VERSION}-${IMAGE_REVISION}`, `:v${CROWDSECWEBUI_VERSION}-<Y>`, `:latest` | `linux/amd64`, `linux/arm64` |
| `crowdsec-web-ui-debug` | `${REGISTRY}/crowdsec-web-ui:v${CROWDSECWEBUI_VERSION}-${IMAGE_REVISION}-debug`, `:v${CROWDSECWEBUI_VERSION}-<Y>-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64` |

`REGISTRY`, `CROWDSECWEBUI_VERSION`, `IMAGE_REVISION` and `BASE_IMAGE` are bake
variables — override any from the environment. The `better-sqlite3` native module
is compiled per target platform in the builder stage; the healthcheck context
lives outside the folder, so a local `docker buildx bake` needs
`--allow=fs.read=../_shared/healthcheck`.

Only `amd64` and `arm64` are built: the Node.js base images
(`node:24-trixie-slim` and `distroless/nodejs24-debian13`) ship no 32-bit
`linux/arm/v7` variant, so that platform cannot be produced on this base.
