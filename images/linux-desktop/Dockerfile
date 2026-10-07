# Linux research desktop: XFCE over TigerVNC. Users reach it only through the KubeTRE
# access gateway (Guacamole), which enforces sign-in and disables clipboard and transfer.
# Runs as an unprivileged user and satisfies the "restricted" Pod Security Standard.
FROM debian:bookworm-slim

RUN apt-get update \
 && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
      xfce4 xfce4-terminal dbus-x11 x11-xserver-utils \
      tigervnc-standalone-server tigervnc-tools \
      firefox-esr python3 python3-pip python3-venv git nano less procps \
      ca-certificates fonts-dejavu-core \
 && rm -rf /var/lib/apt/lists/* \
 && useradd --uid 1000 --create-home --shell /bin/bash researcher

COPY start-desktop.sh /usr/local/bin/start-desktop
RUN chmod 0755 /usr/local/bin/start-desktop

USER 1000:1000
ENV HOME=/home/researcher \
    DISPLAY=:1 \
    XDG_RUNTIME_DIR=/tmp/runtime \
    RESOLUTION=1600x900
WORKDIR /home/researcher
EXPOSE 5901
ENTRYPOINT ["/usr/local/bin/start-desktop"]
