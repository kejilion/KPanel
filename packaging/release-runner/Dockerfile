FROM node:26.10.0-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80 AS node-runtime

FROM golang:1.26.7-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468

LABEL org.opencontainers.image.title="KPanel release gate runner" \
      org.opencontainers.image.description="Fixed Linux release gate runner for KPanel" \
      org.opencontainers.image.source="https://github.com/kejilion/KPanel" \
      org.opencontainers.image.base.golang="golang:1.26.7-alpine@sha256:28d89ee9cc0ff9fec75c82ca201e6bf7fdf9a679d4b7b24dfa04f2bb766bb468" \
      org.opencontainers.image.base.node="node:26.10.0-alpine@sha256:0b36e8c136b94cd4fcf02188228e76c31ad5872eef3fec8cbd2eee500cfd9e80"

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
    && node --version | grep -Fx 'v26.10.0' \
    && npm --version | grep -Fx '11.19.1' \
    && npx --version \
    && gcc --version >/dev/null \
    && docker --version >/dev/null \
    && docker buildx version >/dev/null \
    && git --version >/dev/null \
    && make --version >/dev/null

ENTRYPOINT []
CMD ["bash"]
