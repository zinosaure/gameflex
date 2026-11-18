# ============================================================
#  streamex-ports-zelda64recomp : Zelda 64: Recompiled (le port PC de
#  Majora's Mask) diffusé en WebRTC par streamex — la console « PC Ports »
#  (chantier 40, #609).
#
#  UN PORT, PAS UN ÉMULATEUR. Le jeu est recompilé en code natif ; il ne
#  lit de la ROM que ses données. Elle doit donc être la version exacte que le
#  port connaît : Majora's Mask US 1.0 (run-zelda64.sh le vérifie).
#
#  Même squelette que les autres consoles : Weston headless + Xwayland pour
#  donner à Vulkan (RT64, le moteur de rendu du port) un DRI3 présentable,
#  PulseAudio en puits nul, et le binaire streamex déjà compilé qu'on recopie.
# ============================================================
# Le squelette : streamex-bin en développement, l'image versionnée pour une
# installation publique (--build-arg STREAMEX_BIN=<espace>/streamex-bin:<version>).
ARG STREAMEX_BIN=streamex-bin
FROM ${STREAMEX_BIN} AS skeleton

FROM debian:trixie-slim

# GARDÉE À L'ABRI DE `docker system prune -a` — voir consoles/ps5/Dockerfile.
LABEL gameflex.prune=keep

ENV DEBIAN_FRONTEND=noninteractive

# LA VERSION EST UN ARGUMENT, épinglée sur une publication (#137). L'auteur
# publie un zip qui porte une archive tar.gz : le binaire, ses `assets/` et sa
# base de manettes (consoles/unpack-emulator.sh déballe les deux étages).
ARG ZELDA64_RELEASE=v1.2.2
ARG ZELDA64_FILE=Zelda64Recompiled-${ZELDA64_RELEASE}-Linux-X64.zip
ENV ZELDA64_RELEASE=${ZELDA64_RELEASE}

# MESA VIENT DE trixie-backports, comme pour toutes les consoles streamex
# (le pourquoi est dans consoles/ps5/Dockerfile).
#
# Ce que le binaire ne porte pas (relevé le 2026-10-29 sur la v1.2.2) : SDL 2,
# GTK 3 (le sélecteur de fichier de son menu), freetype et libatomic, liés ;
# libvulkan, ouverte à l'exécution. Sa libstdc++ est statique, et il ne
# demande que la glibc 2.27.
RUN echo "deb http://deb.debian.org/debian trixie-backports main" \
        > /etc/apt/sources.list.d/backports.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t trixie-backports \
        libgl1-mesa-dri mesa-vulkan-drivers libegl-mesa0 libgbm1 libglx-mesa0 mesa-va-drivers \
    && apt-get install -y --no-install-recommends \
        openbox pulseaudio pulseaudio-utils dbus dbus-x11 \
        weston xwayland \
        fontconfig fonts-dejavu-core \
        x11-xserver-utils xauth xdotool procps unzip \
        # runtime streamex : VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 \
        # Zelda 64: Recompiled : ce que son binaire ne porte pas
        libsdl2-2.0-0 libgtk-3-0t64 libfreetype6 libatomic1 libvulkan1 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# L'utilisateur système « pulse » est exigé par PulseAudio en mode --system
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# ── Le port, DÉBALLÉ AU LANCEMENT ──
# L'image ne le porte pas (chantier 29) : le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe le dernier déposé dans /opt/zelda64
# (voir consoles/unpack-emulator.sh). Son dossier est celui de la console,
# cores/ports/, que les autres ports partageront.
ARG ZELDA64_TESTED="${ZELDA64_FILE}"
ENV GAMEFLEX_EMULATOR_FILE=${ZELDA64_FILE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/ports \
    GAMEFLEX_EMULATOR_PATTERN="Zelda64Recompiled-*-Linux-X64.zip" GAMEFLEX_EMULATOR_NAME="Zelda 64: Recompiled" \
    GAMEFLEX_EMULATOR_TESTED="${ZELDA64_TESTED}"
LABEL gameflex.emulator.file=${ZELDA64_FILE} \
    gameflex.emulator.pattern="Zelda64Recompiled-*-Linux-X64.zip" \
    gameflex.emulator.tested="${ZELDA64_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY --from=skeleton /sdiag-guid /usr/local/bin/sdiag-guid
COPY src/microservices/runtimes/streamex/consoles/ports-zelda64recomp/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/ports-zelda64recomp/system.pa /opt/pulse/system.pa
# Le lanceur : la ROM posée là où le port l'attend, puis le port.
COPY src/microservices/runtimes/streamex/consoles/ports-zelda64recomp/run-zelda64.sh /opt/run-zelda64.sh
COPY src/microservices/runtimes/streamex/consoles/ports-zelda64recomp/adapter.json /opt/adapter.json

# LA MANETTE, MAPPÉE POUR SDL : le port lit les manettes par l'API
# GameController de SDL, comme Xenia. Même GUID sans CRC que l'image xbox360 :
# il couvre les quatre pads (« streamex pad ports », puis -p2, -p3, -p4).
ENV SDL_GAMECONTROLLERCONFIG="03000000341200007856000011010000,streamex pad ports,a:b0,b:b1,x:b3,y:b2,back:b6,guide:b8,start:b7,leftstick:b9,rightstick:b10,leftshoulder:b4,rightshoulder:b5,dpup:h0.1,dpdown:h0.4,dpleft:h0.8,dpright:h0.2,leftx:a0,lefty:a1,rightx:a3,righty:a4,lefttrigger:a2,righttrigger:a5,platform:Linux,"

ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator /opt/run-zelda64.sh

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=45s --retries=3 \
    CMD ["/bin/sh", "-c", "curl -fsS http://127.0.0.1:${STREAMEX_PORT}/status >/dev/null || exit 1"]

# JAMAIS DE RENDU LOGICIEL : llvmpipe est retiré du chargeur Vulkan (le
# pourquoi est dans consoles/ps5/Dockerfile).
ENV VK_LOADER_DRIVERS_DISABLE="*lvp*"

ENTRYPOINT ["/entrypoint.sh"]
