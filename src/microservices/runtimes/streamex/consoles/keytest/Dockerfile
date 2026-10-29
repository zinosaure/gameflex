# syntax=docker/dockerfile:1
# ════════════════════════════════════════════════════════════════
#  streamex-keytest: input-latency test console streamed over WebRTC.
#
#  Same pipeline as the other consoles, except the "emulator" slot is a
#  fullscreen tkinter display (keytest/display.py): every input event
#  received by streamex is rendered there with its press/receive
#  timestamps, so the streamed video shows the real key travel time.
# ════════════════════════════════════════════════════════════════

# Squelette pré-compilé (streamex/sessreg/sdiag-guid).
# Le squelette : streamex-bin en développement, l'image versionnée pour une
# installation publique (--build-arg STREAMEX_BIN=<espace>/streamex-bin:<version>).
ARG STREAMEX_BIN=streamex-bin
FROM ${STREAMEX_BIN} AS skeleton

# ────────────────────────── Runtime ──────────────────────────
FROM debian:trixie-slim

# GARDÉE À L'ABRI DE `docker system prune -a`.
#
# Une image de session n'a pas de conteneur permanent : entre deux parties,
# rien ne la référence, et un élagage la considère donc comme inutilisée. Elle
# part, et la partie suivante répond « l'image de cette console est en cours de
# reconstruction » — le kernel la rebâtit tout seul, mais il y en a pour
# plusieurs minutes, à chaque fois (vécu le 2026-09-07, et déjà le 2026-08-17).
#
# Cette étiquette est ce qu'un élagage peut filtrer :
#
#     docker system prune -a --filter "label!=gameflex.prune=keep"
#
# Voir `tools/prune.sh`, qui l'écrit pour ne pas avoir à s'en souvenir.
LABEL gameflex.prune=keep


ENV DEBIAN_FRONTEND=noninteractive

RUN apt-get update && apt-get install -y --no-install-recommends \
        openbox pulseaudio pulseaudio-utils dbus dbus-x11 \
        weston xwayland \
        fontconfig fonts-dejavu-core \
        python3 python3-tk \
        imagemagick \
        # System Mesa: glamor/DRI3 for Xwayland
        libgl1-mesa-dri libegl-mesa0 libgbm1 \
        x11-xserver-utils xauth xdotool procps \
        # streamex runtime: VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 libxcb-cursor0 libc++1 libc++abi1 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# The "pulse" system user is required by PulseAudio in --system mode
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
# In system mode, module-native-protocol-unix only accepts the pulse-access group
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY src/microservices/runtimes/streamex/consoles/keytest/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/keytest/display.py /opt/keytest/display.py
COPY src/microservices/runtimes/streamex/consoles/keytest/system.pa /opt/pulse/system.pa

# GAME_PATH is a placeholder: the KeyTest adapter only uses it as the boot
# trigger for the display app (see adapter/keytest.go).
ENV STREAMEX_CONSOLE=keytest \
    STREAMEX_GAME_PATH=/opt/keytest/game \
    STREAMEX_WINDOW_NAME=keytest

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /opt/keytest/display.py

EXPOSE 8080

# Liveness binaire (healthy/unhealthy) : Docker ne connaît que ces deux
# états, pas de "caution" — la nuance "en service mais dégradé" (fallback
# clavier actif faute de pad uinput) vit dans /status.input.degraded, pas
# ici. start-period large : weston + Xwayland + l'émulateur peuvent prendre
# du temps à booter, un HEALTHCHECK qui tire trop tôt marquerait le
# conteneur unhealthy pendant un démarrage parfaitement normal.
# LE PORT VIENT DE L'ENVIRONNEMENT, comme pour les cinq autres consoles.
#
# `8080` était écrit en dur ici seul. Les conteneurs de session tournent en
# `network_mode: host` : ce 127.0.0.1:8080 n'est donc PAS celui de streamex,
# c'est celui de la MACHINE — et si un autre service y écoute, la sonde
# l'interroge lui. C'est ce qui arrivait (2026-09-09) : des milliers de 404
# dans le journal d'un service voisin, et un healthcheck qui ne mesurait plus
# rien de streamex.
HEALTHCHECK --interval=15s --timeout=5s --start-period=45s --retries=3 \
    CMD ["/bin/sh", "-c", "curl -fsS http://127.0.0.1:${STREAMEX_PORT}/status >/dev/null || exit 1"]

ENTRYPOINT ["/entrypoint.sh"]
