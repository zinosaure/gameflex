# streamex-ports-astrohybrid — PC Ports : Astro Bot par AstroHybrid (chantier 72, À L'ESSAI SUR LA DEV).
#
# Le jeu relié en programme Linux par AnyPS5, rendu par la couche GPU de KytyPS5 (kyty_gpu.so). L'image ne porte ni le
# moteur ni le jeu : le moteur se déballe depuis cores/ports (AstroHybrid-Linux-X64-*.zip, bibliothèques d'AnyPS5,
# kyty_gpu.so, relinker) ; le jeu se construit à la première partie depuis la copie du joueur (run-astrohybrid.sh).
ARG STREAMEX_BIN=streamex-bin
FROM ${STREAMEX_BIN} AS skeleton
FROM debian:trixie-slim
LABEL gameflex.prune=keep
ENV DEBIAN_FRONTEND=noninteractive
ARG ASTROHYBRID_FILE=AstroHybrid-Linux-X64-0.1.2.zip
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
        # AnyPS5 et Kyty : ce que leurs bibliothèques chargent à la demande (SDL, Vulkan), et le déballage du jeu
        python3 libstdc++6 libvulkan1 libx11-6 libxext6 libxrandr2 libxcursor1 libxi6 libxss1 libxtst6 libxfixes3 \
        libxkbcommon0 libwayland-client0 libdecor-0-0 libudev1 libasound2t64 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse
ARG ASTROHYBRID_TESTED="${ASTROHYBRID_FILE}"
ENV GAMEFLEX_EMULATOR_FILE=${ASTROHYBRID_FILE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/ports \
    GAMEFLEX_EMULATOR_PATTERN="AstroHybrid-Linux-X64-*.zip" GAMEFLEX_EMULATOR_NAME="AstroHybrid" \
    GAMEFLEX_EMULATOR_TESTED="${ASTROHYBRID_TESTED}"
LABEL gameflex.emulator.file=${ASTROHYBRID_FILE} \
    gameflex.emulator.pattern="AstroHybrid-Linux-X64-*.zip" \
    gameflex.emulator.tested="${ASTROHYBRID_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator
COPY --from=skeleton /streamex /usr/local/bin/streamex
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY --from=skeleton /sdiag-guid /usr/local/bin/sdiag-guid
COPY src/microservices/runtimes/streamex/consoles/ports-astrohybrid/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/ports-astrohybrid/system.pa /opt/pulse/system.pa
COPY src/microservices/runtimes/streamex/consoles/ports-astrohybrid/run-astrohybrid.sh /opt/run-astrohybrid.sh
COPY src/microservices/runtimes/streamex/consoles/ports-astrohybrid/adapter.json /opt/adapter.json

# LA MANETTE, MAPPÉE POUR SDL (AnyPS5 et Kyty lisent les manettes par SDL) : le même GUID que les autres ports.
ENV SDL_GAMECONTROLLERCONFIG="03000000341200007856000011010000,streamex pad ports,a:b0,b:b1,x:b3,y:b2,back:b6,guide:b8,start:b7,leftstick:b9,rightstick:b10,leftshoulder:b4,rightshoulder:b5,dpup:h0.1,dpdown:h0.4,dpleft:h0.8,dpright:h0.2,leftx:a0,lefty:a1,rightx:a3,righty:a4,lefttrigger:a2,righttrigger:a5,platform:Linux,"

ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator /opt/run-astrohybrid.sh

EXPOSE 8080

HEALTHCHECK --interval=15s --timeout=5s --start-period=45s --retries=3 \
    CMD ["/bin/sh", "-c", "curl -fsS http://127.0.0.1:${STREAMEX_PORT}/status >/dev/null || exit 1"]

# JAMAIS DE RENDU LOGICIEL : llvmpipe est retiré du chargeur Vulkan (le pourquoi est dans consoles/ps5/Dockerfile).
ENV VK_LOADER_DRIVERS_DISABLE="*lvp*"

ENTRYPOINT ["/entrypoint.sh"]
