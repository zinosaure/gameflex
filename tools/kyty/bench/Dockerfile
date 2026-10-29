# Banc d'essai : l'image PS5 du serveur (trixie, Mesa 26 RADV, Weston, Xwayland)
# + de quoi capturer l'écran X. Notre Kyty est monté en /ours.
FROM streamex-ps5:latest
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends imagemagick x11-apps \
 && rm -rf /var/lib/apt/lists/*
COPY test.sh /test.sh
RUN chmod +x /test.sh
ENTRYPOINT ["/test.sh"]
