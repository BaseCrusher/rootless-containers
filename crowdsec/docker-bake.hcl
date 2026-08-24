variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=docker depName=crowdsecurity/crowdsec
variable "CROWDSEC_VERSION" {
  default = "v1.7.8"
}

# renovate: datasource=github-releases depName=BaseCrusher/container-supervisor
variable "SUPERVISOR_VERSION" {
  default = "v1.8.0"
}

# renovate: datasource=github-releases depName=BaseCrusher/envelope
variable "ENVELOPE_VERSION" {
  default = "v1.1.1"
}

variable "IMAGE_REVISION" {
  default = "1.2"
}

group "default" {
  targets = ["crowdsec", "crowdsec-debug"]
}

target "crowdsec" {
  context    = "."
  dockerfile = "Dockerfile"
  args       = {
    CROWDSEC_VERSION   = CROWDSEC_VERSION
    SUPERVISOR_VERSION = SUPERVISOR_VERSION
    ENVELOPE_VERSION   = ENVELOPE_VERSION
  }
  tags       = [
    "${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${IMAGE_REVISION}",
    "${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}",
    "${REGISTRY}/crowdsec:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
    "linux/arm/v7",
  ]
}

target "crowdsec-debug" {
  inherits = ["crowdsec"]
  args     = {
    BASE_IMAGE = "gcr.io/distroless/static-debian13:debug-nonroot"
  }
  tags     = [
    "${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${IMAGE_REVISION}-debug",
    "${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}-debug",
    "${REGISTRY}/crowdsec:latest-debug",
  ]
}
