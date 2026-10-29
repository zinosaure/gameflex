# syntax=docker/dockerfile:1
# ════════════════════════════════════════════════════════════════
#  streamex-ps4: shadPS4 (PlayStation 4) streamed over WebRTC by streamex
#
#  Repris de la branche `feature/ps4` du dépôt streamex (commit 4414f90,
#  « reprise plus tard ») et remis au contrat actuel : squelette
#  pré-compilé au lieu d'un build Go par console, adapter.json data-driven
#  au lieu des variables GAMES_DIR/WINDOW_NAME, variables STREAMEX_*.
#
#  Stage 1: squelette pré-compilé (streamex + sdiag-guid).
#  Stage 2: runtime minimal (weston + xwayland + pulseaudio + shadPS4).
# ════════════════════════════════════════════════════════════════

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

# shadPS4 (build SDL officiel, sans Qt) et pkgextract (PKG PS4 → dossier CUSA).
#
# ÉPINGLÉ SUR UNE VERSION PUBLIÉE. La branche d'origine épinglait une
# pré-version du 2026-08-15 dont le tag a disparu depuis : le build échouait au
# téléchargement (curl 22). Les pré-versions ne restent pas.
#
# Une pré-version s'essaie sans toucher ce fichier — le nom du fichier DANS
# l'archive ne change pas : son zip déposé dans cores/ps4/ sous
# `shadps4-linux-sdl-<date>-<sha court>.zip`, puis
#   --build-arg SHADPS4_VERSION=<date>-<sha court>
# Fait le 2026-09-11 pour FF XV Pocket Edition : même arrêt qu'en 0.18.0
# (docs/streamex/TROUBLESHOOTING.md).
ARG SHADPS4_VERSION=0.20.0
ARG PKGEXTRACT_URL=https://github.com/paulomanrique/ps4-pkg-extractor/releases/download/v0.0.1/pkgextract-x86_64-unknown-linux-gnu.zip

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
        # System Mesa: glamor/DRI3 for Xwayland + Vulkan (RADV) for shadPS4
        x11-xserver-utils xauth xdotool procps unzip \
        # streamex runtime: VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 libxcb-cursor0 \
        # shadPS4 deps: SDL3 frontend (dlopen), X11 glue, udev
        libsdl3-0 libxkbcommon0 libxcursor1 libxi6 libxinerama1 libxrandr2 \
        libxext6 libxrender1 libxfixes3 libudev1 libcap2 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# The "pulse" system user is required by PulseAudio in --system mode
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
# In system mode, module-native-protocol-unix only accepts the pulse-access group
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# ── shadPS4 : la construction SDL officielle, DÉBALLÉE AU LANCEMENT ──
# L'image ne la porte pas : on ne redistribue pas un émulateur (#145), et
# c'est ce qui permet de publier l'image (chantier 29). Elle dit comment
# reconnaître ses versions — un motif de nom —, et laquelle a été testée avec
# elle : celle de consoles/emulators.sh, que l'écran Émulateurs propose de
# télécharger quand le dossier n'en a aucune. Le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe la DERNIÈRE DÉPOSÉE dans /opt/shadps4/squashfs-root
# (voir consoles/unpack-emulator.sh).
#
# DEUX ÉTAGES ici, et c'est le seul cas : shadPS4 publie un zip qui CONTIENT
# l'AppImage. C'est donc le zip qu'on garde dans la bibliothèque — le fichier
# tel que l'éditeur le publie — et le déballage fait les deux.
#
# NOTRE shadPS4, s'il est dans la bibliothèque (tools/shadps4/build.sh) : le
# même, plus nos patchs — le réglage `user_vaddr_max_in_gbytes` (chantier 17,
# #367), et la souris en mode pavé tactile qui ne bloque plus le stick gauche
# vertical, le stick droit vertical, rond et triangle (#424). Il remplace le
# binaire de l'AppImage ; AppRun et le reste restent. Sans lui, celui de
# l'éditeur — et le journal de la partie le dit.
ARG SHADPS4_FILE=shadps4-linux-sdl-${SHADPS4_VERSION}.zip
ARG SHADPS4_URL=https://github.com/shadps4-emu/shadPS4/releases/download/v.0.20.0/shadps4-linux-sdl-0.20.0.zip
# Les versions éprouvées avec cette image (emulator_tested) : la liste informe,
# elle ne bloque pas — toute version que reconnaît le motif se lance.
ARG SHADPS4_TESTED="${SHADPS4_FILE}"
ENV GAMEFLEX_EMULATOR_FILE=${SHADPS4_FILE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/ps4 \
    GAMEFLEX_EMULATOR_PATTERN="shadps4-linux-sdl-*.zip" GAMEFLEX_EMULATOR_NAME="shadPS4" \
    GAMEFLEX_EMULATOR_TESTED="${SHADPS4_TESTED}" \
    GAMEFLEX_EMULATOR_BUILD="shadps4-gameflex-*" \
    GAMEFLEX_EMULATOR_BUILD_TARGET=/opt/shadps4/squashfs-root/usr/bin/shadps4
LABEL gameflex.emulator.file=${SHADPS4_FILE} gameflex.emulator.url=${SHADPS4_URL} \
    gameflex.emulator.pattern="shadps4-linux-sdl-*.zip" \
    gameflex.emulator.tested="${SHADPS4_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

# ── pkgextract: PS4 PKG → CUSA game folder (run by the adapter at /start) ──
RUN mkdir -p /tmp/pkgextract && cd /tmp/pkgextract \
    && curl -fL --retry 3 -o pkgextract.zip "$PKGEXTRACT_URL" \
    && unzip -o -q pkgextract.zip && rm pkgextract.zip \
    && chmod +x pkgextract && mv pkgextract /usr/local/bin/pkgextract

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY --from=skeleton /sdiag-guid /usr/local/bin/sdiag-guid
COPY src/microservices/runtimes/streamex/consoles/ps4/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/ps4/system.pa /opt/pulse/system.pa
COPY src/microservices/runtimes/streamex/consoles/ps4/adapter.json /opt/adapter.json

# Le pad se fait passer pour une DualShock 4 (VID/PID Sony). Aucun filtre SDL
# n'est nécessaire pour écarter les pads des autres sessions : ce conteneur ne
# voit que les siens, streamex créant lui-même leurs nœuds (gamepad.createNode).
ENV STREAMEX_PAD_VENDOR=0x054c \
    STREAMEX_PAD_PRODUCT=0x09cc
# Le mapping deviné par SDL3 ne correspond pas à notre pad uinput (boutons de
# façade tournés, select/start morts, sticks et gâchettes mélangés) — on force
# le mapping exact pour ce périphérique (bustype 3, 054c/09cc, version 0x111).
#
# GUID SANS CRC, comme l'image ps5. Il portait le CRC du nom « streamex pad
# ps4 », qui ne vaut que pour le premier pad : les suivants s'appellent
# « …-p2/-p3/-p4 », et SDL leur appliquait l'entrée intégrée de la vraie
# DualShock 4 (a:b1 b:b2, le stick droit sur les gâchettes) — vérifié le
# 2026-10-22 en lisant ce que SDL3 voit des quatre pads. La forme sans CRC les
# couvre tous, et SDL_GAMECONTROLLERCONFIG l'emporte sur l'entrée intégrée du
# même GUID.
#
# Ce mapping suppose que streamex émet les boutons tels quels :
# `swap_xy_from_slot: -1` dans adapter.json. Absent, il valait 0 — X et Y
# inversés à l'émission sur chaque pad, donc carré et triangle échangés en jeu
# (rapporté sur Final Fantasy VII Remake, 2026-10-22).
ENV SDL_GAMECONTROLLERCONFIG="030000004c050000cc09000011010000,PS4 Controller,a:b0,b:b1,x:b3,y:b2,back:b6,start:b7,guide:b8,leftshoulder:b4,rightshoulder:b5,leftstick:b9,rightstick:b10,leftx:a0,lefty:a1,rightx:a3,righty:a4,lefttrigger:a2,righttrigger:a5,dpup:h0.1,dpdown:h0.4,dpleft:h0.8,dpright:h0.2,platform:Linux,"
# Où les jeux extraits atterrissent. Le chemin lui-même est déclaré dans
# l'adapter (games.rules[].unpack.out_dir) ; cette variable ne sert plus qu'à
# l'entrypoint, qui crée le dossier avant que streamex ne démarre.
ENV PS4_EXTRACT_DIR=/opt/games
# Contrat data-driven : games/launch/stop/gamepad/tools de CETTE console —
# streamex (le squelette) ne les connaît pas, il lit ce fichier (voir
# internal/adapter/config.go).
ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator

EXPOSE 8080

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
