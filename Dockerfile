# syntax=docker/dockerfile:1.27.1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    --mount=type=secret,id=https_proxy,required=false \
    sh -eu -c 'if [ -f /run/secrets/https_proxy ]; then export HTTPS_PROXY="$(cat /run/secrets/https_proxy)"; fi; npm ci'
COPY web/index.html web/tsconfig.json web/vite.config.ts ./
COPY web/scripts/ ./scripts/
COPY web/src/ ./src/
COPY web/public/ ./public/
# Public assets can retain a restrictive source checkout's mode (for example 0600).
# The runtime is non-root: normalize only the generated, publicly served tree.
RUN npm run build \
    && find dist -type d -exec chmod 0755 {} + \
    && find dist -type f -exec chmod 0644 {} +

FROM --platform=$BUILDPLATFORM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS go-build
ARG TARGETOS=linux
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=secret,id=https_proxy,required=false \
    sh -eu -c 'if [ -f /run/secrets/https_proxy ]; then export HTTPS_PROXY="$(cat /run/secrets/https_proxy)"; fi; go mod download'
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=secret,id=https_proxy,required=false \
    sh -eu -c 'if [ -f /run/secrets/https_proxy ]; then export HTTPS_PROXY="$(cat /run/secrets/https_proxy)"; fi; \
      CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
      go build -trimpath \
        -ldflags="-s -w -X github.com/kejilion/kejilion-panel/internal/version.Version=${VERSION}" \
        -o /out/paneld ./cmd/paneld; \
      CGO_ENABLED=0 GOOS="${TARGETOS}" GOARCH="${TARGETARCH}" \
      go build -trimpath \
        -ldflags="-s -w -X github.com/kejilion/kejilion-panel/internal/version.Version=${VERSION}" \
        -o /out/kejilion-agent ./cmd/kejilion-agent'

FROM scratch
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="KPanel" \
      org.opencontainers.image.description="Safe web management plane for kejilion.sh hosts" \
      org.opencontainers.image.url="https://hub.docker.com/r/kjlion/kejilion-panel" \
      org.opencontainers.image.source="https://github.com/kejilion/KPanel" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      io.kejilion.kpanel.update-freeze="1" \
      io.kejilion.script.revision="c3a8bd895f8878d9e4ced7592c91a20c974472a5" \
      io.kejilion.script.sha256="d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0"
COPY --from=go-build /out/paneld /paneld
COPY --from=go-build /out/kejilion-agent /release/kejilion-agent
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD --checksum=sha256:d76a3a267117674baf6911201723b2691f11d2964ecac08bfdf380c45485cdc0 \
    https://raw.githubusercontent.com/kejilion/sh/c3a8bd895f8878d9e4ced7592c91a20c974472a5/kejilion.sh \
    /release/kejilion.sh
COPY --from=web-build /src/web/dist /app/web
COPY VERSION /release/VERSION
COPY packaging/kejilion-app/kpanel.conf /release/kpanel.conf
COPY LICENSE /licenses/LICENSE
COPY NOTICE /licenses/NOTICE
COPY LICENSES/ /licenses/third-party/
COPY THIRD_PARTY_NOTICES.md /licenses/THIRD_PARTY_NOTICES.md
COPY TRADEMARKS.md /licenses/TRADEMARKS.md
COPY deploy/compose/compose.yml /release/compose.yml
COPY deploy/compose/direct-port.yml /release/direct-port.yml
COPY deploy/compose/.env.example /release/panel.env.example
COPY deploy/systemd/kejilion-agent.service /release/kejilion-agent.service
COPY deploy/systemd/agent.env.example /release/agent.env.example
USER 65532:65532
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD ["/paneld", "healthcheck"]
ENTRYPOINT ["/paneld"]
