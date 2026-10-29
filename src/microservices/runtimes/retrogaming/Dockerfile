# ============================================================
# Development stage: Go + Air for hot-reload
# ============================================================
# Debian bookworm (not Alpine) — the libretro bridge uses CGo to dlopen()
# prebuilt Libretro .so cores built against glibc; loading them on Alpine's
# musl fails at dlopen with missing-symbol errors. Same requirement as the
# monolith this code was extracted from.
FROM golang:1.23-bookworm AS dev

# libegl1-mesa-dev/libgles2-mesa-dev/libgbm-dev: headers + .so needed to
# CGo-compile and link internal/retrogaming/gl_context (headless EGL/GL for
# hardware-rendered cores — N64/parallel_n64, PS2/play). mesa-utils is for
# manual debugging, not a build dependency.
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config \
    libegl1-mesa-dev libgles2-mesa-dev libgbm-dev mesa-utils \
    && rm -rf /var/lib/apt/lists/*

RUN go install github.com/air-verse/air@v1.61.7

WORKDIR /app

# ============================================================
# Backend build stage
# ============================================================
FROM dev AS backend-builder

COPY ./microservices/runtimes/retrogaming/src/ ./
# Shared console registry module (replace in go.mod → /shared/consoles)
COPY ./microservices/shared /shared

WORKDIR /app

# CGO_ENABLED=1 — required for the Libretro CGo bridge (dlopen/dlsym via
# cgo's #cgo LDFLAGS: -ldl, see internal/retrogaming/libretro/bridge.go).
RUN go mod tidy && CGO_ENABLED=1 go build -o /app/retrogaming ./cmd/retrogaming/

# ════════════════════════════════════════════════════════════════
#  release — le binaire seul, pour une installation publique. Même principe
#  que le kernel : avant l'étape de développement, qui reste la cible par
#  défaut ; pas de `go mod tidy`.
#
#  CGo : le compilateur et les en-têtes EGL/GLES restent dans l'étape de
#  build ; l'image ne garde que les bibliothèques qu'on charge à l'exécution
#  (EGL/GLES/GBM et les pilotes Mesa, pour les cœurs rendus sur GPU) et
#  ffmpeg, que l'encodeur lance.
# ════════════════════════════════════════════════════════════════
FROM golang:1.23-bookworm AS release-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential pkg-config libegl1-mesa-dev libgles2-mesa-dev libgbm-dev \
    && rm -rf /var/lib/apt/lists/*

COPY ./microservices/shared/ /shared/
WORKDIR /src
COPY ./microservices/runtimes/retrogaming/src/ ./
RUN CGO_ENABLED=1 GOFLAGS=-mod=readonly go build -trimpath -ldflags "-s -w" -o /out/retrogaming ./cmd/retrogaming/

FROM debian:bookworm-slim AS release

# Même étiquette que l'image de dev : une image de partie n'a pas de conteneur
# permanent, un élagage la ramasserait (voir plus bas).
LABEL gameflex.prune=keep

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata ffmpeg \
    libegl1 libgles2 libgbm1 libegl-mesa0 libgl1-mesa-dri \
    && rm -rf /var/lib/apt/lists/*

COPY --from=release-builder /out/retrogaming /app/retrogaming

# Même raison que l'image de dev (voir plus bas).
ENV GODEBUG=asyncpreemptoff=1

ARG GAMEFLEX_VERSION=0.0.0
ARG GITHUB_COMMIT_SHA=dev
ENV GAMEFLEX_VERSION=${GAMEFLEX_VERSION} GITHUB_COMMIT_SHA=${GITHUB_COMMIT_SHA}

# Cette image ne tourne que par le kernel, qui lance chaque partie avec
# `/app/retrogaming retrogaming-child <session>`. Lancée seule, elle attend :
# c'est ce qui la garde référencée entre deux parties.
CMD ["sleep", "infinity"]

# ============================================================
# Runtime stage — Go included for manual rebuilds / Air hot-reload
# (dernière étape : la cible par défaut)
# ============================================================
FROM golang:1.23-bookworm

# GARDÉE À L'ABRI DE `docker system prune -a`.
#
# Une image de session n'a pas de conteneur permanent : entre deux parties,
# rien ne la référence, et un élagage la considère donc comme inutilisée. Elle
# part, et la partie suivante répond « l'image de cette console est en cours de
# reconstruction » — le kernel la rebâtit tout seul, mais il y en a pour
# plusieurs minutes, à chaque fois (vécu le 2026-09-07, et déjà le 2026-08-17).
#
# Cette étiquette est ce qu'un élagage peut filtrer :
#
#     docker system prune -a --filter "label!=gameflex.prune=keep"
#
# Voir `tools/prune.sh`, qui l'écrit pour ne pas avoir à s'en souvenir.
LABEL gameflex.prune=keep


# ffmpeg: encodes libretro's raw video/audio output to H.264/Opus for
# WebRTC (internal/retrogaming/encoder).
# build-essential/pkg-config: CGo needs a C compiler at rebuild time too
# (Air recompiles inside this same running container on file changes).
# libegl1-mesa-dev/libgles2-mesa-dev/libgbm-dev: same reasoning as the dev
# stage — Air's CGo rebuilds happen IN this runtime image.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata git build-essential pkg-config ffmpeg wget iproute2 \
    libegl1-mesa-dev libgles2-mesa-dev libgbm-dev \
    && rm -rf /var/lib/apt/lists/*

# Air binary from dev stage
COPY --from=dev /go/bin/air /usr/local/bin/air

# Pre-built binary (used on first start before any rebuild)
COPY --from=backend-builder /app/retrogaming /app/retrogaming

# Module cache from builder so Air can rebuild without re-downloading deps.
# Vers /go/pkg/mod, là où Go le cherche : cet étage part de l'image golang,
# dont GOPATH vaut /go. Copié vers /root/go/pkg/mod, il était INERTE — mesuré
# le 2026-09-27 : 0 module à l'endroit utile, 6 à côté (#213).
COPY --from=backend-builder /go/pkg/mod /go/pkg/mod

# No cores are baked in — they live under the storage service's cores
# section (the host's STORAGE_PATH bind mount), like every other game-data
# folder.

# GODEBUG=asyncpreemptoff=1 — some libretro JIT cores install their own
# signal handlers on the same signals Go's async preemption uses; leaving
# preemption on made sessions crash randomly. Carried over from the
# monolith, same reason.
ENV GODEBUG=asyncpreemptoff=1

# Air config — watches /src (mounted volume), rebuilds to /app/retrogaming.
# Same shape as the monolith's own: the cd /src is required (Air runs the
# build command from its own workdir, not from root), and rerun=true keeps
# a crashed server from leaving Air sitting idle (a libretro core's fatal
# signal kills the server without touching Air itself).
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
  cmd = "sh -c 'cd /src && CGO_ENABLED=1 go build -o /app/retrogaming ./cmd/retrogaming/'"
  bin = "/app/retrogaming"
  full_bin = "/app/retrogaming"
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
