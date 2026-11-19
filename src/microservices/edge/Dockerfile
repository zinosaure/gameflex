# ════════════════════════════════════════════════════════════════
#  edge — l'interface de GameFlex et son frontal (chantier 56).
#
#  `--target release` : la page d'edge construite et le relais /api/v1/<domaine>
#  (voir src/cmd/edge/main.go), l'image d'une installation publique.
#  La DERNIÈRE étape, celle que construit `docker compose build`, est celle du
#  développement : le serveur de webpack, qui tient le même rôle sur la pile de
#  dev (le modèle du kernel : la release d'abord, le dev en dernier).
#
#  L'interface vit dans src/, à côté du Go (app/, public/, package.json,
#  webpack.config.mjs) : chantier 56, P2.
#
#  CONTEXTE : LA RACINE DU DÉPÔT (chantier 56, P3), comme les consoles de
#  streamex. La page inscrit dans son bundle le fichier LICENSE du dépôt
#  (Réglages › À propos, chantier 53 P13) : une seule source, lue à la
#  construction, qui n'est pas sous src/. Dockerfile.dockerignore ne laisse
#  entrer que ce que lisent les COPY — ni node_modules, ni dist/, ni
#  public/downloads : l'APK et les émulateurs du téléphone ne sont pas
#  redistribués.
#
#  Depuis la bascule (P5), la page ne lit plus rien de la webapp : toute
#  l'interface et la table des domaines de l'API (api-domains.json, que lisent
#  aussi webpack et le frontal) sont dans src/.
# ════════════════════════════════════════════════════════════════

# ── La page ──────────────────────────────────────────────────────
FROM node:20-alpine AS page

WORKDIR /src
COPY src/microservices/edge/src/package.json src/microservices/edge/src/package-lock.json ./
# npm ci, sur le fichier de verrouillage committé (chantier 34) : sans lui, deux
# constructions de la même version tiraient des dépendances différentes — et le
# dépôt public doit reconstruire la même page.
RUN npm ci && npm cache clean --force

COPY src/microservices/edge/src/app ./app
COPY src/microservices/edge/src/public ./public
COPY src/microservices/edge/src/webpack.config.mjs ./webpack.config.mjs
COPY LICENSE ./LICENSE

# Lus par webpack : la version de l'en-tête et build-info.json. Le dépôt n'est
# pas là (/repo) : webpack retombe sur ces deux arguments.
ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
RUN GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA} npm run build

# ── Le frontal ───────────────────────────────────────────────────
FROM golang:1.23-bookworm AS server

# Au même chemin absolu qu'ailleurs : le `replace` du go.mod y pointe.
COPY src/microservices/shared/webpage/ /shared/webpage/
WORKDIR /src
# Le Go seul : l'interface, à côté, ne sert pas au binaire.
COPY src/microservices/edge/src/go.mod ./
COPY src/microservices/edge/src/cmd/ ./cmd/
COPY src/microservices/edge/src/internal/ ./internal/
RUN CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -trimpath -ldflags "-s -w" -o /out/edge ./cmd/edge/

# ── L'image de la release ────────────────────────────────────────
FROM debian:bookworm-slim AS release

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=page /src/dist /app/web
COPY --from=server /out/edge /app/edge
COPY src/microservices/edge/src/api-domains.json /app/api-domains.json

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA}

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/edge", "healthcheck"]

CMD ["/app/edge"]

# ── Le développement — la dernière étape ─────────────────────────
# Le serveur de développement webpack (l'ancien src/launcher/Dockerfile), sans
# le Chromium ni nodemon de la webapp. Le compose de dev monte les sources
# par-dessus : l'image n'en a besoin que pour démarrer seule.
FROM node:20-alpine AS dev

WORKDIR /src

# npm ci sur le verrou committé : la même chaîne à chaque reconstruction.
COPY src/microservices/edge/src/package.json src/microservices/edge/src/package-lock.json ./
RUN npm ci && npm cache clean --force

COPY src/microservices/edge/src/app ./app
COPY src/microservices/edge/src/public ./public
COPY src/microservices/edge/src/webpack.config.mjs ./webpack.config.mjs
COPY LICENSE ./LICENSE

EXPOSE 3000

# 127.0.0.1 et pas localhost : webpack-dev-server écoute en IPv4, le wget de
# busybox essaie ::1 d'abord (la même leçon que la webapp).
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://127.0.0.1:3000/ || exit 1

CMD [ "npm", "start" ]
