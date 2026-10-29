# credential service — dev-focused image mirroring kernel/Dockerfile: Air
# watches /src (mounted) and rebuilds to /app/auth. No CGo (modernc sqlite).
# Go 1.25 depuis le 2026-09-01 : modernc.org/sqlite l'exige à partir de la
# version qui n'appelle plus lstat directement — un appel qu'Android tue au
# lancement (SIGSYS/SECCOMP). Le service tourne maintenant AUSSI dans
# l'application, donc son pilote de base doit convenir aux deux.

# ════════════════════════════════════════════════════════════════
#  release — le binaire seul, pour une installation publique. Même principe
#  que le kernel : avant l'étape de développement, qui reste la cible par
#  défaut ; pas de `go mod tidy`.
# ════════════════════════════════════════════════════════════════
FROM golang:1.25-bookworm AS release-builder

COPY ./microservices/shared/ /shared/
WORKDIR /src
COPY ./microservices/kernel/extras/credential/src/ ./
RUN CGO_ENABLED=0 GOFLAGS=-mod=readonly go build -trimpath -ldflags "-s -w" -o /out/auth ./cmd/auth/

FROM debian:bookworm-slim AS release

RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates tzdata wget \
    && rm -rf /var/lib/apt/lists/*

COPY --from=release-builder /out/auth /app/auth

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA}

EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["/app/auth"]

# ════════════════════════════════════════════════════════════════
#  dev — Air, sur les sources montées (dernière étape : la cible par défaut)
# ════════════════════════════════════════════════════════════════
FROM golang:1.25-bookworm

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git wget \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/air-verse/air@v1.61.7

WORKDIR /app
COPY ./microservices/kernel/extras/credential/src/ ./

# Le module partagé, au MÊME chemin absolu qu'à l'exécution (/shared) — les
# `replace` du go.mod y pointent en dur. Même geste que microservices/kernel.
COPY ./microservices/shared/ /shared/

RUN go mod tidy && go build -o /app/auth ./cmd/auth/

# Air config — watches /src (mounted volume), rebuilds to /app/auth.
# rerun=true: if the binary dies on its own, Air relaunches it instead of
# leaving the container up with the service dead.
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
    cmd = "sh -c 'cd /src && go mod tidy && go build -o /app/auth ./cmd/auth/'"
    bin = "/app/auth"
    full_bin = "/app/auth"
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
    CMD wget -qO- http://localhost:8080/health || exit 1

CMD ["air", "-c", "/app/.air.toml"]
