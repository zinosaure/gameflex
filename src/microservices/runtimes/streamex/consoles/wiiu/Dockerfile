# syntax=docker/dockerfile:1
# ════════════════════════════════════════════════════════════════
#  streamex-wiiu: Cemu (Wii U) streamed over WebRTC by streamex
#
#  Stage 1: build streamex (generic WebRTC server in Go,
#           pion + cgo openh264/opus/pulse).
#  Stage 2: minimal runtime (weston + xwayland + pulseaudio + Cemu).
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
# plusieurs minutes, à chaque fois.
#
# Cette étiquette est ce qu'un élagage peut filtrer :
#
#     docker system prune -a --filter "label!=gameflex.prune=keep"
#
# Voir `tools/prune.sh`, qui l'écrit pour ne pas avoir à s'en souvenir.
LABEL gameflex.prune=keep

ENV DEBIAN_FRONTEND=noninteractive

# Cemu (AppImage officiel, la seule livraison Linux du projet).
#
# Épinglé sur une version PUBLIÉE plutôt que sur `latest/download` : le nom du
# fichier porte le numéro de version, donc `latest` ne suffirait pas à le
# construire, et un tag qui disparaît casse le build au téléchargement (déjà
# vécu côté shadPS4, curl 22).
ARG CEMU_VERSION=2.6

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
        # Mesa système : glamor/DRI3 pour Xwayland + Vulkan (RADV) pour Cemu,
        # dont le moteur de rendu par défaut ici EST Vulkan (voir settings.xml).
        libvulkan1 \
        x11-xserver-utils xauth xdotool procps \
        # streamex runtime: VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 libxcb-cursor0 \
        # Cemu : interface wxWidgets/GTK3, SDL2 pour les manettes, X11 autour
        libgtk-3-0 libsdl2-2.0-0 libxkbcommon0 libxcursor1 libxi6 \
        libxinerama1 libxrandr2 libxext6 libxrender1 libxfixes3 \
        libudev1 libcurl4 libssl3 libzip5 \
        # libOpenGL.so.0 (paquet libopengl0) : l'AppImage de Cemu s'y lie
        # DIRECTEMENT, et ne l'embarque pas. Sans lui le binaire ne démarre
        # même pas — « error while loading shared libraries: libOpenGL.so.0 »,
        # et streamex n'annonce qu'un « launched » qui ne veut plus rien dire
        # (2026-09-08). libglx0/libglvnd viennent avec.
        libopengl0 libglx0 libgl1 libglu1-mesa libbluetooth3 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# The "pulse" system user is required by PulseAudio in --system mode
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
# In system mode, module-native-protocol-unix only accepts the pulse-access group
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# ── Cemu : l'AppImage officielle, DÉBALLÉE AU LANCEMENT ──
# L'image ne la porte pas : on ne redistribue pas un émulateur (#145), et
# c'est ce qui permet de publier l'image (chantier 29). Elle dit comment
# reconnaître ses versions — un motif de nom —, et laquelle a été testée avec
# elle : celle de consoles/emulators.sh, que l'écran Émulateurs propose de
# télécharger quand le dossier n'en a aucune. Le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe la DERNIÈRE DÉPOSÉE dans /opt/cemu/squashfs-root
# (voir consoles/unpack-emulator.sh).
# AppRun doit rester dans squashfs-root, sinon sa résolution d'APPDIR casse.
ARG CEMU_FILE=Cemu-${CEMU_VERSION}-x86_64.AppImage
ARG CEMU_URL=https://github.com/cemu-project/Cemu/releases/download/v2.6/Cemu-2.6-x86_64.AppImage
# Les versions éprouvées avec cette image (emulator_tested) : la liste informe,
# elle ne bloque pas — toute version que reconnaît le motif se lance.
ARG CEMU_TESTED="${CEMU_FILE}"
ENV GAMEFLEX_EMULATOR_FILE=${CEMU_FILE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/wiiu \
    GAMEFLEX_EMULATOR_PATTERN="Cemu-*-x86_64.AppImage" GAMEFLEX_EMULATOR_NAME="Cemu" \
    GAMEFLEX_EMULATOR_TESTED="${CEMU_TESTED}"
LABEL gameflex.emulator.file=${CEMU_FILE} gameflex.emulator.url=${CEMU_URL} \
    gameflex.emulator.pattern="Cemu-*-x86_64.AppImage" \
    gameflex.emulator.tested="${CEMU_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

# AUCUN CORRECTIF DE JEU DANS CETTE IMAGE.
#
# Les packs graphiques de Cemu corrigent UN jeu chacun (celui de New Super
# Mario Bros. U répare son plantage d'écran titre, voir
# docs/TROUBLESHOOTING.md). Les figer ici ferait porter à tout le monde les
# octets d'un seul titre, et l'archive communautaire complète est pire : sur
# ses 398 packs, 89 s'activent tout seuls — triches, retraits de HUD,
# changements de résolution.
#
# Ils vivent donc dans le stockage froid, à côté des BIOS et des ROMs :
# `patches/<console>/`, déposés par l'administrateur, recopiés au lancement
# par l'entrypoint. Même règle que les clés de déchiffrement.

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY src/microservices/runtimes/streamex/consoles/wiiu/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/wiiu/system.pa /opt/pulse/system.pa
COPY src/microservices/runtimes/streamex/consoles/wiiu/settings.xml /opt/cemu-config/settings.xml
# Profils du GamePad (joueur 1) et de la télécommande (joueur 2, #441) : Cemu lit
# les manettes par DSU, comme Citron. streamex les recopie avant chaque lancement
# avec le port DSU de la session (template_file).
COPY src/microservices/runtimes/streamex/consoles/wiiu/controller0.xml /opt/cemu-config/controllerProfiles/controller0.xml
COPY src/microservices/runtimes/streamex/consoles/wiiu/controller1.xml /opt/cemu-config/controllerProfiles/controller1.xml
COPY src/microservices/runtimes/streamex/consoles/wiiu/adapter.json /opt/adapter.json

# Contrat data-driven : games/launch/stop/gamepad/tools de CETTE console —
# streamex (le squelette) ne les connaît pas, il lit ce fichier (voir
# internal/adapter/config.go).
ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator

EXPOSE 8080

# Liveness binaire (healthy/unhealthy) : Docker ne connaît que ces deux
# états. start-period large : weston + Xwayland + Cemu peuvent prendre du
# temps à booter, un HEALTHCHECK qui tire trop tôt marquerait le conteneur
# unhealthy pendant un démarrage parfaitement normal.
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
