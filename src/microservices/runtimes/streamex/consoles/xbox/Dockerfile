# syntax=docker/dockerfile:1
# ════════════════════════════════════════════════════════════════
#  streamex-xbox: xemu (Xbox) streamed over WebRTC by streamex
#
#  Stage 1: build streamex (generic WebRTC server in Go,
#           pion + cgo openh264/opus/pulse).
#  Stage 2: minimal runtime (weston + xwayland + pulseaudio + xemu).
# ════════════════════════════════════════════════════════════════

# Squelette pré-compilé (streamex/sessreg/sdiag-guid) — sdiag-guid, le
# diagnostic de GUID SDL2 du pad virtuel, y est construit aussi.
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

# xemu release (official AppImage).
ARG XEMU_VERSION=0.8.139
ARG XEMU_APPIMAGE=xemu-${XEMU_VERSION}-x86_64.AppImage
# Épinglée par l'étiquette de la version, jamais par « latest » : le fichier
# xemu-<version> quitte « latest » dès la version suivante, et la console ne
# se construirait plus chez personne (#137, relevé le 2026-09-15).

# MESA VIENT DE trixie-backports (26.1.2), comme pour la PS3.
#
# Le RADV 25.0.7 de trixie a fait planter RPCS3 au passage en combat de Final
# Fantasy XIII (segfault DANS libvulkan_radeon.so, 2026-09-11 — le détail est
# dans consoles/ps3/Dockerfile). Toutes les consoles streamex tournent sur le
# même APU et le même pilote : elles s'alignent, plutôt que d'attendre leur
# propre crash. Simulé image par image avant l'écriture : aucun paquet issu de
# Mesa ne reste en 25.0.7.
#
# MESA D'ABORD, le reste ensuite — Weston et Xwayland tirent Mesa en
# dépendance, et dans l'ordre inverse apt poserait la 25.0.7 pour la remplacer
# aussitôt. libglx-mesa0 fait partie du lot : libgl1/libglx0 le tirent, et
# venu de trixie il réclamerait une autre version exacte de libgallium.
RUN echo "deb http://deb.debian.org/debian trixie-backports main" \
        > /etc/apt/sources.list.d/backports.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t trixie-backports \
        libgl1-mesa-dri mesa-vulkan-drivers libegl-mesa0 libgbm1 libglx-mesa0 mesa-va-drivers \
    && apt-get install -y --no-install-recommends \
        openbox pulseaudio pulseaudio-utils dbus dbus-x11 \
        weston xwayland \
        fontconfig fonts-dejavu-core \
        # System Mesa: glamor/DRI3 for Xwayland + OpenGL for xemu
        x11-xserver-utils xauth xdotool procps \
        # streamex runtime: VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 libxcb-cursor0 \
        # xemu AppImage deps (SDL2, OpenGL loader, resampling, X11 glue)
        libsdl2-2.0-0 libepoxy0 libsamplerate0 libusb-1.0-0 \
        libxkbcommon0 libxcursor1 libxi6 libxinerama1 libxrandr2 libxext6 libxrender1 libxfixes3 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# The "pulse" system user is required by PulseAudio in --system mode
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
# In system mode, module-native-protocol-unix only accepts the pulse-access group
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# ── xemu : l'AppImage officielle, DÉBALLÉE AU LANCEMENT ──
# L'image ne la porte pas : on ne redistribue pas un émulateur (#145), et
# c'est ce qui permet de publier l'image (chantier 29). Elle dit comment
# reconnaître ses versions — un motif de nom —, et laquelle a été testée avec
# elle : celle de consoles/emulators.sh, que l'écran Émulateurs propose de
# télécharger quand le dossier n'en a aucune. Le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe la DERNIÈRE DÉPOSÉE dans /opt/xemu/squashfs-root
# (voir consoles/unpack-emulator.sh).
# AppRun must stay inside squashfs-root or its APPDIR resolution breaks.
ARG XEMU_URL=https://github.com/xemu-project/xemu/releases/download/v0.8.139/xemu-0.8.139-x86_64.AppImage
# Les versions éprouvées avec cette image (emulator_tested) : la liste informe,
# elle ne bloque pas — toute version que reconnaît le motif se lance.
ARG XEMU_TESTED="${XEMU_APPIMAGE}"
ENV GAMEFLEX_EMULATOR_FILE=${XEMU_APPIMAGE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/xbox \
    GAMEFLEX_EMULATOR_PATTERN="xemu-*-x86_64.AppImage" GAMEFLEX_EMULATOR_NAME="xemu" \
    GAMEFLEX_EMULATOR_TESTED="${XEMU_TESTED}"
LABEL gameflex.emulator.file=${XEMU_APPIMAGE} gameflex.emulator.url=${XEMU_URL} \
    gameflex.emulator.pattern="xemu-*-x86_64.AppImage" \
    gameflex.emulator.tested="${XEMU_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY --from=skeleton /sdiag-guid /usr/local/bin/sdiag-guid
COPY src/microservices/runtimes/streamex/consoles/xbox/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/xbox/system.pa /opt/pulse/system.pa
COPY src/microservices/runtimes/streamex/consoles/xbox/xemu.toml /opt/xemu-config/xemu.toml
COPY src/microservices/runtimes/streamex/consoles/xbox/adapter.json /opt/adapter.json

# Contrat data-driven : games/launch/stop/gamepad/tools de CETTE console —
# streamex (le squelette) ne les connaît pas, il lit ce fichier (voir
# internal/adapter/config.go).
ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator

EXPOSE 8080

# Liveness binaire (healthy/unhealthy) : Docker ne connaît que ces deux
# états, pas de "caution" — la nuance "en service mais dégradé" (fallback
# clavier actif faute de pad uinput) vit dans /status.input.degraded, pas
# ici. start-period large : weston + Xwayland + l'émulateur peuvent prendre
# du temps à booter, un HEALTHCHECK qui tire trop tôt marquerait le
# conteneur unhealthy pendant un démarrage parfaitement normal.
HEALTHCHECK --interval=15s --timeout=5s --start-period=45s --retries=3 \
    CMD ["/bin/sh", "-c", "curl -fsS http://127.0.0.1:${STREAMEX_PORT}/status >/dev/null || exit 1"]

# JAMAIS DE RENDU LOGICIEL : llvmpipe est retiré du chargeur Vulkan.
#
# mesa-vulkan-drivers installe aussi llvmpipe (lavapipe), un moteur Vulkan qui
# rend sur le PROCESSEUR. Un émulateur qui le choisit tourne quand même —
# lentement, ou sur un écran noir — sans rien signaler. shadPS4 0.18.0 le
# choisissait à chaque partie (2026-09-11) : il départage deux GPU « non
# dédiés » à la taille du tas mémoire, et llvmpipe annonce toute la RAM de
# l'hôte contre la seule VRAM de l'APU. RPCS3 et Kyty choisissent RADV (mesuré),
# mais rien ne le garantit pour les autres : le chargeur l'écarte pour tous.
# Vérifié : avec cette variable, vulkaninfo ne liste plus que RADV.
ENV VK_LOADER_DRIVERS_DISABLE="*lvp*"

ENTRYPOINT ["/entrypoint.sh"]
