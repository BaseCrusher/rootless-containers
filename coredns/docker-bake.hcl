variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=github-releases depName=coredns/coredns
variable "COREDNS_VERSION" {
  default = "v1.14.6"
}

# renovate: datasource=github-releases depName=BaseCrusher/container-supervisor
variable "SUPERVISOR_VERSION" {
  default = "v1.4.2"
}

variable "IMAGE_REVISION" {
  default = "1.0"
}

group "default" {
  targets = ["coredns", "coredns-debug"]
}

target "coredns" {
  context    = "."
  dockerfile = "Dockerfile"
  args       = {
    COREDNS_VERSION    = COREDNS_VERSION
    SUPERVISOR_VERSION = SUPERVISOR_VERSION
  }
  tags       = [
    "${REGISTRY}/coredns:${COREDNS_VERSION}-${IMAGE_REVISION}",
    "${REGISTRY}/coredns:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
    "linux/arm/v7",
  ]
}

target "coredns-debug" {
  inherits = ["coredns"]
  args     = {
    BASE_IMAGE = "gcr.io/distroless/static-debian13:debug-nonroot"
  }
  tags     = [
    "${REGISTRY}/coredns:${COREDNS_VERSION}-${IMAGE_REVISION}-debug",
    "${REGISTRY}/coredns:latest-debug",
  ]
}
