# The optional Tor component is built locally; no unpublished image is required.
FROM golang:1.27.4-alpine@sha256:0a5f7fe796599ff487605987432e7619ae45beeb2fba4c2ce0b907879932aebf AS helper-toolchain

FROM jlesage/baseimage-gui:ubuntu-24.04-v4@sha256:e7c54da3446b4c65b6755bf00a874463c640a1f579f25bed99532051e81649ef AS download
RUN mkdir -p /config/log && apt-get -o Acquire::Retries=3 -o APT::Update::Error-Mode=any update && apt-get -o Acquire::Retries=3 upgrade -y && apt-get -o Acquire::Retries=3 install -y --no-install-recommends ca-certificates curl gnupg xz-utils \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /download
COPY tor-browser.tar.xz browser.tar.xz
COPY tor-browser.tar.xz.asc browser.tar.xz.asc
COPY tor-signing-key.asc signing-key.asc
RUN echo '5b36ad610c20c81faa72caf27d1cd3a11f14a327c320b4cec581400016ba78b0  browser.tar.xz' | sha256sum -c - \
    && gpg --batch --import signing-key.asc \
    && gpg --batch --export EF6E286DDA85EA2A4BA7DE684E2C6E8793298290 > signing-key.gpg \
    && gpgv --keyring ./signing-key.gpg browser.tar.xz.asc browser.tar.xz \
    && mkdir /tor-browser && tar -xJf browser.tar.xz --strip-components=1 -C /tor-browser \
    && mv /tor-browser/Browser/TorBrowser/Data /tor-data \
    && ln -s /config/tor-data /tor-browser/Browser/TorBrowser/Data \
    && chown -R 0:0 /tor-browser /tor-data && chmod -R a+rX /tor-browser /tor-data

# Rebuild only the two pluggable-transport helpers; the Tor browser archive is
# still verified above before extracting it. Sources and module locks are fixed.
FROM download AS helpers
COPY --from=helper-toolchain /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" GOPATH=/go GOTOOLCHAIN=local CGO_ENABLED=0 GOMAXPROCS=2
WORKDIR /build
COPY tor-helper-locks.tar /build/tor-helper-locks.tar
COPY --chmod=0555 tor-helpers-build.sh /build/tor-helpers-build.sh
RUN mkdir -p /build/locks /build/src/conjure /build/src/lyrebird /out \
    && tar -xf /build/tor-helper-locks.tar -C /build/locks \
    && cd /build/locks && sha256sum -c SHA256SUMS
RUN curl --fail --show-error --silent --location --proto '=https' --max-redirs 3 --max-filesize 33554432 --connect-timeout 15 --max-time 180 --retry 2 \
      'https://gitlab.torproject.org/api/v4/projects/tpo%2Fanti-censorship%2Fpluggable-transports%2Fconjure/repository/archive.tar.gz?sha=14e660f0a64bcbb2b55f01f69c1aafdfd5a6dcfa' -o /build/conjure.tar.gz \
    && echo '145bb11353ca6d1b18fa29fb249e15ba603326835c2672569f972ae4d0f264c0  /build/conjure.tar.gz' | sha256sum -c - \
    && curl --fail --show-error --silent --location --proto '=https' --max-redirs 3 --max-filesize 33554432 --connect-timeout 15 --max-time 180 --retry 2 \
      'https://gitlab.torproject.org/api/v4/projects/tpo%2Fanti-censorship%2Fpluggable-transports%2Flyrebird/repository/archive.tar.gz?sha=59ac7e400c384773f5584dbe366516ad04a33d9f' -o /build/lyrebird.tar.gz \
    && echo 'a8e294e74a0737576f020e273000969ab4bb427ad93846291b6d9c8be33a7e75  /build/lyrebird.tar.gz' | sha256sum -c - \
    && tar -xzf /build/conjure.tar.gz --strip-components=1 -C /build/src/conjure \
    && tar -xzf /build/lyrebird.tar.gz --strip-components=1 -C /build/src/lyrebird \
    && /build/tor-helpers-build.sh

FROM jlesage/baseimage-gui:ubuntu-24.04-v4@sha256:e7c54da3446b4c65b6755bf00a874463c640a1f579f25bed99532051e81649ef
RUN mkdir -p /config/log && apt-get -o Acquire::Retries=3 -o APT::Update::Error-Mode=any update && apt-get -o Acquire::Retries=3 upgrade -y && apt-get -o Acquire::Retries=3 install -y --no-install-recommends libgtk-3-0t64 libdbus-glib-1-2 libx11-xcb1 libxt6t64 libasound2t64 fonts-noto-cjk curl procps \
    && rm -rf /var/lib/apt/lists/*
COPY --from=download /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=download /tor-browser /opt/tor-browser
COPY --from=download /tor-data /opt/tor-data
COPY --from=helpers /out/conjure-client /opt/tor-browser/Browser/TorBrowser/Tor/PluggableTransports/conjure-client
COPY --from=helpers /out/lyrebird /opt/tor-browser/Browser/TorBrowser/Tor/PluggableTransports/lyrebird
COPY --from=helpers /out/provenance /opt/kpanel-tor-helpers
COPY --chmod=0555 tor-start.sh /startapp.sh
# Package post-install scripts replace the baseimage's writable-state symlinks.
# Restore them so initialization works with the read-only root filesystem.
RUN ln -sf /tmp/.passwd /etc/passwd && ln -sf /tmp/.group /etc/group \
    && ln -sf /tmp/.shadow /etc/shadow && ln -sf /tmp/.gshadow /etc/gshadow \
    && rm -rf /run && ln -s /tmp/run /run
ENV APP_NAME="Tor Browser" USER_ID=65532 GROUP_ID=65532 KEEP_APP_RUNNING=1 \
    DISPLAY_WIDTH=1280 DISPLAY_HEIGHT=720 WEB_LISTENING_PORT=8092 \
    VNC_LISTENING_PORT=-1 WEB_AUDIO=0 WEB_FILE_MANAGER=0 WEB_TERMINAL=0 WEB_NOTIFICATION=0
LABEL io.kpanel.browser.kind="tor" io.kpanel.tor.version="16.0.1" io.kpanel.tor.helpers="kpanel.1"
