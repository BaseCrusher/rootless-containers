variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=github-releases depName=traefik/traefik
variable "TRAEFIK_VERSION" {
  default = "v3.7.11"
}

# renovate: datasource=github-releases depName=BaseCrusher/container-supervisor
variable "SUPERVISOR_VERSION" {
  default = "v1.4.2"
}

variable "IMAGE_REVISION" {
  default = "1.0"
}

group "default" {
  targets = ["traefik", "traefik-debug"]
}

target "traefik" {
  context    = "."
  dockerfile = "Dockerfile"
  target     = "final"
  args       = {
    TRAEFIK_VERSION    = TRAEFIK_VERSION
    SUPERVISOR_VERSION = SUPERVISOR_VERSION
  }
  tags       = [
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}-${IMAGE_REVISION}",
    "${REGISTRY}/traefik:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
    "linux/arm/v7",
  ]
}

target "traefik-debug" {
  inherits = ["traefik"]
  target   = "debug"
  tags     = [
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}-${IMAGE_REVISION}-debug",
    "${REGISTRY}/traefik:latest-debug",
  ]
}
