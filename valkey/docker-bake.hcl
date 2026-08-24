variable "REGISTRY" {
  default = "ghcr.io/basecrusher/rootless-containers"
}

# renovate: datasource=docker depName=valkey/valkey
variable "VALKEY_VERSION" {
  default = "9.1.1"
}

variable "IMAGE_REVISION" {
  default = "1.0"
}

group "default" {
  targets = ["valkey", "valkey-debug"]
}

target "valkey" {
  context    = "."
  dockerfile = "Dockerfile"
  args       = {
    VALKEY_VERSION = VALKEY_VERSION
  }
  tags       = [
    "${REGISTRY}/valkey:${VALKEY_VERSION}-${IMAGE_REVISION}",
    "${REGISTRY}/valkey:${VALKEY_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}",
    "${REGISTRY}/valkey:latest",
  ]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
    "linux/arm/v7",
  ]
}

target "valkey-debug" {
  inherits = ["valkey"]
  args     = {
    BASE_IMAGE = "gcr.io/distroless/base-debian13:debug-nonroot"
  }
  tags     = [
    "${REGISTRY}/valkey:${VALKEY_VERSION}-${IMAGE_REVISION}-debug",
    "${REGISTRY}/valkey:${VALKEY_VERSION}-${regex_replace(IMAGE_REVISION, "\\.[0-9]+$", "")}-debug",
    "${REGISTRY}/valkey:latest-debug",
  ]
}
