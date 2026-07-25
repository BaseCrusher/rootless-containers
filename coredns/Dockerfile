FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS build
ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

ARG COREDNS_VERSION=v1.14.6
ARG TRAEFIK_REF=v1.0.0
ARG ACMEDNSCHALLENGE_REF=v1.0.0
ARG RECORDS_REF=a3157e710d9e57c75e4950a3750228f3ed9bb47a

WORKDIR /coredns

RUN apt-get update && apt-get install -y \
    ca-certificates \
    libcap2-bin

RUN git clone --depth 1 --branch $COREDNS_VERSION https://github.com/coredns/coredns.git .

RUN set -e; \
    for spec in "traefik|https://github.com/BaseCrusher/coredns-traefik|$TRAEFIK_REF" \
                "acmednschallenge|https://github.com/BaseCrusher/coredns-acmednschallenge|$ACMEDNSCHALLENGE_REF" \
                "records|https://github.com/coredns/records|$RECORDS_REF"; do \
        name=${spec%%|*}; rest=${spec#*|}; url=${rest%|*}; ref=${rest##*|}; \
        mkdir -p plugin/$name; \
        git -C plugin/$name init -q; \
        git -C plugin/$name fetch -q --depth 1 $url $ref; \
        git -C plugin/$name checkout -q FETCH_HEAD; \
        rm -rf plugin/$name/.git plugin/$name/go.mod plugin/$name/go.sum; \
    done

RUN sed -i '1itraefik:traefik' plugin.cfg \
    && sed -i '2iacmednschallenge:acmednschallenge' plugin.cfg \
    && sed -i '3irecords:records' plugin.cfg \
    && sed -i '/^dnssec:dnssec$/d' plugin.cfg \
    && sed -i '1idnssec:dnssec' plugin.cfg

RUN GOFLAGS="-buildvcs=false" make gen \
    && GOFLAGS="-buildvcs=false" go mod tidy \
    && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
       GOFLAGS="-buildvcs=false" make

WORKDIR /coredns_temp
RUN mv /coredns/coredns ./coredns
RUN setcap cap_net_bind_service=+ep coredns
COPY startup.sh ./startup.sh
COPY tls_cert_template.mustache tls_cert_template.mustache
RUN chmod 777 ./startup.sh
RUN mkdir bin
RUN mkdir config
RUN for cmd in ash sh cat rm ls wc awk grep mkdir basename; do \
        ln -s /home/nonroot/bin/busybox ./bin/$cmd; \
    done
RUN chown 65532:65532 -R .

FROM busybox:stable-musl AS busybox

FROM gcr.io/distroless/static-debian12:nonroot
ENV PATH=/home/nonroot/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
SHELL ["/home/nonroot/bin/ash"]
COPY --from=build /coredns_temp/ /home/nonroot/
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=busybox --chown=65532:65532 /bin/busybox /home/nonroot/bin/busybox
USER nonroot:nonroot


EXPOSE 53 53/udp
ENTRYPOINT ["/home/nonroot/bin/ash", "-c", "/home/nonroot/startup.sh"]