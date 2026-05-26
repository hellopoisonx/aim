FROM debian:trixie-slim

ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        ca-certificates \
        dbus-x11 \
        fonts-noto-cjk \
        libgtk-3-0 \
        libsqlite3-0 \
        libwebkit2gtk-4.1-0 \
        netbase \
        novnc \
        openbox \
        procps \
        python3-websockify \
        tini \
        x11vnc \
        xvfb \
    && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /tmp/.X11-unix \
    && chmod 1777 /tmp/.X11-unix \
    && useradd --create-home --shell /bin/bash aim \
    && mkdir -p /home/aim/.config/aim-desktop \
    && chown -R aim:aim /home/aim/.config

COPY app/desktop/docker/gui/entrypoint.sh /usr/local/bin/aim-desktop-gui-entrypoint
RUN chmod +x /usr/local/bin/aim-desktop-gui-entrypoint

USER aim
WORKDIR /home/aim

ENV DISPLAY=:99 \
    SCREEN_GEOMETRY=1280x800x24 \
    NO_AT_BRIDGE=1 \
    GDK_BACKEND=x11 \
    LIBGL_ALWAYS_SOFTWARE=1 \
    AIM_DESKTOP_BIN=/opt/aim-desktop/aim-desktop \
    AIM_DESKTOP_CONFIG_DIR=/home/aim/.config/aim-desktop \
    AIM_GATEWAY_URL=http://aim-gateway:8888 \
    AIM_WS_URL=ws://aim-gateway:8888/ws \
    VNC_LISTEN=0.0.0.0 \
    VNC_PORT=5900 \
    NOVNC_LISTEN=0.0.0.0 \
    NOVNC_PORT=6080

EXPOSE 5900 6080

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/aim-desktop-gui-entrypoint"]
