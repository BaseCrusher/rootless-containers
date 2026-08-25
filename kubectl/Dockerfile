ARG BASE_IMAGE=gcr.io/distroless/static-debian13:nonroot
ARG KUBECTL_VERSION=v1.36.4

FROM registry.k8s.io/kubectl:${KUBECTL_VERSION:?} AS upstream

FROM ${BASE_IMAGE}
COPY --from=upstream /bin/kubectl /bin/kubectl
USER nonroot:nonroot
ENTRYPOINT ["/bin/kubectl"]
