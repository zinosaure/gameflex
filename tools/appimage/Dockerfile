# L'ATELIER DE L'APPIMAGE (chantier 70) : ce que le programme du PC n'a pas sur la machine qui le reçoit, figé à des
# versions et des sommes connues — un ffmpeg statique, Electron, appimagetool et le runtime d'une AppImage. Construit
# par tools/appimage/build.sh, qui y assemble l'AppDir (assemble.sh). Le porteur l'a accordé le 2026-11-24 : « Oui, les
# deux » (télécharger appimagetool et son runtime, construire ffmpeg depuis ffmpeg.org).
#
# Debian 12, comme l'image gameflex-retrogaming où se construit le programme : la même glibc.

# ─── ffmpeg, statique, réduit à ce que l'encodeur du PC demande (shared/ffmpegenc) ─────────────────────────────────
# L'image en NV12 ou BGRA par un tuyau → H264 (libx264) ; le son en PCM 16 bits → Opus (libopus) dans de l'Ogg. x264 et
# Opus viennent des paquets Debian (leurs bibliothèques statiques). Ni l'analyseur H264 ni `h264_metadata` : libx264
# sort déjà en Annex B, et ffmpeg 7.1 les compile sans leur dépendance quand tout le reste est coupé (le lien échoue). Sous GPL (x264) : la source exacte et la ligne de
# configuration partent dans l'AppImage (usr/share/doc/ffmpeg/SOURCE.txt).
FROM debian:bookworm-slim AS ffmpeg
ARG FFMPEG_VERSION=7.1.1
ARG FFMPEG_SHA256=733984395e0dbbe5c046abda2dc49a5544e7e0e1e2366bba849222ae9e3a03b1
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl xz-utils build-essential nasm \
        pkg-config libx264-dev libopus-dev && rm -rf /var/lib/apt/lists/*
WORKDIR /build
RUN curl -fsSL -o ffmpeg.tar.xz "https://ffmpeg.org/releases/ffmpeg-${FFMPEG_VERSION}.tar.xz" \
    && echo "${FFMPEG_SHA256}  ffmpeg.tar.xz" | sha256sum -c - \
    && tar xf ffmpeg.tar.xz
ENV FFMPEG_CONFIGURE="--enable-gpl --enable-libx264 --enable-libopus --enable-static --disable-shared \
 --pkg-config-flags=--static --extra-ldflags=-static --extra-libs=-lpthread \
 --disable-doc --disable-ffplay --disable-ffprobe --disable-network --disable-autodetect --disable-everything \
 --enable-protocol=pipe,file --enable-demuxer=rawvideo,pcm_s16le --enable-decoder=rawvideo,pcm_s16le \
 --enable-encoder=libx264,libopus --enable-muxer=h264,opus,ogg --enable-bsf=h264_mp4toannexb \
 --enable-swscale --enable-swresample \
 --enable-filter=scale,format,null,anull,aresample,aformat"
RUN cd ffmpeg-${FFMPEG_VERSION} && ./configure --prefix=/opt/ffmpeg ${FFMPEG_CONFIGURE} \
    && make -j"$(nproc)" ffmpeg && strip ffmpeg && mkdir -p /opt/ffmpeg/bin && cp ffmpeg /opt/ffmpeg/bin/ \
    && printf 'ffmpeg %s, construit pour GameFlex (tools/appimage/Dockerfile)\nSource : https://ffmpeg.org/releases/ffmpeg-%s.tar.xz\nSHA-256 : %s\nLicence : GPL v2 ou ultérieure (avec libx264), voir https://ffmpeg.org/legal.html\nx264 et Opus : paquets Debian 12 (libx264-dev, libopus-dev), sources sur https://sources.debian.org\nConfiguration : %s\n' \
        "${FFMPEG_VERSION}" "${FFMPEG_VERSION}" "${FFMPEG_SHA256}" "${FFMPEG_CONFIGURE}" > /opt/ffmpeg/SOURCE.txt \
    && dpkg-query -W -f='${Package} ${Version} (source : ${source:Package})\n' libx264-dev libopus-dev >> /opt/ffmpeg/SOURCE.txt

# ─── L'atelier ─────────────────────────────────────────────────────────────────────────────────────────────────────
# L'ÉCRAN DES CONSOLES RÉCENTES (chantier 70, P2) : le Xvfb de Debian 12, un par partie (cmd/desktop/guest.go), et les
# bibliothèques propres au serveur X qu'un bureau n'a pas forcément. Le reste (la pile GL, libX11, la compression) est
# celui de l'hôte, comme xkbcomp et les données xkb, que Xvfb appelle par leur chemin (`/usr/bin/xkbcomp`) : tout bureau
# les a (Xwayland en dépend). Licence MIT/X11 ; sources sur https://sources.debian.org/src/xorg-server/.
FROM debian:bookworm-slim AS xvfb
RUN apt-get update && apt-get install -y --no-install-recommends xvfb && rm -rf /var/lib/apt/lists/* \
    && mkdir -p /opt/xvfb/bin /opt/xvfb/lib && cp /usr/bin/Xvfb /opt/xvfb/bin/ \
    && for lib in libpixman-1.so.0 libXfont2.so.2 libfontenc.so.1 libXau.so.6 libXdmcp.so.6 libunwind.so.8 \
            libaudit.so.1 libcap-ng.so.0 libbsd.so.0 libmd.so.0; do \
        cp -L "/lib/x86_64-linux-gnu/$lib" /opt/xvfb/lib/; done \
    && dpkg-query -W -f='${Package} ${Version} (source : ${source:Package})\n' xvfb libpixman-1-0 libxfont2 libfontenc1 \
        libxau6 libxdmcp6 libunwind8 libaudit1 libcap-ng0 libbsd0 libmd0 > /opt/xvfb/SOURCE.txt

# LE SON DES CONSOLES RÉCENTES (chantier 70, P4) : le PulseAudio de Debian 12, privé, un par partie
# (cmd/desktop/guest.go), comme sur le téléphone — un puits nul, la socket des émulateurs, et le moniteur du puits en PCM
# brut sur une socket (module-simple-protocol-unix) que le programme lit. Seuls le démon et ses quatre modules, avec
# leurs bibliothèques hors glibc, à part : le son du bureau n'est pas touché. Licence LGPL v2.1 ; sources sur
# https://sources.debian.org/src/pulseaudio/.
FROM debian:bookworm-slim AS pulse
RUN apt-get update && apt-get install -y --no-install-recommends pulseaudio && rm -rf /var/lib/apt/lists/* \
    && modules=$(dirname "$(find /usr/lib -name module-null-sink.so | head -n 1)") \
    && mkdir -p /opt/pulse/bin /opt/pulse/lib /opt/pulse/modules && cp /usr/bin/pulseaudio /opt/pulse/bin/ \
    && for module in module-null-sink module-native-protocol-unix module-simple-protocol-unix module-always-sink; do \
        cp "$modules/$module.so" /opt/pulse/modules/; done \
    && for file in /opt/pulse/bin/pulseaudio /opt/pulse/modules/*.so; do ldd "$file"; done | awk '{print $3}' | grep '^/' \
        | sort -u | grep -vE '/(libc|libm|libpthread|libdl|librt|libresolv)\.so' | xargs -I{} cp -L {} /opt/pulse/lib/ \
    && dpkg-query -W -f='${Package} ${Version} (source : ${source:Package})\n' pulseaudio libpulse0 > /opt/pulse/SOURCE.txt

FROM debian:bookworm-slim
ARG ELECTRON_VERSION=38.8.6
ARG ELECTRON_SHA256=41a8f2bc7c0be836b4dc8de19550a65947cffa58433753032af36ee3b849e8be
# appimagetool et le runtime n'ont qu'une version « continuous » : leurs sommes du 2026-11-24 les figent — une autre
# version arrête la construction ici, au lieu de passer en silence.
ARG APPIMAGETOOL_SHA256=e8c1169c4af66c3184d7ee110713d4be04befe671fa358e2c1591ee840553b03
ARG RUNTIME_SHA256=617aa77314d57c8b8e4fae9c3f0b35af7b30d5529cb69e98f64f92510ed98a46
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl unzip file desktop-file-utils \
    && rm -rf /var/lib/apt/lists/*
RUN curl -fsSL -o /tmp/electron.zip "https://github.com/electron/electron/releases/download/v${ELECTRON_VERSION}/electron-v${ELECTRON_VERSION}-linux-x64.zip" \
    && echo "${ELECTRON_SHA256}  /tmp/electron.zip" | sha256sum -c - \
    && mkdir -p /opt/electron && unzip -q /tmp/electron.zip -d /opt/electron && rm /tmp/electron.zip
RUN curl -fsSL -o /opt/appimagetool.AppImage https://github.com/AppImage/appimagetool/releases/download/continuous/appimagetool-x86_64.AppImage \
    && echo "${APPIMAGETOOL_SHA256}  /opt/appimagetool.AppImage" | sha256sum -c - && chmod +x /opt/appimagetool.AppImage \
    && cd /opt && ./appimagetool.AppImage --appimage-extract >/dev/null && mv squashfs-root appimagetool && rm appimagetool.AppImage \
    && curl -fsSL -o /opt/runtime-x86_64 https://github.com/AppImage/type2-runtime/releases/download/continuous/runtime-x86_64 \
    && echo "${RUNTIME_SHA256}  /opt/runtime-x86_64" | sha256sum -c -
COPY --from=ffmpeg /opt/ffmpeg /opt/ffmpeg
COPY --from=xvfb /opt/xvfb /opt/xvfb
COPY --from=pulse /opt/pulse /opt/pulse
COPY assemble.sh AppRun gameflex.desktop /opt/atelier/
ENTRYPOINT ["/opt/atelier/assemble.sh"]
