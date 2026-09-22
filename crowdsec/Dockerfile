ARG BASE_IMAGE=gcr.io/distroless/static-debian13:nonroot
ARG CROWDSEC_VERSION

FROM crowdsecurity/crowdsec:${CROWDSEC_VERSION:?} AS upstream

FROM --platform=$BUILDPLATFORM crowdsecurity/crowdsec:${CROWDSEC_VERSION:?} AS config

ARG TARGETARCH
ARG TARGETVARIANT
ARG SUPERVISOR_VERSION
ARG ENVELOPE_VERSION

WORKDIR /staging

RUN yq -i ' \
      del(.plugin_config) | \
      del(.config_paths.plugin_dir) | \
      del(.api.server.tls) | \
      .common.log_dir = "/var/lib/crowdsec/log" | \
      .api.server.online_client.credentials_path = "/etc/crowdsec/online_api_credentials.yaml" \
    ' etc/crowdsec/config.yaml

RUN mkdir -p var/lib/crowdsec/log supervisor_environment

RUN wget -q -O container-supervisor \
    https://github.com/BaseCrusher/container-supervisor/releases/download/$SUPERVISOR_VERSION/container-supervisor-linux-$TARGETARCH$TARGETVARIANT \
    && chmod +x container-supervisor

RUN wget -q -O envelope \
    https://github.com/BaseCrusher/envelope/releases/download/$ENVELOPE_VERSION/envelope-linux-$TARGETARCH$TARGETVARIANT \
    && chmod +x envelope

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS healthcheck-build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /healthcheck

COPY --from=healthcheck . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o healthcheck .

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS register-bouncer-build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /register-bouncer

COPY register-bouncer/ .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o register-bouncer .

FROM $BASE_IMAGE
COPY --from=upstream /usr/local/bin/crowdsec /usr/local/bin/cscli /usr/local/bin/
COPY --from=healthcheck-build /healthcheck/healthcheck /usr/local/bin/healthcheck
COPY --from=register-bouncer-build /register-bouncer/register-bouncer /usr/local/bin/register-bouncer
COPY --from=config /staging/container-supervisor /staging/envelope /usr/local/bin/
COPY --from=config --chown=65532:65532 /staging/etc/crowdsec/ /etc/crowdsec/
COPY --from=config --chown=65532:65532 /staging/var/lib/crowdsec/ /var/lib/crowdsec/
COPY --from=config --chown=65532:65532 /staging/supervisor_environment/ /container-supervisor/supervisor_environment/
COPY supervisor.yml /container-supervisor/config.yml
USER nonroot:nonroot

EXPOSE 8080 6060
ENTRYPOINT ["/usr/local/bin/container-supervisor"]
