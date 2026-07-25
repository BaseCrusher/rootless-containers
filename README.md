# rootless-containers

A collection of Docker images I use myself. They are secure and rootless — each
runs as an unprivileged user, not root.

## Images

- [coredns](coredns/) — CoreDNS with the traefik, acmednschallenge and records
  plugins, all cloned from git at pinned refs during the build.

## Usage

```sh
docker buildx build -t <image> ./<image>
docker run --rm <image>
```

## Cross-platform builds

Each folder carries a `docker-bake.hcl` listing the platforms that image is
built for, so one command produces every architecture:

```sh
docker buildx bake -f ./<image>/docker-bake.hcl
```

Built with [Buildx](https://docs.docker.com/build/) for multi-platform images.
