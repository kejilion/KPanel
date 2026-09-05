# syntax=docker/dockerfile:1.24.0@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM node:24.20.0-alpine@sha256:e67514e5d0f6c46656005e1b693b2ec9d52e80b641307de684d4a015ba7a4eaf AS web-build
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN --mount=type=cache,target=/root/.npm \
    --mount=type=secret,id=https_proxy,required=false \
    sh -eu -c 'if [ -f /run/secrets/https_proxy ]; then export HTTPS_PROXY="$(cat /run/secrets/https_proxy)"; fi; npm ci'
COPY web/index.html web/tsconfig.json web/vite.config.ts ./
COPY web/scripts/ ./scripts/
COPY web/src/ ./src/
COPY web/public/ ./public/
RUN npm run build

FROM --platform=$BUILDPLATFORM golang:1.26.7-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468 AS go-build
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
      io.kejilion.script.revision="2d3243621ecf5bb1661d9ae089e9d4de2e837e5a" \
      io.kejilion.script.sha256="dd3c45e17b981836a33f1284bfa44a350f93a3f3a6a81aec64ba40351ce55e34"
COPY --from=go-build /out/paneld /paneld
COPY --from=go-build /out/kejilion-agent /release/kejilion-agent
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD --checksum=sha256:dd3c45e17b981836a33f1284bfa44a350f93a3f3a6a81aec64ba40351ce55e34 \
    https://raw.githubusercontent.com/kejilion/sh/2d3243621ecf5bb1661d9ae089e9d4de2e837e5a/kejilion.sh \
    /release/kejilion.sh
COPY --from=web-build /src/web/dist /app/web
COPY VERSION /release/VERSION
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
