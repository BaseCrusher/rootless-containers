# coredns

[CoreDNS](https://coredns.io/) built from source with the `traefik`,
`acmednschallenge` and `records` plugins, plus `dnssec` moved to the front of
the plugin chain.

## Sources

The build clones everything at a pinned ref; override any of them with
`--build-arg`.

| Component | Repository | Build arg | Default ref |
| --- | --- | --- | --- |
| coredns | https://github.com/coredns/coredns | `COREDNS_VERSION` | `v1.14.2` |
| traefik | https://github.com/BaseCrusher/coredns-traefik | `TRAEFIK_REF` | `e55898ac56e99c049a3310a98881ad8f4b4ca2d8` |
| acmednschallenge | https://github.com/BaseCrusher/coredns-acmednschallenge | `ACMEDNSCHALLENGE_REF` | `a74d1527c4b9cd557112c588d89132d47a798046` |
| records | https://github.com/coredns/records | `RECORDS_REF` | `a3157e710d9e57c75e4950a3750228f3ed9bb47a` |

Plugins are checked out into `plugin/<name>/` inside the CoreDNS tree (their
own `go.mod`/`go.sum` are dropped so they build as part of the CoreDNS module)
and registered by prepending them to `plugin.cfg`.

Runs as `nonroot` (uid/gid 65532) on `gcr.io/distroless/static-debian12`, with
`cap_net_bind_service` set on the binary so it can bind port 53 without root. A
static busybox provides a minimal shell for `startup.sh`.

## Build

```sh
docker buildx build -t coredns .
docker run --rm -p 53:53/udp coredns
```

## Cross-platform build

`docker-bake.hcl` in this folder declares one target, `coredns`, built for
`linux/amd64`, `linux/arm64` and `linux/arm/v7`. It is the `default` group, so:

```sh
docker buildx bake -f docker-bake.hcl
TAG=1.2.3 docker buildx bake -f docker-bake.hcl
```

Relative paths resolve against the bake file, so it also works from the repo
root as `docker buildx bake -f ./coredns/docker-bake.hcl`.

The Dockerfile cross-compiles: the build stage is pinned to `$BUILDPLATFORM`
and Go is pointed at `$TARGETOS`/`$TARGETARCH`/`$TARGETVARIANT`, so adding a
platform to the bake file is enough — no emulation needed.

## Ports

- `53/tcp`, `53/udp`
