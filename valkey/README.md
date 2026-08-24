# valkey

[Valkey](https://valkey.io/) on distroless — the upstream `valkey-server` and
`valkey-cli` binaries and the handful of libraries the base doesn't already
ship, and nothing else. No shell, no package manager, no `docker-entrypoint.sh`.

## How it differs from `valkey/valkey`

| | official image | this image |
| --- | --- | --- |
| Base | Debian | `gcr.io/distroless/base-debian13` |
| User | starts `root`, entrypoint drops to `valkey` (uid 999) | uid/gid `65532` (`nonroot`) by construction, never root |
| PID 1 | `docker-entrypoint.sh valkey-server` | `valkey-server` |
| Binaries | `valkey-server`, `-cli`, `-sentinel`, `-benchmark`, `-check-*`, `redis-*` aliases | `valkey-server`, `valkey-cli` |
| Shell, `apt`, entrypoint script | present | none |

The binaries are the upstream ones, unmodified — copied out of
`valkey/valkey:$VALKEY_VERSION` (the Debian image, not Alpine: it is
glibc-linked like the distroless base). `glibc`, `libssl` and `libcrypto` come
from the base; the four libraries it lacks — `libsystemd`, `libcap`, `libz`,
`libzstd` — are `ldd`-resolved from the upstream image and copied into
`/usr/lib`.

### The entrypoint script is gone

Upstream's `docker-entrypoint.sh` exists mainly to start as root, `chown /data`,
and drop to the `valkey` user. This image is `USER nonroot` from the start, so
that whole dance is dead code. What is left of the script — prepending
`valkey-server` to a flag/`.conf` argument — the `ENTRYPOINT` does for free:

```sh
docker run ghcr.io/basecrusher/rootless-containers/valkey:9.1.1-1.0 --appendonly yes
```

Arguments after the image name are appended to `valkey-server`. Pass a config
file the same way, by mounting it and naming its path.

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| `valkey-server`, `valkey-cli` | [valkey-io/valkey](https://github.com/valkey-io/valkey) | `VALKEY_VERSION` — copied out of `valkey/valkey:$VALKEY_VERSION` |

`protected-mode` is off in the upstream build (as it is in the official image),
so the server accepts connections in a container without extra configuration.

## Usage

```sh
docker run --rm -p 6379:6379 \
  -v valkeydata:/data \
  ghcr.io/basecrusher/rootless-containers/valkey:9.1.1-1.0
```

### Persist `/data`

`WORKDIR` is `/data`, owned by uid `65532`. Use a **named volume** — Docker
seeds its ownership from the image, so the mount stays writable by the
container's user. An empty bind mount arrives `root`-owned and the container
cannot write to it, so RDB/AOF saves fail:

```
Failed opening the temp RDB file ... (in server root dir /data) for saving: Permission denied
```

`chown 65532:65532` the host directory, or use a named volume.

### Configuration

There is no baked config file. Either pass flags:

```sh
docker run ... valkey:9.1.1-1.0 --maxmemory 256mb --maxmemory-policy allkeys-lru
```

or mount your own and point at it:

```sh
docker run ... -v ./valkey.conf:/etc/valkey/valkey.conf:ro \
  valkey:9.1.1-1.0 /etc/valkey/valkey.conf
```

## Distroless caveats

- **No shell.** `docker exec valkey valkey-cli ping` works (it is a binary in
  the image); shell-form commands and shell health checks do not. Use the
  `-debug` image below when you need a shell.
- **No `/tmp` writes as `nonroot`.** Mount a `tmpfs` if a feature needs one.

CA certificates and the timezone database come with the distroless base, so TLS
verification and `TZ` work.

### Ports

- `6379/tcp` — Valkey

Unprivileged, so no capability is needed on the binary.

## Images

| Tag | Base | Notes |
| --- | --- | --- |
| `:9.1.1-1.0`, `:9.1.1-1`, `:latest` | `base-debian13:nonroot` | no shell, no package manager |
| `:9.1.1-1.0-debug`, `:9.1.1-1-debug`, `:latest-debug` | `base-debian13:debug-nonroot` | identical, plus busybox at `/busybox/sh` |

All under `ghcr.io/basecrusher/rootless-containers/valkey`. Tags are
`<version>-Y.Z`: `<version>` is `VALKEY_VERSION` (the upstream image the binaries
are copied from, so the tag and what is inside cannot drift), `Y.Z` is
`IMAGE_REVISION` — `Y` for breaking repackaging of the same Valkey, `Z` for
fixes that don't. `<version>-Y` is a rolling tag that always points at the
newest `Z` of that revision. `latest` follows `main`.

## Cross-platform builds

```sh
cd valkey && docker buildx bake
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `valkey` | `${REGISTRY}/valkey:${VALKEY_VERSION}-${IMAGE_REVISION}`, `:${VALKEY_VERSION}-<Y>`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |
| `valkey-debug` | `${REGISTRY}/valkey:${VALKEY_VERSION}-${IMAGE_REVISION}-debug`, `:${VALKEY_VERSION}-<Y>-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY`, `VALKEY_VERSION`, `IMAGE_REVISION` and `BASE_IMAGE` are bake
variables — override any from the environment (`VALKEY_VERSION=9.1.0 docker
buildx bake …`). Nothing is compiled: the binaries and libraries come from the
upstream image for the target platform.
