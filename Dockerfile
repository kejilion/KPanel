# syntax=docker/dockerfile:1.24.0@sha256:87999aa3d42bdc6bea60565083ee17e86d1f3339802f543c0d03998580f9cb89

ARG BUILDPLATFORM

FROM --platform=$BUILDPLATFORM node:24.18.0-alpine@sha256:a0b9bf06e4e6193cf7a0f58816cc935ff8c2a908f81e6f1a95432d679c54fbfd AS web-build
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

FROM --platform=$BUILDPLATFORM golang:1.26.6-alpine@sha256:af8d6740070b8906d12eae1c3e3ea0957fb63f492051ea05e354c38ef9fe88df AS go-build
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
      io.kejilion.script.revision="4d7265310cb89d39c09201d99bef213a0e494e3c" \
      io.kejilion.script.sha256="25c5f252029a0ce0e1bdaa46844247a4fe5d7104b1fe642c39af1ae3c4b6b024"
COPY --from=go-build /out/paneld /paneld
COPY --from=go-build /out/kejilion-agent /release/kejilion-agent
COPY --from=go-build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
ADD --checksum=sha256:25c5f252029a0ce0e1bdaa46844247a4fe5d7104b1fe642c39af1ae3c4b6b024 \
    https://raw.githubusercontent.com/kejilion/sh/4d7265310cb89d39c09201d99bef213a0e494e3c/kejilion.sh \
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
