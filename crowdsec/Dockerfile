ARG BASE_IMAGE=gcr.io/distroless/static-debian13:nonroot
# renovate: datasource=docker depName=crowdsecurity/crowdsec
ARG CROWDSEC_VERSION=v1.7.8

FROM crowdsecurity/crowdsec:${CROWDSEC_VERSION:?} AS upstream

FROM --platform=$BUILDPLATFORM crowdsecurity/crowdsec:${CROWDSEC_VERSION:?} AS config

ARG TARGETARCH
ARG TARGETVARIANT
# renovate: datasource=github-releases depName=BaseCrusher/container-supervisor
ARG SUPERVISOR_VERSION=v1.4.0
# renovate: datasource=github-releases depName=BaseCrusher/envelope
ARG ENVELOPE_VERSION=v1.1.0

WORKDIR /staging

RUN yq -i ' \
      del(.plugin_config) | \
      del(.config_paths.plugin_dir) | \
      del(.api.server.tls) | \
      .common.log_dir = "/var/lib/crowdsec/log" | \
      .api.server.online_client.credentials_path = "/etc/crowdsec/online_api_credentials.yaml" \
    ' etc/crowdsec/config.yaml

RUN mkdir -p var/lib/crowdsec/log

RUN wget -q -O container-supervisor \
    https://github.com/BaseCrusher/container-supervisor/releases/download/$SUPERVISOR_VERSION/container-supervisor-linux-$TARGETARCH$TARGETVARIANT \
    && chmod +x container-supervisor

RUN wget -q -O envelope \
    https://github.com/BaseCrusher/envelope/releases/download/$ENVELOPE_VERSION/envelope-linux-$TARGETARCH$TARGETVARIANT \
    && chmod +x envelope

FROM $BASE_IMAGE
COPY --from=upstream /usr/local/bin/crowdsec /usr/local/bin/cscli /usr/local/bin/
COPY --from=config /staging/container-supervisor /staging/envelope /usr/local/bin/
COPY --from=config --chown=65532:65532 /staging/etc/crowdsec/ /etc/crowdsec/
COPY --from=config --chown=65532:65532 /staging/var/lib/crowdsec/ /var/lib/crowdsec/
COPY supervisor.yml /container-supervisor/config.yml
USER nonroot:nonroot

EXPOSE 8080 6060
ENTRYPOINT ["/usr/local/bin/container-supervisor"]
