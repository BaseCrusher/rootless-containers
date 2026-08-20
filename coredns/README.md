# coredns

[CoreDNS](https://coredns.io/) with the `acmednschallenge`, `traefik`, and
`records` plugins, taking its whole configuration from environment variables
instead of a mounted Corefile.

## How it differs from `coredns/coredns`

| | official image | this image |
| --- | --- | --- |
| Plugins | upstream set | upstream set **+** `acmednschallenge`, `traefik`, `records` |
| Corefile | mounted, read from `/Corefile` | generated at startup from `COREDNS_*` env vars |
| PID 1 | `coredns` | `container-supervisor`, which runs the generator then CoreDNS |
| Working dir | `/` | `/home/nonroot` |
| Base | `gcr.io/distroless/static-debian12:nonroot` | `gcr.io/distroless/static-debian13:nonroot` |

Everything else is stock CoreDNS: same Corefile syntax, same upstream plugins,
same `-conf`/`-dns.port` flags.

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| CoreDNS | [coredns/coredns](https://github.com/coredns/coredns) | `COREDNS_VERSION` |
| `acmednschallenge` plugin | [BaseCrusher/coredns-acmednschallenge](https://github.com/BaseCrusher/coredns-acmednschallenge) | `plugins.json` |
| `traefik` plugin | [BaseCrusher/coredns-traefik](https://github.com/BaseCrusher/coredns-traefik) | `plugins.json` |
| `records` plugin | [coredns/records](https://github.com/coredns/records) | `plugins.json` |
| `corefile-gen` | [BaseCrusher/coredns-envvar-corefile](https://github.com/BaseCrusher/coredns-envvar-corefile) | `COREFILE_GEN_VERSION` |
| `container-supervisor` | [BaseCrusher/container-supervisor](https://github.com/BaseCrusher/container-supervisor) | `SUPERVISOR_VERSION` |

### Patched Go modules

A CoreDNS release ships whatever dependency versions it was cut with, so an
advisory published afterwards fails the Trivy scan with no upstream release to
bump to. `modules.json` lists a floor for those modules:

```json
[
  {
    "module": "google.golang.org/grpc",
    "min": "v1.82.1",
    "advisory": "GHSA-hrxh-6v49-42gf"
  }
]
```

The build raises each module to `min` only if CoreDNS pins something older —
once a CoreDNS release carries that version or newer the entry does nothing but
log `override no longer needed`, so a stale entry can never drag a dependency
back down. An entry for a module CoreDNS no longer depends on fails the build.
`[]` is a valid, empty list.

## Usage

No Corefile to mount — configure it with `COREDNS_*` environment variables:

```sh
docker run --rm -p 53:53/udp -p 53:53/tcp \
  -e COREDNS_MYZONE_ZONE=example.org \
  -e COREDNS_MYZONE__file=db.example.org \
  -e COREDNS_MYZONE__log= \
  ghcr.io/basecrusher/rootless-containers/coredns:v1.14.7-3.2
```

That produces and runs:

```
example.org:53 {
    file db.example.org
    log
}
```

At startup
[`corefile-gen`](https://github.com/BaseCrusher/coredns-envvar-corefile) renders
the `COREDNS_*` variables into `/home/nonroot/config/Corefile`, then CoreDNS
starts against it — sequenced by
[`container-supervisor`](https://github.com/BaseCrusher/container-supervisor) as
PID 1. If the Corefile cannot be written the container exits instead of starting
CoreDNS. CoreDNS's log lines reach `docker logs` unprefixed, exactly as from the
official image; lines tagged `[supervisor]` come from the supervisor itself.

In short: one group per server block, `_ZONE` and `_PORT` for the header, `__`
for a directive inside the block and one more `__` per nesting level, empty
value for a bare directive. See
[coredns-envvar-corefile](https://github.com/BaseCrusher/coredns-envvar-corefile)
for the full syntax.

Because the generator always writes that path, mounting your own Corefile over
it does not work: a writable mount is overwritten, a `:ro` one fails the write
and stops the container.

`/home/nonroot` is the working directory, so relative paths in the Corefile
(zone files, keys) resolve there — mount those in, readable by uid 65532. Mount
individual files or a subdirectory; mounting over `/home/nonroot` itself hides
the binaries and the container will not start. There is no shell in the image, so `docker exec` and shell-form health
checks do not work; debug with `docker logs`, CoreDNS's own `health` and
`prometheus` plugins, or the `-debug` image below.

### Ports

- `53/tcp`, `53/udp`

## Images

Every push to `main` touching this folder publishes both tags for all three
platforms:

| Tag | Base | Notes |
| --- | --- | --- |
| `:v1.14.7-3.2`, `:v1.14.7-3`, `:latest` | `gcr.io/distroless/static-debian13:nonroot` | what you want in production |
| `:v1.14.7-3.2-debug`, `:v1.14.7-3-debug`, `:latest-debug` | `gcr.io/distroless/static-debian13:debug-nonroot` | identical, plus a busybox shell at `/busybox/sh` for `docker exec` |

All under `ghcr.io/basecrusher/rootless-containers/coredns`. Tags are
`<version>-Y.Z`: `<version>` is `COREDNS_VERSION` (what gets built, so the two
can't drift), `Y.Z` is `IMAGE_REVISION` — `Y` for breaking repackaging of the
same CoreDNS, `Z` for fixes that don't. `<version>-Y` is a rolling tag that
always points at the newest `Z` of that revision. The workflow aborts before
building any tag that isn't `<version>-Y` or `<version>-Y.Z` (see the repo
README). `latest` follows `main`.

The workflow lints the Dockerfile with droast before building and scans the
pushed image with Trivy afterwards, failing on any fixable `HIGH` or `CRITICAL`
vulnerability. A nightly `coredns-scan` workflow rescans `latest` and
`latest-debug` without rebuilding.

## Cross-platform builds

```sh
cd coredns && docker buildx bake
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `coredns` | `${REGISTRY}/coredns:${COREDNS_VERSION}-${IMAGE_REVISION}`, `:${COREDNS_VERSION}-<Y>`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |
| `coredns-debug` | `${REGISTRY}/coredns:${COREDNS_VERSION}-${IMAGE_REVISION}-debug`, `:${COREDNS_VERSION}-<Y>-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY`, `COREDNS_VERSION` and `IMAGE_REVISION` (default `4.0`) are bake
variables — override any from the environment
(`COREDNS_VERSION=v1.14.5 docker buildx bake …`).

