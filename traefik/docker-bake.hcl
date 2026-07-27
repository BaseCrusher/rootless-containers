variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

variable "TRAEFIK_VERSION" {
  default = "v3.7.9"
}

variable "GIT_SHA" {
  default = "dev"
}

group "default" {
  targets = ["traefik"]
}

target "traefik" {
  context    = "."
  dockerfile = "Dockerfile"
  args       = {
    TRAEFIK_VERSION = TRAEFIK_VERSION
  }
  tags       = [
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}",
    "${REGISTRY}/traefik:${GIT_SHA}",
    "${REGISTRY}/traefik:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
    "linux/arm/v7",
  ]
}
