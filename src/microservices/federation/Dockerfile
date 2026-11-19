# gameflex-federation — un serveur fédéré fait de partages Samba, WebDAV, kDrive, FTP et SFTP (chantier 61). Il se lance
# SEUL : cette image suffit, avec un volume de données et le compte de l'administrateur.
#
#   docker run -d --name gameflex-federation -p 8090:8090 -v federation:/data \
#       -e FEDERATION_ADMIN_USERNAME=admin -e FEDERATION_ADMIN_PASSWORD=… gameflex/federation
#
# Pour que le mode découverte s'annonce sur le réseau local (mDNS), `--network host` : le multicast ne traverse pas un
# bridge Docker. Sans lui, un appareil tape l'adresse du service.
#
# Contexte de construction : la racine du dépôt (comme edge). L'interface vient de la chaîne d'edge (ses briques, sa
# connexion, son cadre des Réglages : app/federation/), embarquée dans le binaire. Pas de `go mod tidy` ni de
# `npm install` libre : go.sum et package-lock.json sont committés.

FROM node:20-alpine AS interface
WORKDIR /src
COPY src/microservices/edge/src/package.json src/microservices/edge/src/package-lock.json ./
RUN npm ci && npm cache clean --force
COPY src/microservices/edge/src/app ./app
COPY src/microservices/edge/src/public ./public
COPY src/microservices/edge/src/webpack.config.js ./webpack.config.js
COPY LICENSE ./LICENSE
ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
RUN GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA} npm run build:federation

FROM golang:1.25-bookworm AS builder
COPY src/microservices/shared/ /shared/
WORKDIR /src
COPY src/microservices/federation/src/ ./
COPY --from=interface /src/dist-federation/ ./internal/web/dist/
RUN CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -trimpath -ldflags "-s -w" -o /out/federation ./cmd/federation/

FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/* \
    && useradd --system --uid 10001 --home-dir /data federation \
    && mkdir -p /data && chown federation /data

COPY --from=builder /out/federation /app/federation

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA} \
    FEDERATION_DATA=/data FEDERATION_PORT=8090

USER federation
VOLUME /data
EXPOSE 8090

# Le port d'écoute, celui que le service prend vraiment : le compose le change (10206, en réseau hôte), et une sonde
# figée sur 8090 disait le service malade alors qu'il répondait.
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -qO- "http://localhost:${FEDERATION_PORT:-8090}/api/v1/health" || exit 1

CMD ["/app/federation"]
