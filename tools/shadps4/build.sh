#!/usr/bin/env bash
# Construit shadPS4 depuis NOTRE FOURCHE — https://github.com/zinosaure/shadPS4 —, branche
# `gameflex`, à la révision épinglée ci-dessous (FORK_REV) : un commit de `main` de l'amont
# (7c0b846d, du 2026-11-17 — plus récent que la dernière version publiée : le porteur, 2026-10-27,
# le rendu de la 0.20 arrêtait FIFA 22), plus nos commits `gameflex 00NN: …`, chacun décrit dans
# README.md par son numéro. Une seule branche : le serveur et le téléphone (sous Box64,
# tools/box64/bundle.sh) prennent le même binaire, les réglages du téléphone sont des réglages
# (0001) ou des variables d'environnement (0006).
#
# Plus de fichiers de patch depuis le 2026-11-11, comme pour Kyty (le porteur : « il faut du vrai
# changement de code »). Changer shadPS4, c'est un commit sur la branche, poussé sur la fourche,
# puis FORK_REV avancé ici — et son entrée dans README.md.
#
# Le binaire remplace celui de l'AppImage de la version du serveur (consoles/ps4/Dockerfile,
# SHADPS4_VERSION), et en garde le nom à la livraison (shadps4-gameflex-<version>).
#
#   tools/shadps4/build.sh [dossier de sortie]     (défaut : ./shadps4-out)
#
# Tout se passe dans un conteneur Debian trixie (rien ne s'installe sur l'hôte) :
# clang 19 comme la CI de shadPS4, ses externals en submodules superficiels. Le
# conteneur est GARDÉ entre deux lancements — dépendances et sources coûtent
# plusieurs minutes, et ninja reprend un build interrompu là où il s'est arrêté.
# `docker rm -f <conteneur>` pour repartir de zéro.
#
# Le résultat est un x86_64 lié dynamiquement à trixie : il tourne dans l'image
# streamex-ps4 (trixie aussi), à la place de l'AppImage de l'amont.
set -euo pipefail

FORK=https://github.com/zinosaure/shadPS4.git
BRANCH=gameflex
# La révision construite ; à avancer avec la fourche (README.md).
FORK_REV=e441ba5de6d2d16641ee6701ac1bcc8516439170

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
OUT=${1:-$PWD/shadps4-out}
CONTAINER=${SHADPS4_BUILD_CONTAINER:-gameflex-shadps4-build}
VERSION=$(sed -n 's/^ARG SHADPS4_VERSION=//p' "$ROOT/src/microservices/runtimes/streamex/consoles/ps4/Dockerfile")
# Quelques cœurs laissés à la machine : la pile de dev et une partie en cours
# tournent à côté.
JOBS=${JOBS:-$(( $(nproc) > 4 ? $(nproc) - 4 : 2 ))}

[ -n "$VERSION" ] || { echo "✗ SHADPS4_VERSION introuvable dans le Dockerfile de la PS4" >&2; exit 1; }
mkdir -p "$OUT"

if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker run -d --name "$CONTAINER" debian:trixie sleep infinity > /dev/null
fi
docker start "$CONTAINER" > /dev/null

echo "shadPS4, fourche $BRANCH@${FORK_REV:0:12}, $JOBS tâches, dans $CONTAINER"
# -i : le script passe par l'entrée standard ; sans lui, bash -s ne lit rien et
# sort aussitôt, sans erreur.
docker exec -i -e FORK="$FORK" -e BRANCH="$BRANCH" -e FORK_REV="$FORK_REV" -e JOBS="$JOBS" "$CONTAINER" bash -s <<'EOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

if ! command -v clang > /dev/null; then
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends build-essential clang git ca-certificates cmake \
        ninja-build pkg-config libasound2-dev libpulse-dev libopenal-dev libssl-dev zlib1g-dev \
        libedit-dev libudev-dev libevdev-dev libsdl2-dev libjack-dev libsndio-dev libvulkan-dev \
        libpng-dev uuid-dev libx11-dev libxext-dev libxcursor-dev libxfixes-dev libxi-dev \
        libxrandr-dev libxss-dev libxtst-dev libxkbcommon-dev libwayland-dev libdecor-0-dev \
        libdbus-1-dev > /tmp/apt.log 2>&1 \
        || { tail -5 /tmp/apt.log; exit 1; }
fi

# Un arbre d'une autre origine (l'ancien clone de l'éditeur plus ses patchs) se reclone.
if [ -d /src/.git ] && [ "$(git -C /src remote get-url origin 2>/dev/null)" != "$FORK" ]; then
    echo "→ /src n'est pas un clone de la fourche : nouveau clone"
    rm -rf /src
fi
if [ ! -d /src/.git ]; then
    git clone -q --depth 1 --branch "$BRANCH" --recurse-submodules --shallow-submodules \
        "$FORK" /src
fi

cd /src
# La révision voulue, et ses externals : un clone superficiel ne la connaît pas
# forcément, on va la chercher.
if [ "$(git rev-parse HEAD)" != "$FORK_REV" ]; then
    git fetch -q --depth 1 origin "$FORK_REV"
    git checkout -q --detach "$FORK_REV"
    git submodule update -q --init --recursive --depth 1
fi
git status --short | grep -q . && { echo "✗ /src a des modifications locales : les ranger avant de construire" >&2; git status --short | head; exit 1; }
mkdir -p build
# SCAN_FOR_MODULES=OFF : sans lui, CMake cherche clang-scan-deps (paquet
# clang-tools) pour des modules C++20 que shadPS4 n'emploie pas.
cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ -DENABLE_QT_GUI=OFF \
    -DCMAKE_CXX_SCAN_FOR_MODULES=OFF \
    > build/configure.log 2>&1 || { tail -20 build/configure.log; exit 1; }
cmake --build build --parallel "$JOBS"
EOF

docker cp "$CONTAINER:/src/build/shadps4" "$OUT/shadps4"
echo "→ $OUT/shadps4 ($(du -h "$OUT/shadps4" | cut -f1) — fourche $BRANCH@${FORK_REV:0:12} ; à livrer sous le nom shadps4-gameflex-$VERSION)"
