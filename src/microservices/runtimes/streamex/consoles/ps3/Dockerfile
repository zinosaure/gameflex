# syntax=docker/dockerfile:1
# ════════════════════════════════════════════════════════════════
#  streamex-ps3: RPCS3 (PS3) streamed over WebRTC by streamex
#
#  Runtime SEUL (weston + xwayland + pulseaudio + rpcs3) : les binaires du
#  squelette viennent de l'image streamex-bin, compilée une seule fois pour
#  toutes les consoles (voir streamex/Dockerfile).
# ════════════════════════════════════════════════════════════════

# Squelette pré-compilé (streamex/sessreg/sdiag-guid).
# Le squelette : streamex-bin en développement, l'image versionnée pour une
# installation publique (--build-arg STREAMEX_BIN=<espace>/streamex-bin:<version>).
ARG STREAMEX_BIN=streamex-bin
FROM ${STREAMEX_BIN} AS skeleton

# ────────────────────────── Runtime ──────────────────────────
# Trixie (Debian 13): recent Mesa, needed for Xwayland glamor/DRI3
# and Vulkan (RDNA3) for RPCS3's GPU rendering.
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

# RPCS3 version/variant (official AppImage, rpcs3-binaries-linux).
ARG RPCS3_APPIMAGE=rpcs3-v0.0.43-20404-e8979d31_linux64.AppImage

# MESA VIENT DE trixie-backports (26.1.2), PAS DE trixie (25.0.7).
#
# Final Fantasy XIII s'est figé au passage en combat le 2026-09-11 : le thread
# RSX de RPCS3 a lu un pointeur nul (« Segfault reading location
# 0000000000000000 »), et l'adresse fautive, rapportée à la carte mémoire du
# processus encore vivant, tombe DANS libvulkan_radeon.so — le pilote RADV,
# pas le code de RPCS3. Aucune erreur Vulkan juste avant, aucun reset GPU
# côté noyau : un bug logiciel du pilote, sur un APU HawkPoint (RADV PHOENIX)
# que Mesa 25.0 connaît mal. Le 25.0.7 de trixie a un an de retard ; la
# backport 26.1.2 s'installe sans rien retirer (simulé avant d'écrire ceci).
#
# MESA D'ABORD, le reste ensuite : Weston et Xwayland tirent Mesa en
# dépendance, et dans l'ordre inverse apt poserait la 25.0.7 pour la
# remplacer aussitôt.
#
# Les cinq paquets vont ensemble — ils viennent de la même source Mesa et se
# dépendent à version exacte. libopengl0 n'en fait pas partie (libglvnd).
RUN echo "deb http://deb.debian.org/debian trixie-backports main" \
        > /etc/apt/sources.list.d/backports.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t trixie-backports \
        libgl1-mesa-dri mesa-vulkan-drivers libegl-mesa0 libgbm1 libglx-mesa0 mesa-va-drivers \
    && apt-get install -y --no-install-recommends \
        openbox pulseaudio pulseaudio-utils dbus dbus-x11 \
        weston xwayland \
        fontconfig fonts-dejavu-core \
        # System Mesa (installed above): glamor/DRI3 for Xwayland + Vulkan/OpenGL
        # for RPCS3 (the RPCS3 AppImage links against the system libOpenGL.so.0)
        libopengl0 \
        x11-xserver-utils xauth xdotool procps imagemagick wmctrl \
        # streamex runtime: VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        # + ALSA (RPCS3 audio path) + Qt system deps for the AppImage
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 libxcb-cursor0 libc++1 libc++abi1 \
        libasound2 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# The "pulse" system user is required by PulseAudio in --system mode
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
# In system mode, module-native-protocol-unix only accepts the pulse-access group
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# ── RPCS3 : l'AppImage officielle, DÉBALLÉE AU LANCEMENT ──
# L'image ne la porte pas : on ne redistribue pas un émulateur (#145), et
# c'est ce qui permet de publier l'image (chantier 29). Elle dit comment
# reconnaître ses versions — un motif de nom —, et laquelle a été testée avec
# elle : celle de consoles/emulators.sh, que l'écran Émulateurs propose de
# télécharger quand le dossier n'en a aucune. Le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe la DERNIÈRE DÉPOSÉE dans /opt/rpcs3/squashfs-root
# (voir consoles/unpack-emulator.sh).
# NB: AppRun must be invoked as squashfs-root/AppRun (a symlink outside would
# break its APPDIR resolution and the apprun-hooks lookup).
ARG RPCS3_URL=https://github.com/RPCS3/rpcs3-binaries-linux/releases/download/build-e8979d31b3a22c8bc9a6eae6bdc31756eac8ee89/rpcs3-v0.0.43-20404-e8979d31_linux64.AppImage
# Les versions éprouvées avec cette image (emulator_tested) : la liste informe,
# elle ne bloque pas — toute version que reconnaît le motif se lance.
ARG RPCS3_TESTED="${RPCS3_APPIMAGE}"
ENV GAMEFLEX_EMULATOR_FILE=${RPCS3_APPIMAGE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/ps3 \
    GAMEFLEX_EMULATOR_PATTERN="rpcs3-v*_linux64.AppImage" GAMEFLEX_EMULATOR_NAME="RPCS3" \
    GAMEFLEX_EMULATOR_TESTED="${RPCS3_TESTED}"
LABEL gameflex.emulator.file=${RPCS3_APPIMAGE} gameflex.emulator.url=${RPCS3_URL} \
    gameflex.emulator.pattern="rpcs3-v*_linux64.AppImage" \
    gameflex.emulator.tested="${RPCS3_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY src/microservices/runtimes/streamex/consoles/ps3/rpcs3/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/ps3/rpcs3/config.yml /opt/rpcs3-config/config.yml
COPY src/microservices/runtimes/streamex/consoles/ps3/rpcs3/input_configs/global/Default.yml /opt/rpcs3-config/input_configs/global/Default.yml
COPY src/microservices/runtimes/streamex/consoles/ps3/rpcs3/system.pa /opt/pulse/system.pa
COPY src/microservices/runtimes/streamex/consoles/ps3/rpcs3/adapter.json /opt/adapter.json

# Unique pad identity per console (DualShock 3 ids): the virtual pads of all
# containers share the host uinput, SDL-based emulators elsewhere must be
# able to tell them apart.
ENV STREAMEX_PAD_VENDOR=0x054c STREAMEX_PAD_PRODUCT=0x0268
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

# LE CACHE DE SHADERS DU PILOTE, DANS LE DOSSIER QUI SURVIT À LA SESSION.
#
# Depuis la build 20078 environ, RPCS3 précompile au lancement les 6650
# « variantes » de son interpréteur de shaders (« Precompiling shader
# interpreter variants »), et il a retiré son propre cache de pipelines de ce
# code (« remove pipeline cache from interpreter code ») : il compte sur celui
# du pilote. Or celui de Mesa s'écrit par défaut dans ~/.cache/mesa_shader_cache…,
# hors du seul dossier que le kernel persiste (/root/.cache/rpcs3, docker.go).
# Mesuré le 2026-10-08 avec la 20247 : cinq minutes de précompilation à CHAQUE
# partie, puis le jeu. Placé dans le dossier persisté, RADV retrouve ses
# pipelines d'une session à l'autre — un compte paie la compilation une fois
# par version de pilote.
ENV MESA_SHADER_CACHE_DIR=/root/.cache/rpcs3/mesa_shader_cache

ENTRYPOINT ["/entrypoint.sh"]
