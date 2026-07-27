# rootless-containers

A collection of Docker images I use myself. They are secure and rootless — each
runs as an unprivileged user, not root.

## Images

- [coredns](coredns/) — CoreDNS with the traefik, acmednschallenge and records
  plugins, cloned from git at pinned refs listed in `coredns/plugins.json`.
  No Corefile to mount — it is generated at startup from `COREDNS_*` env vars.
- [traefik](traefik/) — Traefik on `scratch`: the upstream release binary, a CA
  bundle and nothing else. Binds ports 80/443 without root via
  `cap_net_bind_service`; the Docker provider goes through
  [docker-socket-proxy-go](https://github.com/BaseCrusher/docker-socket-proxy-go),
  the image never touches `docker.sock` itself. Ships an optional `certwatcher`
  that turns a directory of certificates — the ones `coredns` issues, say — into
  a dynamic configuration Traefik reloads on its own. Configured through
  `TRAEFIK_*` env vars only; flags passed as container arguments do not reach
  Traefik.

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
repo's GitHub Container Registry:

| Tag | Base |
| --- | --- |
| `<version>`, `latest` | the image's own base — no shell, no package manager |
| `<version>-debug`, `latest-debug` | distroless debug — busybox shell for `docker exec` |

The `-debug` flavour exists only for the distroless images; `traefik` is built
`FROM scratch` and has no debug variant.

`<version>` is the version of the packaged upstream software (for coredns, its
`COREDNS_VERSION`), so the tag says exactly what is inside; `latest` follows
`main`. Pin to `<version>` when a deployment must keep running a known release.

Before anything is built the workflow lints the Dockerfile with
[droast](https://github.com/immanuwell/dockerfile-roast); after the push it
scans the published image with
[Trivy](https://github.com/aquasecurity/trivy-action) and fails on any fixable
`HIGH` or `CRITICAL` vulnerability.

## Cross-platform builds

Each folder carries a `docker-bake.hcl` listing the platforms that image is
built for, so one command produces every architecture:

```sh
docker buildx bake -f ./<image>/docker-bake.hcl
```

Built with [Buildx](https://docs.docker.com/build/) for multi-platform images.
