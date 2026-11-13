# The optional Tor component is built locally; no unpublished image is required.
FROM jlesage/baseimage-gui:ubuntu-24.04-v4@sha256:e7c54da3446b4c65b6755bf00a874463c640a1f579f25bed99532051e81649ef AS download
RUN mkdir -p /config/log && apt-get -o Acquire::Retries=3 -o APT::Update::Error-Mode=any update && apt-get -o Acquire::Retries=3 install -y --no-install-recommends ca-certificates curl gnupg xz-utils \
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

FROM jlesage/baseimage-gui:ubuntu-24.04-v4@sha256:e7c54da3446b4c65b6755bf00a874463c640a1f579f25bed99532051e81649ef
RUN mkdir -p /config/log && apt-get -o Acquire::Retries=3 -o APT::Update::Error-Mode=any update && apt-get -o Acquire::Retries=3 install -y --no-install-recommends libgtk-3-0t64 libdbus-glib-1-2 libx11-xcb1 libxt6t64 libasound2t64 fonts-noto-cjk curl procps \
    && rm -rf /var/lib/apt/lists/*
COPY --from=download /tor-browser /opt/tor-browser
COPY --from=download /tor-data /opt/tor-data
COPY --chmod=0555 tor-start.sh /startapp.sh
# Package post-install scripts replace the baseimage's writable-state symlinks.
# Restore them so initialization works with the read-only root filesystem.
RUN ln -sf /tmp/.passwd /etc/passwd && ln -sf /tmp/.group /etc/group \
    && ln -sf /tmp/.shadow /etc/shadow && ln -sf /tmp/.gshadow /etc/gshadow \
    && rm -rf /run && ln -s /tmp/run /run
ENV APP_NAME="Tor Browser" USER_ID=65532 GROUP_ID=65532 KEEP_APP_RUNNING=1 \
    DISPLAY_WIDTH=1280 DISPLAY_HEIGHT=720 WEB_LISTENING_PORT=8092 \
    VNC_LISTENING_PORT=-1 WEB_AUDIO=0 WEB_FILE_MANAGER=0 WEB_TERMINAL=0 WEB_NOTIFICATION=0
LABEL io.kpanel.browser.kind="tor" io.kpanel.tor.version="16.0.1"
