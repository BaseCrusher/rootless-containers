variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=docker depName=crowdsecurity/crowdsec
variable "CROWDSEC_VERSION" {
  default = "v1.7.8"
}

group "default" {
  targets = ["crowdsec", "crowdsec-debug"]
}

target "crowdsec" {
  context    = "."
  dockerfile = "Dockerfile"
  args       = {
    CROWDSEC_VERSION = CROWDSEC_VERSION
  }
  tags       = [
    "${REGISTRY}/crowdsec:${CROWDSEC_VERSION}",
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
    "${REGISTRY}/crowdsec:${CROWDSEC_VERSION}-debug",
    "${REGISTRY}/crowdsec:latest-debug",
  ]
}
