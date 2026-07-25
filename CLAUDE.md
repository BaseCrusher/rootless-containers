# rootless-containers

Secure, rootless Docker images. Built with `docker buildx`.

## Conventions

- Every container lives in its own folder (`<image>/Dockerfile`).
- Every container folder has a `docker-bake.hcl` declaring its cross-platform
  targets, so the image builds for every supported architecture with
  `docker buildx bake`.
- Containers run as an unprivileged user, never root.
- Never add comments unless explicitly asked.

## Always update documentation

Any change to a container (add, remove, or modify) must update:
- The repo `README.md`.
- That container's own `README.md` in its folder, including its
  `docker-bake.hcl` targets and platforms.
