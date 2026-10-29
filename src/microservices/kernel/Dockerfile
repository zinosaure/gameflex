# kernel service — dev-focused image mirroring golang/Dockerfile: Air
# watches /src (mounted) and rebuilds to /app/sessions. No CGo, no GPU.

# ════════════════════════════════════════════════════════════════
#  release — le binaire seul, pour une installation publique.
#
#  `docker build --target release` : ni Air, ni Go, ni sources dans l'image.
#  Placée AVANT l'étape de développement, qui reste la dernière et donc celle
#  que construisent `docker compose build` et `launch.sh` : le dev ne change
#  pas. Pas de `go mod tidy` : go.sum est committé, un build de release doit
#  échouer sur un go.sum incomplet plutôt que le réparer en silence.
#
#  Le kernel est ouvert (chantier 34, le porteur, 2026-10-28) : le dépôt public
#  le construit depuis sa source, comme ici.
# ════════════════════════════════════════════════════════════════
FROM golang:1.23-bookworm AS release-builder

COPY ./microservices/shared/ /shared/
WORKDIR /src
COPY ./microservices/kernel/src/ ./
RUN mkdir -p /app && CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -trimpath -ldflags "-s -w" -o /app/sessions ./cmd/sessions/

FROM debian:bookworm-slim AS release

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/*

COPY --from=release-builder /app/sessions /app/sessions

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA}

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

CMD ["/app/sessions"]

# ════════════════════════════════════════════════════════════════
#  dev — Air, sur les sources montées (dernière étape : la cible par défaut)
# ════════════════════════════════════════════════════════════════
FROM golang:1.23-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git wget \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/air-verse/air@v1.61.7

WORKDIR /app
COPY ./microservices/kernel/src/ ./

# Shared console registry — go.mod replaces gameflex/consoles => /shared/consoles
# (absolute path). At runtime the volume ./src/microservices/shared:/shared
# overrides this copy; it exists here so `go mod tidy`/`go build` can resolve
# the replacement during this image stage too.
COPY ./microservices/shared/ /shared/

RUN go mod tidy && go build -o /app/sessions ./cmd/sessions/

# Air config — watches /src (mounted volume), rebuilds to /app/sessions.
# rerun=true: if the binary dies on its own, Air relaunches it instead of
# leaving the container up with the service dead (same reasoning as
# golang/Dockerfile).
# AIR SURVEILLE AUSSI LE MODULE PARTAGÉ (2026-10-15). Il ne regardait que
# /src : un changement de /shared (le registre des consoles, les paramètres
# livrés des émulateurs, embarqués en JSON) n'était pris qu'au prochain
# changement d'un .go du service — le kernel de dev a tourné 32 heures sur un
# binaire du 13 octobre, sans les paramètres de la PS4 et de la PS5 que le
# téléphone et la release avaient. La racine est donc « / », limitée à src et
# shared ; les fichiers JSON comptent.
RUN cat > /app/.air.toml <<'EOF'
root = "/"
tmp_dir = "/tmp/air"

[build]
    cmd = "sh -c 'cd /src && go mod tidy && go build -o /app/sessions ./cmd/sessions/'"
    bin = "/app/sessions"
    full_bin = "/app/sessions"
    include_dir = ["src", "shared"]
    include_ext = ["go", "mod", "json"]
    exclude_dir = ["src/vendor", "src/tmp"]
    delay = 500
    rerun = true
    rerun_delay = 500

[log]
    time = false
EOF

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --retries=3 \
    CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

CMD ["air", "-c", "/app/.air.toml"]
