variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=github-releases depName=TheDuffman85/crowdsec-web-ui
variable "CROWDSECWEBUI_VERSION" {
  default = "2026.8.3"
}

variable "IMAGE_REVISION" {
  default = "1.1"
}

group "default" {
  targets = ["crowdsec-web-ui", "crowdsec-web-ui-debug"]
}

target "crowdsec-web-ui" {
  context    = "."
  contexts   = {
    healthcheck = "../_shared/healthcheck"
  }
  dockerfile = "Dockerfile"
  args       = {
    CROWDSECWEBUI_VERSION = CROWDSECWEBUI_VERSION
  }
  tags       = [
    "${REGISTRY}/crowdsec-web-ui:${CROWDSECWEBUI_VERSION}-${IMAGE_REVISION}",
    "${REGISTRY}/crowdsec-web-ui:${CROWDSECWEBUI_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}",
    "${REGISTRY}/crowdsec-web-ui:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
  ]
}

target "crowdsec-web-ui-debug" {
  inherits = ["crowdsec-web-ui"]
  args     = {
    BASE_IMAGE = "gcr.io/distroless/nodejs24-debian13:debug-nonroot"
  }
  tags     = [
    "${REGISTRY}/crowdsec-web-ui:${CROWDSECWEBUI_VERSION}-${IMAGE_REVISION}-debug",
    "${REGISTRY}/crowdsec-web-ui:${CROWDSECWEBUI_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}-debug",
    "${REGISTRY}/crowdsec-web-ui:latest-debug",
  ]
}
