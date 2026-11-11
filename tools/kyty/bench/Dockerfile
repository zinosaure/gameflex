# Banc d'essai : l'image PS5 du serveur (trixie, Mesa 26 RADV, Weston, Xwayland)
# + de quoi capturer l'écran X. Notre Kyty est monté en /ours.
FROM streamex-ps5:latest
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends imagemagick x11-apps \
 && rm -rf /var/lib/apt/lists/*
# Le RADV du banc est celui de l'image PS5, corrigé (consoles/ps5/mesa/) : la
# base doit le porter, et l'apt ci-dessus ne doit pas avoir remis celui du
# paquet. Sinon le banc mesurerait un autre pilote que les parties, sans le dire.
RUN sha256sum --check --quiet /usr/share/doc/gameflex/radv/libvulkan_radeon.so.sha256 \
 || { echo "le libvulkan_radeon.so de la base n'est pas le RADV corrigé de consoles/ps5/Dockerfile :" \
        "construire le banc sur une image streamex-ps5 qui le porte (tools/kyty/README.md, bench.sh)" >&2; exit 1; }
COPY test.sh /test.sh
RUN chmod +x /test.sh
ENTRYPOINT ["/test.sh"]
