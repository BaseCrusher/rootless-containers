# kubectl

[`kubectl`](https://kubernetes.io/docs/reference/kubectl/) on distroless — one
static binary and the base's CA bundle, nothing else. No shell, no package
manager.

## How it differs from `registry.k8s.io/kubectl`

| | official image | this image |
| --- | --- | --- |
| Base | distroless static | `gcr.io/distroless/static-debian13` |
| User | `root` (uid 0) | uid/gid `65532` (`nonroot`), never root |
| Contents | `kubectl`, CA certificates | `kubectl`, CA certificates |

The binary is the upstream one, unmodified — `COPY`'d out of
`registry.k8s.io/kubectl:$KUBECTL_VERSION`. It is a fully static Go binary (that
is how it runs on distroless static upstream). The base ships the CA bundle at
`/etc/ssl/certs`, so TLS to a public API server works without embedding the CA
in the kubeconfig.

## What's inside

| Component | Repo | Pinned by |
| --- | --- | --- |
| `kubectl` | [kubernetes/kubernetes](https://github.com/kubernetes/kubernetes) | `KUBECTL_VERSION` — copied out of `registry.k8s.io/kubectl:$KUBECTL_VERSION` |

## Usage

```sh
docker run --rm \
  -v ~/.kube/config:/home/nonroot/.kube/config:ro \
  ghcr.io/basecrusher/rootless-containers/kubectl:v1.36.4-1.0 get pods
```

Arguments after the image name are passed to `kubectl`. `HOME` is
`/home/nonroot`, so mount your kubeconfig at `/home/nonroot/.kube/config`, or
set `KUBECONFIG`:

```sh
docker run --rm -e KUBECONFIG=/kube.yaml -v ~/.kube/config:/kube.yaml:ro \
  ghcr.io/basecrusher/rootless-containers/kubectl:v1.36.4-1.0 get nodes
```

## Distroless caveats

- **No shell, no `/tmp` writes as `nonroot`.** `docker exec` and shell health
  checks do not work; mount a `tmpfs` if a plugin needs scratch space. Use the
  `-debug` image below when you need a shell.

CA certificates come with the distroless base, so TLS verification to the API
server works out of the box.

### Ports

None — `kubectl` is a client.

## Images

| Tag | Base | Notes |
| --- | --- | --- |
| `:v1.36.4-1.0`, `:v1.36.4-1`, `:latest` | `static-debian13:nonroot` | no shell, no package manager |
| `:v1.36.4-1.0-debug`, `:v1.36.4-1-debug`, `:latest-debug` | `static-debian13:debug-nonroot` | identical, plus busybox at `/busybox/sh` |

All under `ghcr.io/basecrusher/rootless-containers/kubectl`. Tags are
`<version>-Y.Z`: `<version>` is `KUBECTL_VERSION` (the upstream image the binary
is copied from, so the tag and what is inside cannot drift), `Y.Z` is
`IMAGE_REVISION` — `Y` for breaking repackaging of the same kubectl, `Z` for
fixes that don't. `<version>-Y` is a rolling tag that always points at the
newest `Z` of that revision. `latest` follows `main`.

## Cross-platform builds

```sh
cd kubectl && docker buildx bake
```

| Target | Tag | Platforms |
| --- | --- | --- |
| `kubectl` | `${REGISTRY}/kubectl:${KUBECTL_VERSION}-${IMAGE_REVISION}`, `:${KUBECTL_VERSION}-<Y>`, `:latest` | `linux/amd64`, `linux/arm64` |
| `kubectl-debug` | `${REGISTRY}/kubectl:${KUBECTL_VERSION}-${IMAGE_REVISION}-debug`, `:${KUBECTL_VERSION}-<Y>-debug`, `:latest-debug` | `linux/amd64`, `linux/arm64` |

`REGISTRY`, `KUBECTL_VERSION`, `IMAGE_REVISION` and `BASE_IMAGE` are bake
variables — override any from the environment (`KUBECTL_VERSION=v1.35.0 docker
buildx bake …`). Nothing is compiled: the binary comes from the upstream image
for the target platform. Upstream publishes `amd64` and `arm64` only, so there
is no `linux/arm/v7`.
