# syntax=docker/dockerfile:1.27.1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM --platform=$BUILDPLATFORM golang:1.27.3-alpine@sha256:f2c872bcee949b7be9c490efd02b48a8a2fbf947cdf533f1c58525ec9d8170fd AS build
ARG TARGETOS=linux
ARG TARGETARCH
ENV GOMAXPROCS=2
WORKDIR /src
COPY go.mod go.sum ./
COPY third_party/torrent/ ./third_party/torrent/
COPY internal/browserprotocol/ ./internal/browserprotocol/
COPY internal/netpolicy/ ./internal/netpolicy/
COPY internal/webbrowser/ ./internal/webbrowser/
COPY cmd/kpanel-browser/ ./cmd/kpanel-browser/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    --mount=type=secret,id=https_proxy,required=false \
    sh -eu -c 'if [ -f /run/secrets/https_proxy ]; then export HTTPS_PROXY="$(cat /run/secrets/https_proxy)"; fi; \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -p 2 -trimpath -ldflags="-s -w" -o /out/kpanel-browser ./cmd/kpanel-browser \
    && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go test -p 2 -c -o /out/browser-tests ./internal/webbrowser'

FROM alpine:3.24@sha256:be2656061b536c1883c4034790e21d0a09c5aaaf51968b89368a3c0371ea2c9a AS browser-base
ARG ALPINE_MIRROR=https://dl-cdn.alpinelinux.org/alpine
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN --mount=type=secret,id=https_proxy,required=false \
    --mount=type=secret,id=proxy_ca,required=false,mode=0444 \
    --mount=type=cache,target=/var/cache/apk,sharing=locked \
    sh -eu -c 'if [ -f /run/secrets/https_proxy ]; then \
      export http_proxy="$(cat /run/secrets/https_proxy)" https_proxy="$(cat /run/secrets/https_proxy)"; fi; \
    case "$ALPINE_MIRROR" in https://dl-cdn.alpinelinux.org/alpine|https://mirrors.ustc.edu.cn/alpine|https://mirrors.tuna.tsinghua.edu.cn/alpine) ;; *) exit 2;; esac; \
    sed -i "s|https://dl-cdn.alpinelinux.org/alpine|$ALPINE_MIRROR|" /etc/apk/repositories; \
    cp /etc/ssl/certs/ca-certificates.crt /tmp/stock-ca; \
    if [ -f /run/secrets/proxy_ca ]; then cp /run/secrets/proxy_ca /etc/ssl/certs/ca-certificates.crt; fi; \
    apk --cache-dir /var/cache/apk --update-cache add ca-certificates chromium chromium-swiftshader font-noto-cjk font-liberation tini tigervnc openbox xauth xprop xrandr \
    && cp /tmp/stock-ca /etc/ssl/certs/ca-certificates.crt \
    && rm /tmp/stock-ca \
    && apk info -vv | sort > /usr/share/kpanel-browser-packages.txt \
    && addgroup -g 65532 browser \
    && adduser -D -H -u 65532 -G browser -h /tmp/browser browser \
    && if [ ! -x /usr/bin/tini ]; then ln -s /sbin/tini /usr/bin/tini; fi'
ENV HOME=/tmp/browser LANG=C.UTF-8
USER 65532:65532
WORKDIR /tmp

FROM browser-base AS test
COPY --from=build --chmod=0555 /out/browser-tests /usr/local/bin/browser-tests
ENV KPANEL_TEST_CHROME=/usr/bin/chromium KPANEL_TEST_SANDBOX=1 KPANEL_TEST_VNC=1
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/browser-tests"]
CMD ["-test.v", "-test.timeout=180s"]

FROM browser-base AS runtime
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.title="KPanel Browser" \
      org.opencontainers.image.description="Optional sandboxed Chromium service for KPanel" \
      org.opencontainers.image.source="https://github.com/kejilion/KPanel" \
      org.opencontainers.image.url="https://hub.docker.com/r/kjlion/kpanel-browser" \
      org.opencontainers.image.licenses="AGPL-3.0-only" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}"
COPY --from=build --chmod=0555 /out/kpanel-browser /usr/local/bin/kpanel-browser
COPY LICENSE NOTICE THIRD_PARTY_NOTICES.md TRADEMARKS.md /licenses/
COPY LICENSES/ /licenses/third-party/
EXPOSE 8092
HEALTHCHECK --interval=30s --timeout=5s --start-period=40s --retries=3 \
    CMD ["/usr/local/bin/kpanel-browser", "-browser=/usr/bin/chromium", "-token-file=/run/secrets/browser-token", "-healthcheck"]
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/kpanel-browser"]
CMD ["-listen=0.0.0.0:8092", "-browser=/usr/bin/chromium", "-token-file=/run/secrets/browser-token", "-check-runtime"]
