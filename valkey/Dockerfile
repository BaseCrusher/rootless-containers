ARG BASE_IMAGE=gcr.io/distroless/base-debian13:nonroot
ARG VALKEY_VERSION=9.1.1

FROM valkey/valkey:${VALKEY_VERSION:?} AS upstream

RUN set -eux; \
    mkdir -p /staging; \
    { ldd /usr/local/bin/valkey-server; ldd /usr/local/bin/valkey-cli; } \
      | awk '/=> \//{ print $3 }' \
      | sort -u \
      | grep -Ev '/lib(c|m|ssl|crypto)\.so' \
      | while IFS= read -r lib; do \
          cp -L "$lib" "/staging/$(basename "$lib")"; \
        done

FROM ${BASE_IMAGE}
COPY --from=upstream /usr/local/bin/valkey-server /usr/local/bin/valkey-cli /usr/local/bin/
COPY --from=upstream /staging/ /usr/lib/
COPY --from=upstream --chown=65532:65532 /data/ /data/
USER nonroot:nonroot
WORKDIR /data

EXPOSE 6379
ENTRYPOINT ["/usr/local/bin/valkey-server"]
