FROM node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1 AS node-runtime

FROM golang:1.26.7-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468

LABEL org.opencontainers.image.title="KPanel release gate runner" \
      org.opencontainers.image.description="Fixed Linux release gate runner for KPanel" \
      org.opencontainers.image.source="https://github.com/kejilion/KPanel" \
      org.opencontainers.image.base.golang="golang:1.26.7-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468" \
      org.opencontainers.image.base.node="node:24.21.0-alpine@sha256:ebfe2f90462722a7a4de65e91990e97fe0d401c70e0e762c5b53302f905ec1c1"

RUN apk add --no-cache \
      bash=5.3.9-r1 \
      build-base=0.5-r4 \
      ca-certificates=20260909-r0 \
      coreutils=9.11-r0 \
      docker-cli=29.8.2-r0 \
      docker-cli-buildx=0.34.1-r1 \
      git=2.54.0-r0 \
      make=4.4.1-r4

COPY --from=node-runtime /usr/local/bin/node /usr/local/bin/node
COPY --from=node-runtime /usr/local/include/node /usr/local/include/node
COPY --from=node-runtime /usr/local/lib/node_modules /usr/local/lib/node_modules

RUN ln -s ../lib/node_modules/npm/bin/npm-cli.js /usr/local/bin/npm \
    && ln -s ../lib/node_modules/npm/bin/npx-cli.js /usr/local/bin/npx \
    && go version | grep -F 'go1.26.7' \
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
