# Banc d'essai : l'image PS4 du serveur (trixie, Mesa 26 RADV, Weston, Xwayland,
# pkgextract) + de quoi capturer l'écran X. Notre shadPS4 est monté en /ours.
FROM streamex-ps4:latest
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends imagemagick x11-apps python3 \
 && rm -rf /var/lib/apt/lists/*
COPY test.sh /test.sh
RUN chmod +x /test.sh
ENTRYPOINT ["/test.sh"]
