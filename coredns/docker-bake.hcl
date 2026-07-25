variable "TAG" {
  default = "latest"
}

group "default" {
  targets = ["coredns"]
}

target "coredns" {
  context    = "."
  dockerfile = "Dockerfile"
  tags       = ["coredns:${TAG}"]
  platforms  = [
    "linux/amd64",
    "linux/arm64",
    "linux/arm/v7",
  ]
}
