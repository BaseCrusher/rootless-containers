# rootless-containers

A collection of Docker images I use myself. They are secure and rootless — each
runs as an unprivileged user, not root.

## Images

- [coredns](coredns/) — CoreDNS with the acmednschallenge and records
  plugins, cloned from git at pinned refs listed in `coredns/plugins.json`.
  No Corefile to mount — it is generated at startup from `COREDNS_*` env vars.
  `coredns/modules.json` sets version floors for Go dependencies with an
  advisory but no CoreDNS release to bump to.
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
docker buildx bake -f ./<image>/docker-bake.hcl
```

## Published images

Pushes to `main` that touch an image's folder build and publish it to this
repo's GitHub Container Registry:

| Tag | Base |
| --- | --- |
| `<version>`, `latest` | the image's own base — no shell, no package manager |
| `<version>-debug`, `latest-debug` | same base plus a busybox shell for `docker exec` |

Every image ships a `-debug` flavour. The distroless images get it from the
`:debug-nonroot` base; `traefik` is built `FROM scratch`, so its debug variant
copies in Debian's static busybox with the applets symlinked into `/bin`.

`<version>` is the version of the packaged upstream software (for coredns, its
`COREDNS_VERSION`), so the tag says exactly what is inside; `latest` follows
`main`. Pin to `<version>` when a deployment must keep running a known release.

Before anything is built the workflow lints the Dockerfile with
[droast](https://github.com/immanuwell/dockerfile-roast); after the push it
scans the published image with
[Trivy](https://github.com/aquasecurity/trivy-action) and fails on any fixable
`HIGH` or `CRITICAL` vulnerability.

Each image also rebuilds nightly, so a vulnerability published against a base
layer after the fact still turns into a failing Trivy scan. Pull requests run
the same lint and build without publishing, which is what validates a
dependency bump before it reaches `main`.

Each image also gets its own nightly `trivy-<image>` workflow that scans its
published tags — including `latest-debug` — without rebuilding, so the registry
still gets a verdict on a night the build itself breaks, and a failure names the
image directly. Those are thin callers of the reusable `trivy` workflow, which
takes the container folder name and scans every tag of it.

The shared steps live in `.github/common` as composite actions: `build` lints
and bakes a container folder, `scan` applies the vulnerability policy. Each
workflow in `.github/workflows` is then just its triggers plus a call to those.

## Dependency updates

[Renovate](https://docs.renovatebot.com) opens a pull request per update. It
tracks the version variables in each `docker-bake.hcl` and `Dockerfile` (via
the `# renovate:` annotation above each one), the plugin pins in
`coredns/plugins.json`, base images, and GitHub Actions. Configuration lives in
`renovate.json`.

## Cross-platform builds

Each folder carries a `docker-bake.hcl` listing the platforms that image is
built for, so one command produces every architecture:

```sh
docker buildx bake -f ./<image>/docker-bake.hcl
```

Built with [Buildx](https://docs.docker.com/build/) for multi-platform images.
