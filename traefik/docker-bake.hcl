variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=github-releases depName=traefik/traefik
variable "TRAEFIK_VERSION" {
  default = "v3.7.9"
}

group "default" {
  targets = ["traefik", "traefik-debug"]
}

target "traefik" {
  context    = "."
  dockerfile = "Dockerfile"
  target     = "final"
  args       = {
    TRAEFIK_VERSION = TRAEFIK_VERSION
  }
  tags       = [
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}",
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
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}-debug",
    "${REGISTRY}/traefik:latest-debug",
  ]
}
