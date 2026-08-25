variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=github-releases depName=traefik/traefik
variable "TRAEFIK_VERSION" {
  default = "v3.7.11"
}

# renovate: datasource=github-releases depName=BaseCrusher/container-supervisor
variable "SUPERVISOR_VERSION" {
  default = "v1.8.0"
}

# renovate: datasource=github-releases depName=maxlerebourg/crowdsec-bouncer-traefik-plugin
variable "CROWDSEC_PLUGIN_VERSION" {
  default = "v1.7.1"
}

variable "IMAGE_REVISION" {
  default = "2.0"
}

group "default" {
  targets = ["traefik", "traefik-debug"]
}

target "traefik" {
  context    = "."
  dockerfile = "Dockerfile"
  target     = "final"
  args       = {
    TRAEFIK_VERSION         = TRAEFIK_VERSION
    SUPERVISOR_VERSION      = SUPERVISOR_VERSION
    CROWDSEC_PLUGIN_VERSION = CROWDSEC_PLUGIN_VERSION
  }
  tags       = [
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}-${IMAGE_REVISION}",
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}",
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
    "${REGISTRY}/traefik:${TRAEFIK_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}-debug",
    "${REGISTRY}/traefik:latest-debug",
  ]
}
