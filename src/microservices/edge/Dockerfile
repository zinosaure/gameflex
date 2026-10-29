# ════════════════════════════════════════════════════════════════
#  edge — le frontal d'une installation publique : la page construite et le
#  relais /api/v1/<domaine> (voir src/cmd/edge/main.go). Une seule cible : en
#  développement, c'est le serveur de webpack qui tient ce rôle.
#
#  Contexte : ./src. L'étape de la page lit webapp/src, celle du binaire
#  microservices/{edge,shared/webpage}. Dockerfile.dockerignore ne laisse
#  entrer que ça — ni node_modules, ni dist/, ni public/downloads : l'APK et
#  les émulateurs du téléphone ne sont pas redistribués.
# ════════════════════════════════════════════════════════════════

# ── La page ──────────────────────────────────────────────────────
FROM node:20-alpine AS page

WORKDIR /src
COPY ./webapp/src/package.json ./webapp/src/package-lock.json ./
# npm ci, sur le fichier de verrouillage committé (chantier 34) : sans lui, deux
# constructions de la même version tiraient des dépendances différentes — et le
# dépôt public doit reconstruire la même page.
RUN npm ci && npm cache clean --force

COPY ./webapp/src/app ./app
COPY ./webapp/src/public ./public
COPY ./webapp/src/webpack.config.js ./webpack.config.js

# Lus par webpack (DefinePlugin) : le pied de page affiche la version.
ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
RUN GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA} \
    npx webpack --mode production

# ── Le frontal ───────────────────────────────────────────────────
FROM golang:1.23-bookworm AS server

# Au même chemin absolu qu'ailleurs : le `replace` du go.mod y pointe.
COPY ./microservices/shared/webpage/ /shared/webpage/
WORKDIR /src
COPY ./microservices/edge/src/ ./
RUN CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -trimpath -ldflags "-s -w" -o /out/edge ./cmd/edge/

# ── L'image ──────────────────────────────────────────────────────
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata \
    && rm -rf /var/lib/apt/lists/*

COPY --from=page /src/dist /app/web
COPY --from=server /out/edge /app/edge
COPY ./webapp/src/api-domains.json /app/api-domains.json

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA}

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/app/edge", "healthcheck"]

CMD ["/app/edge"]
