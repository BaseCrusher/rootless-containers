# coredns

[CoreDNS](https://coredns.io/) with the `acmednschallenge` and `records`
plugins, taking its whole configuration from environment variables instead of a
mounted Corefile.

## How it differs from `coredns/coredns`

| | official image | this image |
| --- | --- | --- |
| Plugins | upstream set | upstream set **+** `acmednschallenge`, `records` |
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
  ghcr.io/basecrusher/rootless-containers/coredns:v1.14.6
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
| `:v1.14.6`, `:latest` | `gcr.io/distroless/static-debian13:nonroot` | what you want in production |
| `:v1.14.6-debug`, `:latest-debug` | `gcr.io/distroless/static-debian13:debug-nonroot` | identical, plus a busybox shell at `/busybox/sh` for `docker exec` |

All under `ghcr.io/basecrusher/rootless-containers/coredns`. The version tag is
`COREDNS_VERSION` from `docker-bake.hcl`, which is also what gets built, so the
two can't drift; `latest` follows `main`.

The workflow lints the Dockerfile with droast before building and scans the
pushed image with Trivy afterwards, failing on any fixable `HIGH` or `CRITICAL`
vulnerability. A nightly `trivy-coredns` workflow rescans `latest` and
`latest-debug` without rebuilding.

## Cross-platform builds

```sh
docker buildx bake -f ./coredns/docker-bake.hcl
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `coredns` | `${REGISTRY}/coredns:${COREDNS_VERSION}`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |
| `coredns-debug` | `${REGISTRY}/coredns:${COREDNS_VERSION}-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY` and `COREDNS_VERSION` are bake variables — override either from the
environment (`COREDNS_VERSION=v1.14.5 docker buildx bake …`).

