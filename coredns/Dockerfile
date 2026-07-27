ARG BASE_IMAGE=gcr.io/distroless/static-debian13:nonroot

FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT
ARG COREDNS_VERSION=v1.14.6
ARG COREFILE_GEN_VERSION=v1.0.3
ARG SUPERVISOR_VERSION=v1.2.0

WORKDIR /coredns

RUN apt-get update && apt-get install -y \
    ca-certificates \
    curl \
    jq \
    libcap2-bin

COPY plugins.json /plugins.json
COPY modules.json /modules.json

RUN git clone --depth 1 --branch $COREDNS_VERSION https://github.com/coredns/coredns.git .

RUN set -e; \
    jq -r '.[] | select(.repo) | "\(.name) \(.repo) \(.ref)"' /plugins.json \
    | while read -r name repo ref; do \
        mkdir -p plugin/$name; \
        git -C plugin/$name init -q; \
        git -C plugin/$name fetch -q --depth 1 $repo $ref; \
        git -C plugin/$name checkout -q FETCH_HEAD; \
        rm -rf plugin/$name/.git plugin/$name/go.mod plugin/$name/go.sum; \
    done

RUN set -e; \
    jq -r '.[] | "\(.name) \(if .before then "i" else "a" end) \(.before // .after)"' /plugins.json > /placements; \
    while read -r name mode anchor; do \
        [ "$anchor" != null ] || { echo "$name needs a 'before' or 'after'" >&2; exit 1; }; \
        sed -i "/^$name:/d" plugin.cfg; \
        grep -q "^$anchor:" plugin.cfg || { echo "plugin.cfg has no '$anchor:' anchor for $name" >&2; exit 1; }; \
        sed -i "/^$anchor:/$mode$name:$name" plugin.cfg; \
    done < /placements

RUN GOFLAGS="-buildvcs=false" make gen

RUN set -e; \
    export GOFLAGS="-buildvcs=false"; \
    jq -r '.[] | "\(.module) \(.min)"' /modules.json > /overrides; \
    while read -r module min; do \
        cur=$(go list -m -f '{{.Version}}' "$module"); \
        if [ "$(printf '%s\n%s\n' "$cur" "$min" | sort -V | tail -1)" = "$cur" ]; then \
            echo "$module $cur >= $min, override no longer needed"; \
        else \
            go get "$module@$min"; \
        fi; \
    done < /overrides

RUN GOFLAGS="-buildvcs=false" go mod tidy \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
       GOFLAGS="-buildvcs=false" make

WORKDIR /coredns_temp
RUN mv /coredns/coredns ./coredns
RUN setcap cap_net_bind_service=+ep coredns
RUN curl -fsSL -o corefile-gen \
    https://github.com/BaseCrusher/coredns-envvar-corefile/releases/download/$COREFILE_GEN_VERSION/coredns-envvar-corefile-linux-$TARGETARCH \
    && chmod +x corefile-gen
RUN curl -fsSL -o container-supervisor \
    https://github.com/BaseCrusher/container-supervisor/releases/download/$SUPERVISOR_VERSION/container-supervisor-linux-$TARGETARCH$TARGETVARIANT \
    && chmod +x container-supervisor
RUN mkdir config

FROM $BASE_IMAGE
COPY --from=build --chown=65532:65532 /coredns_temp/ /home/nonroot/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY supervisor.yml /container-supervisor/config.yml
USER nonroot:nonroot
WORKDIR /home/nonroot

EXPOSE 53 53/udp
ENTRYPOINT ["/home/nonroot/container-supervisor"]
