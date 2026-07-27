FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS certwatcher

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /certwatcher

COPY certwatcher/ .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o certwatcher .

FROM --platform=$BUILDPLATFORM debian:trixie-slim AS build

ARG TARGETARCH
ARG TARGETVARIANT
ARG TRAEFIK_VERSION
# renovate: datasource=github-releases depName=BaseCrusher/container-supervisor
ARG SUPERVISOR_VERSION=v1.2.0

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    curl \
    libcap2-bin

WORKDIR /traefik_temp

RUN curl -fsSL https://github.com/traefik/traefik/releases/download/${TRAEFIK_VERSION:?}/traefik_${TRAEFIK_VERSION}_linux_${TARGETARCH}${TARGETVARIANT}.tar.gz \
    | tar -xz traefik
RUN setcap cap_net_bind_service=+ep traefik
RUN curl -fsSL -o container-supervisor \
    https://github.com/BaseCrusher/container-supervisor/releases/download/$SUPERVISOR_VERSION/container-supervisor-linux-$TARGETARCH$TARGETVARIANT \
    && chmod +x container-supervisor
COPY --from=certwatcher /certwatcher/certwatcher ./certwatcher
COPY certs.tmpl ./certs.tmpl
RUN mkdir -p config/dynamic

FROM scratch
COPY --from=build --chown=65532:65532 /traefik_temp/ /home/nonroot/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY supervisor.yml /container-supervisor/config.yml
USER 65532:65532
WORKDIR /home/nonroot

EXPOSE 80 443
ENTRYPOINT ["/home/nonroot/container-supervisor"]
