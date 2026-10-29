# ============================================================
# Development stage: Go + Air for hot-reload
# ============================================================
# Pure Go (pion/webrtc v3, stdlib net/http) — no CGo, no ffmpeg, no EGL:
# this service streams the console's already-encoded video through WebRTC,
# it doesn't encode anything itself. Much lighter than the retro-gaming
# image for exactly that reason.

# Le binaire de l'étape release, construit ou repris d'une image publiée :
# un service fermé prend son binaire en argument (chantier 34, REMOTEPLAY_BIN).
ARG REMOTEPLAY_BIN=release-builder

FROM golang:1.23-bookworm AS dev

RUN go install github.com/air-verse/air@v1.61.7

WORKDIR /app

# ============================================================
# Backend build stage
# ============================================================
FROM dev AS backend-builder

COPY ./microservices/runtimes/remoteplay/src/ ./
# Shared internal gateway transport (src/microservices/shared/gatewayclient) — Phase 3.
COPY ./microservices/shared /shared

WORKDIR /app

RUN go mod tidy && CGO_ENABLED=0 go build -o /app/remoteplay ./cmd/remoteplay/

# ════════════════════════════════════════════════════════════════
#  release — le binaire seul, pour une installation publique. Même principe
#  que le kernel : avant l'étape de développement, qui reste la cible par
#  défaut ; pas de `go mod tidy`. Le kernel lance ses parties avec
#  `/app/remoteplay remoteplay-child` : le binaire garde ce chemin.
# ════════════════════════════════════════════════════════════════
FROM golang:1.23-bookworm AS release-builder

COPY ./microservices/shared/ /shared/
WORKDIR /src
COPY ./microservices/runtimes/remoteplay/src/ ./
# LA CLÉ PUBLIQUE DE LA PASSERELLE à reconnaître (chantier 34, P9) : celle que
# tools/publish.sh a tirée pour cette version. Publique, un ARG suffit. Vide
# (le dev), la clé de développement reste.
ARG GATEWAY_SEAL_PUBLIC=
RUN mkdir -p /app && LDFLAGS="-s -w" \
    && if [ -n "$GATEWAY_SEAL_PUBLIC" ]; then LDFLAGS="$LDFLAGS -X gameflex/remoteplay/internal/api.gatewayKey=$GATEWAY_SEAL_PUBLIC"; fi \
    && CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -trimpath -ldflags "$LDFLAGS" -o /app/remoteplay ./cmd/remoteplay/

FROM ${REMOTEPLAY_BIN} AS bin

FROM debian:bookworm-slim AS release

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/*

COPY --from=bin /app/remoteplay /app/remoteplay

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA}

# PAS DE HEALTHCHECK DANS L'IMAGE : elle sert deux rôles. Le service répond
# sous /api/v1/playstation/health, une partie (`remoteplay-child`) sous
# /health, sur un autre port. Chacun a sa sonde là où il est lancé — le
# compose pour le service ; le kernel surveille lui-même ses parties.
CMD ["/app/remoteplay"]

# ============================================================
# Runtime stage — Go included for manual rebuilds / Air hot-reload
# (dernière étape : la cible par défaut)
# ============================================================
FROM golang:1.23-bookworm

# GARDÉE À L'ABRI DE `docker system prune -a`.
#
# Une image de session n'a pas de conteneur permanent : entre deux parties,
# rien ne la référence, et un élagage la considère donc comme inutilisée. Le
# kernel lance `gameflex-remoteplay:latest` par son nom (champ `image:`) —
# `internal/docker/build.go` le dit déjà — donc une image élaguée casse le
# lancement jusqu'à sa reconstruction (vécu le 2026-09-07, et déjà le
# 2026-08-17, pour les images de console).
#
# Cette étiquette est ce qu'un élagage peut filtrer :
#
#     docker system prune -a --filter "label!=gameflex.prune=keep"
#
# Voir `tools/prune.sh`, qui l'écrit pour ne pas avoir à s'en souvenir.
#
# Elle MANQUAIT ici, alors que les images de console l'avaient depuis le
# 2026-09-07 (constaté le 2026-09-23 en inspectant les étiquettes réelles).
LABEL gameflex.prune=keep

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata wget iproute2 \
    && rm -rf /var/lib/apt/lists/*

# Air binary from dev stage
COPY --from=dev /go/bin/air /usr/local/bin/air

# Pre-built binary (used on first start before any rebuild)
COPY --from=backend-builder /app/remoteplay /app/remoteplay

# Module cache from builder so Air can rebuild without re-downloading deps.
# Vers /go/pkg/mod, là où Go le cherche : cet étage part de l'image golang,
# dont GOPATH vaut /go. Copié vers /root/go/pkg/mod, il était INERTE — mesuré
# le 2026-09-27 : 4 modules à l'endroit utile, 7 à côté (#213).
COPY --from=backend-builder /go/pkg/mod /go/pkg/mod

# Air config — watches /src (mounted volume), rebuilds to /app/remoteplay.
# Same shape as the retro-gaming service's: the cd /src is required (Air
# runs the build command from its own workdir, not from root), and
# rerun=true keeps a crashed server from leaving Air sitting idle.
# AIR SURVEILLE AUSSI LE MODULE PARTAGÉ (2026-10-15). Il ne regardait que
# /src : un changement de /shared (le registre des consoles, les paramètres
# livrés des émulateurs, embarqués en JSON) n'était pris qu'au prochain
# changement d'un .go du service — le kernel de dev a tourné 32 heures sur un
# binaire du 13 octobre, sans les paramètres de la PS4 et de la PS5 que le
# téléphone et la release avaient. La racine est donc « / », limitée à src et
# shared ; les fichiers JSON comptent.
RUN mkdir -p /app && cat > /app/.air.toml <<'EOF'
root = "/"
tmp_dir = "/tmp/air"

[build]
  cmd = "sh -c 'cd /src && CGO_ENABLED=0 go build -o /app/remoteplay ./cmd/remoteplay/'"
  bin = "/app/remoteplay"
  full_bin = "/app/remoteplay"
  include_dir = ["src", "shared"]
  include_ext = ["go", "toml", "mod", "json"]
  exclude_dir = ["src/vendor", "src/tmp"]
  delay = 500
  rerun = true
  rerun_delay = 500

[log]
  time = false

[misc]
  clean_on_exit = false
EOF

WORKDIR /app
CMD ["air", "-c", "/app/.air.toml"]
