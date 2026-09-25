ARG BASE_IMAGE=gcr.io/distroless/nodejs24-debian13:nonroot
ARG CROWDSECWEBUI_VERSION

FROM node:24.21.0-trixie-slim AS builder

ARG CROWDSECWEBUI_VERSION

RUN apt-get update && apt-get install -y --no-install-recommends \
    python3 make g++ git ca-certificates \
    && rm -rf /var/lib/apt/lists/* \
    && npm install -g pnpm@11.17.0

WORKDIR /app

COPY packages_override.json apply-floors.mjs /

RUN git clone https://github.com/TheDuffman85/crowdsec-web-ui.git . \
    && git checkout "${CROWDSECWEBUI_VERSION:?}"

ENV NODE_ENV=production

RUN pnpm install --frozen-lockfile --prod=false

RUN node /apply-floors.mjs; rc=$?; \
    if [ "$rc" = "10" ]; then pnpm install --no-frozen-lockfile --prod=false; \
    elif [ "$rc" != "0" ]; then exit "$rc"; fi

RUN node scripts/download-geonames-data.mjs \
    && pnpm exec vite build \
    && pnpm exec tsup \
    && pnpm prune --prod \
    && mkdir -p /app/data

FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS healthcheck-build

ARG TARGETOS
ARG TARGETARCH
ARG TARGETVARIANT

WORKDIR /healthcheck

COPY --from=healthcheck . .

RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH GOARM=${TARGETVARIANT#v} \
    go build -o healthcheck .

FROM ${BASE_IMAGE}

ENV NODE_ENV=production

WORKDIR /app

COPY --from=healthcheck-build /healthcheck/healthcheck /usr/local/bin/healthcheck
COPY --from=builder /app/dist ./dist
COPY --from=builder /app/node_modules ./node_modules
COPY --from=builder /app/geonames ./geonames
COPY --from=builder /app/package.json ./package.json
COPY --from=builder --chown=65532:65532 /app/data ./data

USER nonroot:nonroot

EXPOSE 3000
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/healthcheck", "http://localhost:3000/api/health"]

CMD ["dist/server/index.js"]
