# ============================================================
# Development stage: Go + Air for hot-reload
# ============================================================
# Pure Go, no CGo — same reasoning as remoteplay's own Dockerfile: this
# service streams already-encoded frames through WebRTC, it doesn't encode
# anything itself.

# Le binaire de l'étape release, construit ou repris d'une image publiée :
# un service fermé prend son binaire en argument (chantier 34, GATEWAY_BIN).
ARG GATEWAY_BIN=release-builder

FROM golang:1.23-bookworm AS dev

RUN go install github.com/air-verse/air@v1.61.7

WORKDIR /app

# ============================================================
# Backend build stage
# ============================================================
FROM dev AS backend-builder

COPY ./microservices/gatewaywebrtc/src/ ./

# Le module partagé, au MÊME chemin absolu qu'à l'exécution (/shared) — le
# `replace gameflex/gatewaystreams` du go.mod y pointe en dur, et sans ce
# COPY le `go mod tidy` ci-dessous ne peut pas le résoudre. Même geste que
# microservices/runtimes/{retrogaming,remoteplay} et le squelette streamex.
COPY ./microservices/shared/ /shared/

WORKDIR /app

RUN go mod tidy && CGO_ENABLED=0 go build -o /app/gatewaywebrtc ./cmd/gatewaywebrtc/

# ════════════════════════════════════════════════════════════════
#  release — le binaire seul, pour une installation publique. Même principe
#  que le kernel : avant l'étape de développement, qui reste la cible par
#  défaut ; pas de `go mod tidy`.
# ════════════════════════════════════════════════════════════════
FROM golang:1.23-bookworm AS release-builder

COPY ./microservices/shared/ /shared/
WORKDIR /src
COPY ./microservices/gatewaywebrtc/src/ ./
# LA CLÉ DE LA PASSERELLE (chantier 34, P9) : celle qui prouve à streamex et au
# Remote Play que cette passerelle est la nôtre. tools/publish.sh en tire une
# par version et la passe en SECRET BuildKit — jamais en ARG, qui resterait
# dans l'historique de l'image. Sans secret (le dev), la clé de développement
# inscrite dans main.go reste.
RUN --mount=type=secret,id=gateway_seal_key \
    mkdir -p /app && LDFLAGS="-s -w" \
    && if [ -s /run/secrets/gateway_seal_key ]; then LDFLAGS="$LDFLAGS -X main.gatewaySealKey=$(cat /run/secrets/gateway_seal_key)"; fi \
    && CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -trimpath -ldflags "$LDFLAGS" -o /app/gatewaywebrtc ./cmd/gatewaywebrtc/

FROM ${GATEWAY_BIN} AS bin

FROM debian:bookworm-slim AS release

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/*

COPY --from=bin /app/gatewaywebrtc /app/gatewaywebrtc

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA}

# Réseau hôte : le port d'écoute vient de HTTP_PORT.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- "http://localhost:${HTTP_PORT:-8080}/health" || exit 1

CMD ["/app/gatewaywebrtc"]

# ============================================================
# Runtime stage — Go included for manual rebuilds / Air hot-reload
# (dernière étape : la cible par défaut)
# ============================================================
FROM golang:1.23-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata wget iproute2 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=dev /go/bin/air /usr/local/bin/air
COPY --from=backend-builder /app/gatewaywebrtc /app/gatewaywebrtc
# Le cache de modules, vers /go/pkg/mod : là où Go le cherche (GOPATH=/go dans
# l'image golang). Copié vers /root/go/pkg/mod, il était INERTE — mesuré le
# 2026-09-27 : 3 modules à l'endroit utile, 6 à côté (#213).
COPY --from=backend-builder /go/pkg/mod /go/pkg/mod

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
  cmd = "sh -c 'cd /src && CGO_ENABLED=0 go build -o /app/gatewaywebrtc ./cmd/gatewaywebrtc/'"
  bin = "/app/gatewaywebrtc"
  full_bin = "/app/gatewaywebrtc"
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
