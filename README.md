# rootless-containers

A collection of Docker images I use myself. They are secure and rootless — each
runs as an unprivileged user, not root.

## Images

- [coredns](coredns/) — CoreDNS with the traefik, acmednschallenge and records
  plugins, cloned from git at pinned refs listed in `coredns/plugins.json`.
  No Corefile to mount — it is generated at startup from `COREDNS_*` env vars.

## Usage

```sh
docker run --rm ghcr.io/basecrusher/rootless-containers/coredns:v1.14.6
```

Or build it yourself:

```sh
docker buildx build -t <image> ./<image>
docker run --rm <image>
```

## Published images

Pushes to `main` that touch an image's folder build and publish it to this
repo's GitHub Container Registry, in two flavours:

| Tag | Base |
| --- | --- |
| `<version>`, `latest` | distroless — no shell, no package manager |
| `<version>-debug`, `latest-debug` | distroless debug — busybox shell for `docker exec` |

`<version>` is the version of the packaged upstream software (for coredns, its
`COREDNS_VERSION`), so the tag says exactly what is inside; `latest` follows
`main`.

## Cross-platform builds

Each folder carries a `docker-bake.hcl` listing the platforms that image is
built for, so one command produces every architecture:

```sh
docker buildx bake -f ./<image>/docker-bake.hcl
```

Built with [Buildx](https://docs.docker.com/build/) for multi-platform images.
