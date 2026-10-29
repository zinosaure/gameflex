# ============================================================
#  streamex-xbox360 : Xenia Canary (émulateur Xbox 360) diffusé en WebRTC
#  par streamex (#414).
#
#  UN ÉMULATEUR EXPÉRIMENTAL SOUS LINUX, ET C'EST DIT ICI.
#
#  Le guide de construction de Xenia l'écrit lui-même : « Linux support is
#  extremely experimental and presently incomplete » (relevé le 2026-10-22,
#  publication e6cd2e4 du 2026-10-20). Ce qui en découle ici :
#    - Vulkan seulement, et X11 seulement : la surface Wayland de Xenia est
#      encore à faire. D'où Xwayland, comme les autres consoles, et
#      GDK_BACKEND=x11 dans l'entrypoint ;
#    - l'AVX est exigé au démarrage (le Ryzen de l'hôte l'a, AVX-512 compris) ;
#    - glibc 2.38 au moins (mesuré sur le binaire) : trixie, pas bookworm.
#
#  Même squelette que les autres images : Weston headless + Xwayland pour
#  donner à Vulkan un DRI3 présentable, PulseAudio en puits nul, et le binaire
#  streamex déjà compilé qu'on recopie.
# ============================================================
# Le squelette : streamex-bin en développement, l'image versionnée pour une
# installation publique (--build-arg STREAMEX_BIN=<espace>/streamex-bin:<version>).
ARG STREAMEX_BIN=streamex-bin
FROM ${STREAMEX_BIN} AS skeleton

FROM debian:trixie-slim

# GARDÉE À L'ABRI DE `docker system prune -a` — voir consoles/ps5/Dockerfile :
# une image de session n'a pas de conteneur permanent, un élagage la prendrait
# pour inutilisée.
LABEL gameflex.prune=keep

ENV DEBIAN_FRONTEND=noninteractive

# LA VERSION EST UN ARGUMENT, épinglée sur une publication et jamais sur
# « latest » (#137). Xenia Canary publie depuis son dépôt principal, une
# AppImage par commit ; l'ancien dépôt `xenia-canary-releases` ne bouge plus
# depuis le 2026-03-24, et ses adresses répondent 404 depuis le 2026-06-24.
# Le nom de l'AppImage de l'éditeur ne porte pas la version : le fichier
# rangé dans la bibliothèque, lui, la porte (consoles/emulators.sh).
ARG XENIA_RELEASE=291f0c9
ARG XENIA_FILE=xenia_canary_linux-${XENIA_RELEASE}.AppImage
# La version, pour que le journal d'une partie dise contre quoi on jouait.
ENV XENIA_RELEASE=${XENIA_RELEASE}

# MESA VIENT DE trixie-backports, comme pour toutes les consoles streamex :
# même APU, même pilote (le pourquoi est dans consoles/ps5/Dockerfile). Mesa
# d'abord, le reste ensuite.
#
# Ce que l'AppImage n'embarque pas, et que le conteneur doit donc fournir
# (relevé sur ses bibliothèques le 2026-10-22) : la libc et libstdc++ (GCC 13
# au moins), X11 et XCB, ALSA, DRM/GBM, le client Wayland que GTK lie,
# fontconfig/freetype/fribidi/harfbuzz, et deux bibliothèques ouvertes à
# l'exécution — libvulkan (Xenia la charge par dlopen) et libudev (SDL).
# GTK 3 et SDL 2.30, eux, sont dans l'AppImage.
RUN echo "deb http://deb.debian.org/debian trixie-backports main" \
        > /etc/apt/sources.list.d/backports.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t trixie-backports \
        libgl1-mesa-dri mesa-vulkan-drivers libegl-mesa0 libgbm1 libglx-mesa0 mesa-va-drivers \
    && apt-get install -y --no-install-recommends \
        openbox pulseaudio pulseaudio-utils dbus dbus-x11 \
        weston xwayland \
        fontconfig fonts-dejavu-core \
        x11-xserver-utils xauth xdotool procps \
        # runtime streamex : VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 \
        # Xenia : ce que son AppImage n'embarque pas
        libstdc++6 libx11-6 libx11-xcb1 libxcb1 libasound2t64 libdrm2 \
        libwayland-client0 libfontconfig1 libfreetype6 libfribidi0 libharfbuzz0b \
        libgpg-error0 zlib1g libvulkan1 libudev1 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# L'utilisateur système « pulse » est exigé par PulseAudio en mode --system
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# ── Xenia Canary : l'AppImage officielle, DÉBALLÉE AU LANCEMENT ──
# L'image ne la porte pas : on ne redistribue pas un émulateur (#145), et
# c'est ce qui permet de publier l'image (chantier 29). Elle dit comment
# reconnaître ses versions — un motif de nom —, et laquelle a été testée avec
# elle : celle de consoles/emulators.sh, que l'écran Émulateurs propose de
# télécharger quand le dossier n'en a aucune. Le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe la DERNIÈRE DÉPOSÉE dans /opt/xenia/squashfs-root
# (voir consoles/unpack-emulator.sh).
# Le binaire se lance ensuite directement (`usr/bin/xenia_canary`, son
# RUNPATH trouve les bibliothèques de l'AppImage dans `usr/lib`).
#
# Xenia publie son AppImage sous un nom SANS version : celui rangé dans la
# bibliothèque la porte (emulators.sh), et le téléchargement de l'écran
# Émulateurs l'enregistre sous ce nom-là.
ARG XENIA_URL=https://github.com/xenia-canary/xenia-canary/releases/download/291f0c9/xenia_canary_linux.AppImage
# Les versions éprouvées avec cette image (emulator_tested) : la liste informe,
# elle ne bloque pas — toute version que reconnaît le motif se lance.
ARG XENIA_TESTED="${XENIA_FILE}"
ENV GAMEFLEX_EMULATOR_FILE=${XENIA_FILE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/xbox360 \
    GAMEFLEX_EMULATOR_PATTERN="xenia_canary_linux-*.AppImage" GAMEFLEX_EMULATOR_NAME="Xenia Canary" \
    GAMEFLEX_EMULATOR_TESTED="${XENIA_TESTED}"
LABEL gameflex.emulator.file=${XENIA_FILE} gameflex.emulator.url=${XENIA_URL} \
    gameflex.emulator.pattern="xenia_canary_linux-*.AppImage" \
    gameflex.emulator.tested="${XENIA_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY --from=skeleton /sdiag-guid /usr/local/bin/sdiag-guid
COPY src/microservices/runtimes/streamex/consoles/xbox360/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/xbox360/system.pa /opt/pulse/system.pa
# Le lanceur : Xenia relancé une fois s'il meurt à son démarrage (voir le script).
COPY src/microservices/runtimes/streamex/consoles/xbox360/run-xenia.sh /opt/xenia/run-xenia.sh
# L'adaptateur : un jeu Xbox 360 est un dossier qui porte son ISO (le dump
# d'un disque, que Xenia lit directement), ou un `.xex` posé seul. Les
# réglages fixes de Xenia sur ce serveur passent sur sa ligne de commande —
# Vulkan, plein écran, le son et les manettes par SDL, pas de Discord — et
# gagnent ainsi sur son fichier ; les réglables, eux, passent par les
# paramètres de l'émulateur (gameflex/emuparams, `xenia`).
COPY src/microservices/runtimes/streamex/consoles/xbox360/adapter.json /opt/adapter.json

# LA MANETTE, MAPPÉE POUR SDL : Xenia ne lit les manettes, sous Linux, que par
# l'API GameController de SDL. Notre pad uinput n'a pas l'ordre de boutons que
# SDL devinerait (il croiserait X et Y). GUID SANS CRC, comme l'image ps5 : il
# couvre les QUATRE pads d'un coup (« streamex pad xbox360 », puis -p2, -p3,
# -p4), là où un CRC ne vaut que pour le nom du premier. D'où aussi
# `swap_xy_from_slot: -1` dans l'adaptateur : aucun pad n'est à recroiser.
ENV SDL_GAMECONTROLLERCONFIG="03000000341200007856000011010000,streamex pad xbox360,a:b0,b:b1,x:b3,y:b2,back:b6,guide:b8,start:b7,leftstick:b9,rightstick:b10,leftshoulder:b4,rightshoulder:b5,dpup:h0.1,dpdown:h0.4,dpleft:h0.8,dpright:h0.2,leftx:a0,lefty:a1,rightx:a3,righty:a4,lefttrigger:a2,righttrigger:a5,platform:Linux,"

ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator /opt/xenia/run-xenia.sh

EXPOSE 8080

# Liveness binaire (healthy/unhealthy), comme les autres consoles : un
# start-period large, Weston + Xwayland + Xenia peuvent prendre du temps à
# booter.
HEALTHCHECK --interval=15s --timeout=5s --start-period=45s --retries=3 \
    CMD ["/bin/sh", "-c", "curl -fsS http://127.0.0.1:${STREAMEX_PORT}/status >/dev/null || exit 1"]

# JAMAIS DE RENDU LOGICIEL : llvmpipe est retiré du chargeur Vulkan (le
# pourquoi est dans consoles/ps5/Dockerfile).
ENV VK_LOADER_DRIVERS_DISABLE="*lvp*"

ENTRYPOINT ["/entrypoint.sh"]
