# syntax=docker/dockerfile:1.27.1@sha256:4edf897a3ffa55b89f906fc8cc78afdb3f1834cc9c7083565e611a8a7d5fe99e

FROM node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS node-runtime

FROM golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414

LABEL org.opencontainers.image.title="KPanel release gate runner" \
      org.opencontainers.image.description="Fixed Linux release gate runner for KPanel" \
      org.opencontainers.image.source="https://github.com/kejilion/KPanel" \
      org.opencontainers.image.base.golang="golang:1.27.1-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414" \
      org.opencontainers.image.base.node="node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1"

RUN --mount=type=secret,id=https_proxy,required=false \
    sh -eu -c 'if [ -f /run/secrets/https_proxy ]; then \
      export HTTPS_PROXY="$(cat /run/secrets/https_proxy)"; \
      export https_proxy="$HTTPS_PROXY"; \
    fi; apk add --no-cache \
      bash=5.3.9-r1 \
      build-base=0.5-r4 \
      ca-certificates=20260909-r0 \
      coreutils=9.11-r0 \
      docker-cli=29.8.2-r0 \
      docker-cli-buildx=0.34.1-r1 \
      git=2.54.0-r0 \
      make=4.4.1-r4'

COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/include/node /usr/local/include/node
COPY --from=node-runtime /usr/local/lib/node_modules /usr/local/lib/node_modules

RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && go version | grep -F 'go1.27.1' \
    && node --version | grep -Fx 'v24.21.0' \
    && npm --version \
    && npx --version \
    && gcc --version >/dev/null \
    && docker --version >/dev/null \
    && docker buildx version >/dev/null \
    && git --version >/dev/null \
    && make --version >/dev/null

ENTRYPOINT []
CMD ["bash"]
