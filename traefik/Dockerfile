FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS certwatcher

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /certwatcher

COPY certwatcher/ .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o certwatcher .

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS access-log-exporter

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /access-log-exporter

COPY access-log-exporter/ .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o access-log-exporter .

FROM --platform=$BUILDPLATFORM debian:trixie-slim AS build

ARG TARGETARCH
ARG TARGETVARIANT
ARG TRAEFIK_VERSION
ARG SUPERVISOR_VERSION
ARG CROWDSEC_PLUGIN_VERSION

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

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
COPY --from=access-log-exporter /access-log-exporter/access-log-exporter ./access-log-exporter
COPY certs.tmpl ./certs.tmpl
RUN mkdir -p config/dynamic
RUN mkdir -p plugins-local/src/github.com/maxlerebourg/crowdsec-bouncer-traefik-plugin \
    && curl -fsSL https://github.com/maxlerebourg/crowdsec-bouncer-traefik-plugin/archive/refs/tags/${CROWDSEC_PLUGIN_VERSION:?}.tar.gz \
    | tar -xz --strip-components=1 -C plugins-local/src/github.com/maxlerebourg/crowdsec-bouncer-traefik-plugin

FROM debian:trixie-slim AS busybox

RUN apt-get update && apt-get install -y --no-install-recommends busybox-static

RUN mkdir -p /busybox_root/bin \
    && cp /bin/busybox /busybox_root/bin/busybox \
    && for applet in $(/bin/busybox --list); do \
         [ "$applet" = busybox ] || ln -sf /bin/busybox "/busybox_root/bin/$applet"; \
       done

FROM scratch AS final
COPY --from=build --chown=65532:65532 /traefik_temp/ /home/nonroot/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY supervisor.yml /container-supervisor/config.yml
USER 65532:65532
WORKDIR /home/nonroot

EXPOSE 80 443
ENTRYPOINT ["/home/nonroot/container-supervisor"]

FROM final AS debug
COPY --from=busybox /busybox_root/bin/ /bin/
