variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=github-releases depName=kubernetes/kubernetes
variable "KUBECTL_VERSION" {
  default = "v1.36.4"
}

variable "IMAGE_REVISION" {
  default = "1.0"
}

group "default" {
  targets = ["kubectl", "kubectl-debug"]
}

target "kubectl" {
  context    = "."
  dockerfile = "Dockerfile"
  args       = {
    KUBECTL_VERSION = KUBECTL_VERSION
  }
  tags       = [
    "${REGISTRY}/kubectl:${KUBECTL_VERSION}-${IMAGE_REVISION}",
    "${REGISTRY}/kubectl:${KUBECTL_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}",
    "${REGISTRY}/kubectl:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
  ]
}

target "kubectl-debug" {
  inherits = ["kubectl"]
  args     = {
    BASE_IMAGE = "gcr.io/distroless/static-debian13:debug-nonroot"
  }
  tags     = [
    "${REGISTRY}/kubectl:${KUBECTL_VERSION}-${IMAGE_REVISION}-debug",
    "${REGISTRY}/kubectl:${KUBECTL_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}-debug",
    "${REGISTRY}/kubectl:latest-debug",
  ]
}
