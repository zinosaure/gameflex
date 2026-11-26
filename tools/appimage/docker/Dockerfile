# GAMEFLEX DANS UN CONTENEUR, POUR CEUX QUI N'ONT PAS DE PC LINUX (chantier 70) — Windows avec Docker Desktop, un NAS, un
# serveur sans bureau. Une Ubuntu 24.04 avec ce qu'un bureau a déjà (le pilote graphique de Mesa, xkbcomp, les polices,
# les bibliothèques dont les émulateurs des éditeurs ont besoin), puis l'AppImage de la Release, telle quelle, lancée sans
# fenêtre. Rien de GameFlex n'est construit ici.
#
#   docker build -t gameflex-appimage tools/appimage/docker
#   docker build -t gameflex-appimage --build-arg APPIMAGE_URL=… tools/appimage/docker    # une autre version
#
# Voir README.md : le compose, le mot de passe de root, le GPU, et ce que Windows ne permet pas.
FROM ubuntu:24.04

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl tini xkb-data x11-xkb-utils \
        mesa-vulkan-drivers libvulkan1 libgl1 libopengl0 libegl1 libgl1-mesa-dri libglx-mesa0 libgbm1 fontconfig fonts-dejavu-core \
        libxkbcommon-x11-0 libxcb-cursor0 libasound2t64 libpulse0 libfreetype6 libx11-6 libxcb1 libxrandr2 libxi6 \
        libxinerama1 libxcursor1 libxext6 libxfixes3 libxrender1 libsm6 libice6 libdbus-1-3 libglib2.0-0t64 libusb-1.0-0 \
        libharfbuzz0b libfribidi0 \
    && rm -rf /var/lib/apt/lists/*

# L'AppImage posée dans local/ (une version construite ici, ou téléchargée à la main), sinon celle de la dernière
# Release, ou celle qu'on nomme. Téléchargée, son SHA-256 se vérifie contre la fiche de la Release
# (gameflex-desktop.json), comme le fait la mise à jour de GameFlex.
COPY local/ /opt/local/
ARG APPIMAGE_URL=https://github.com/zinosaure/gameflex/releases/latest/download/GameFlex-x86_64.AppImage
ARG DESKTOP_JSON_URL=https://github.com/zinosaure/gameflex/releases/latest/download/gameflex-desktop.json
RUN if [ -f /opt/local/GameFlex-x86_64.AppImage ]; then mv /opt/local/GameFlex-x86_64.AppImage /opt/GameFlex.AppImage; \
    else curl -fsSL -o /opt/GameFlex.AppImage "$APPIMAGE_URL" \
    && if curl -fsSL -o /tmp/desktop.json "$DESKTOP_JSON_URL"; then \
         want=$(sed -n 's/.*"sha256" *: *"\([0-9a-f]*\)".*/\1/p' /tmp/desktop.json); \
         [ -z "$want" ] || echo "$want  /opt/GameFlex.AppImage" | sha256sum -c -; \
       fi; fi \
    && chmod +x /opt/GameFlex.AppImage && rm -rf /tmp/desktop.json /opt/local

COPY gameflex-root-password /usr/local/bin/gameflex-root-password

# Une AppImage se monte par FUSE, qu'un conteneur n'a pas : elle se déballe et tourne (APPIMAGE_EXTRACT_AND_RUN).
# La bibliothèque (jeux, BIOS, émulateurs, sauvegardes) dans /library ; les réglages et les émulateurs déballés dans
# /root. Les deux sont des volumes du compose.
ENV APPIMAGE_EXTRACT_AND_RUN=1 GAMEFLEX_LIBRARY=/library GAMEFLEX_PORT=10200
VOLUME ["/library", "/root"]
EXPOSE 10200

ENTRYPOINT ["/usr/bin/tini", "--"]
CMD ["/opt/GameFlex.AppImage", "--no-window"]
