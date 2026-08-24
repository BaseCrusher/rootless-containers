# rootless-containers

A collection of Docker images I use myself. They are secure and rootless — each
runs as an unprivileged user, not root.

## Images

- [coredns](coredns/) — CoreDNS with the acmednschallenge, traefik, and records
  plugins, cloned from git at pinned refs listed in `coredns/plugins.json`.
  No Corefile to mount — it is generated at startup from `COREDNS_*` env vars.
  `coredns/modules.json` sets version floors for Go dependencies with an
  advisory but no CoreDNS release to bump to.
- [crowdsec](crowdsec/) — CrowdSec on distroless: the release binaries lifted out
  of the official image, its preloaded hub and GeoLite2 databases, and no
  `docker_start.sh`. The env vars that script reads do not work here;
  `CROWDSEC_CONFIG_*` env vars are turned into `config.yaml.local` at startup,
  a `cscli` slot in `container-supervisor` runs one bootstrap command before the
  agent starts, and `docker exec cscli …` does the rest. Notification plugins
  are not included.
- [traefik](traefik/) — Traefik on `scratch`: the upstream release binary, a CA
  bundle and nothing else. Binds ports 80/443 without root via
  `cap_net_bind_service`; the Docker provider goes through
  [docker-socket-proxy-go](https://github.com/BaseCrusher/docker-socket-proxy-go),
  the image never touches `docker.sock` itself. Ships an optional `certwatcher`
  that turns a directory of certificates — the ones `coredns` issues, say — into
  a dynamic configuration Traefik reloads on its own, and an optional
  `access-log-exporter` that pushes the access log to CrowdSec over HTTP,
  identically on Swarm and Kubernetes. Configured through
  `TRAEFIK_*` env vars only; flags passed as container arguments do not reach
  Traefik.

## Usage

```sh
docker run --rm ghcr.io/basecrusher/rootless-containers/coredns:v1.14.7-3.2
```

Or build it yourself:

```sh
cd <image> && docker buildx bake
```

## Published images

Pushes to `main` that touch an image's folder build and publish it to this
repo's GitHub Container Registry:

| Tag | Base |
| --- | --- |
| `<version>-<image-rev>`, `<version>-Y`, `latest` | the image's own base — no shell, no package manager |
| `<version>-<image-rev>-debug`, `<version>-Y-debug`, `latest-debug` | same base plus a busybox shell for `docker exec` |

Every image ships a `-debug` flavour. The distroless images get it from the
`:debug-nonroot` base; `traefik` is built `FROM scratch`, so its debug variant
copies in Debian's static busybox with the applets symlinked into `/bin`.

### Tag format

Every non-floating tag is `<version>-Y.Z` or the rolling `<version>-Y`, and the
workflow **aborts before building** any image whose tags don't match — see the
`Enforce image tag format` step in `.github/common/build`. Only `latest` and
`dev` (each optionally `-debug`) are exempt.

- `<version>` — the packaged upstream software (for coredns, its
  `COREDNS_VERSION`), so the tag says exactly what is inside. Usually
  `vX.X.X`, but the format follows however that upstream versions itself.
- `Y` — the image revision's **major**: bump it when the same upstream version
  is repackaged with breaking changes to the image (extra software, a moved
  path, a changed default).
- `Z` — the image revision's **minor**: bump it for fixes that keep the current
  featureset (a base-layer CVE rebuild, a config tweak).

`Y.Z` is the `IMAGE_REVISION` bake variable (default `1.2`); `<version>` is the
per-image version variable. `<version>-Y` is a rolling tag that always points at
the newest `Z` of that revision. `latest` follows `main`. Pin to a full
`<version>-Y.Z` when a deployment must keep running a known build.

Before anything is built the workflow lints the Dockerfile with
[droast](https://github.com/immanuwell/dockerfile-roast); after the push it
scans the published image with
[Trivy](https://github.com/aquasecurity/trivy-action) and fails on any fixable
`HIGH` or `CRITICAL` vulnerability.

Each image also rebuilds nightly, so a vulnerability published against a base
layer after the fact still turns into a failing Trivy scan. Pull requests run
the same lint and build without publishing, which is what validates a
dependency bump before it reaches `main`.

Each image also gets its own nightly `<image>-scan` workflow that scans its
published tags — including `latest-debug` — without rebuilding, so the registry
still gets a verdict on a night the build itself breaks, and a failure names the
image directly. Those are thin callers of the `scan-published` composite
action, which takes the container folder name and scans both its published tags.

The shared steps live in `.github/common` as composite actions: `build` lints
and bakes a container folder, `scan` applies the vulnerability policy to one
image, and `scan-published` logs in and runs `scan` against both published tags.
Each workflow in `.github/workflows` is then just its triggers plus a call to
those.

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
cd <image> && docker buildx bake
```

Run it from the image's folder: each bake file declares `context = "."`, which
buildx resolves against the working directory, not against the bake file. That
is how CI builds them too — the workflow passes the folder as the build source.

Built with [Buildx](https://docs.docker.com/build/) for multi-platform images.
