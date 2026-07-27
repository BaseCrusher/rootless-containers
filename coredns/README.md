# coredns

[CoreDNS](https://coredns.io/) with the `traefik`, `acmednschallenge` and
`records` plugins, taking its whole configuration from environment variables
instead of a mounted Corefile.

## How it differs from `coredns/coredns`

| | official image | this image |
| --- | --- | --- |
| Plugins | upstream set | upstream set **+** `traefik`, `acmednschallenge`, `records` |
| Corefile | mounted, read from `/Corefile` | generated at startup from `COREDNS_*` env vars |
| PID 1 | `coredns` | `container-supervisor`, which runs the generator then CoreDNS |
| Working dir | `/` | `/home/nonroot` |
| Base | `gcr.io/distroless/static-debian12:nonroot` | `gcr.io/distroless/static-debian13:nonroot` |

Everything else is stock CoreDNS: same Corefile syntax, same upstream plugins,
same `-conf`/`-dns.port` flags.

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| CoreDNS | [coredns/coredns](https://github.com/coredns/coredns) | `COREDNS_VERSION` (`v1.14.6`) |
| `traefik` plugin | [BaseCrusher/coredns-traefik](https://github.com/BaseCrusher/coredns-traefik) | `plugins.json` |
| `acmednschallenge` plugin | [BaseCrusher/coredns-acmednschallenge](https://github.com/BaseCrusher/coredns-acmednschallenge) | `plugins.json` |
| `records` plugin | [coredns/records](https://github.com/coredns/records) | `plugins.json` |
| `corefile-gen` | [BaseCrusher/coredns-envvar-corefile](https://github.com/BaseCrusher/coredns-envvar-corefile) | `COREFILE_GEN_VERSION` (`v1.0.3`) |
| `container-supervisor` | [BaseCrusher/container-supervisor](https://github.com/BaseCrusher/container-supervisor) | `SUPERVISOR_VERSION` (`v1.2.0`) |

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
| `:v1.14.6`, `:<commit-sha>`, `:latest` | `gcr.io/distroless/static-debian13:nonroot` | what you want in production |
| `:v1.14.6-debug`, `:<commit-sha>-debug`, `:latest-debug` | `gcr.io/distroless/static-debian13:debug-nonroot` | identical, plus a busybox shell at `/busybox/sh` for `docker exec` |

All under `ghcr.io/basecrusher/rootless-containers/coredns`. The version tag is
`COREDNS_VERSION` from `docker-bake.hcl`, which is also what gets built, so the
two can't drift; `latest` follows `main`.

Every push is also tagged with its full commit sha, so a given image always has
one tag that is never reused — `:v1.14.6` and `:latest` both move when the image
is rebuilt from the same CoreDNS release, the sha tag does not. Pin to it when
you need a deployment to keep running exactly what it rolled out with.

## Cross-platform builds

```sh
docker buildx bake -f ./coredns/docker-bake.hcl
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `coredns` | `${REGISTRY}/coredns:${COREDNS_VERSION}`, `:${GIT_SHA}`, `:latest` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |
| `coredns-debug` | `${REGISTRY}/coredns:${COREDNS_VERSION}-debug`, `:${GIT_SHA}-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64`, `linux/arm/v7` |

`REGISTRY`, `COREDNS_VERSION` and `GIT_SHA` are bake variables — override any of
them from the environment (`COREDNS_VERSION=v1.14.5 docker buildx bake …`).
`GIT_SHA` defaults to `dev`, so a local bake does not need it; the workflow
passes `github.sha`.

