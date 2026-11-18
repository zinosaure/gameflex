#!/usr/bin/env bash
# Construit shadPS4 depuis le DÉPÔT OFFICIEL — https://github.com/shadps4-emu/shadPS4 —, au commit
# de base épinglé ci-dessous (BASE : un commit de `main`, 7c0b846d du 2026-11-17 — plus récent que la
# dernière version publiée : le porteur, 2026-10-27, le rendu de la 0.20 arrêtait FIFA 22), plus NOS
# PATCHS, `patches/00NN-gameflex-00NN-….patch`, chacun décrit dans README.md par son numéro. Une seule
# série : le serveur et le téléphone (sous Box64, tools/box64/bundle.sh) prennent le même binaire.
#
# PLUS DE FOURCHE depuis le 2026-11-18 (chantier 63 ; le porteur : « on travaille que par des patchs
# […] Ce que je ne veux pas c'est d'avoir des repos avec le code source des émulateurs »). Le code se
# change dans le clone de travail (/home/gixi/Github/shadPS4, branche `gameflex`), puis la série se
# réexporte : tools/emulator-patches.sh export shadps4 (README.md).
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

UPSTREAM=https://github.com/shadps4-emu/shadPS4.git
# Le commit officiel sur lequel la série s'applique ; à avancer avec elle (README.md).
BASE=7c0b846d0579c7c13b4c2d76032227b68615d068

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
ls "$HERE"/patches/*.patch > /dev/null 2>&1 || { echo "✗ aucun patch dans $HERE/patches" >&2; exit 1; }
# L'empreinte de la série : un arbre déjà patché par la même série n'est pas refait (ninja reprend).
SERIES=$(cat "$HERE"/patches/*.patch | sha256sum | cut -c1-16)

if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker run -d --name "$CONTAINER" debian:trixie sleep infinity > /dev/null
fi
docker start "$CONTAINER" > /dev/null
docker exec "$CONTAINER" rm -rf /patches
docker cp "$HERE/patches" "$CONTAINER:/patches"

echo "shadPS4, officiel@${BASE:0:12} + $(ls "$HERE"/patches/*.patch | wc -l) patchs ($SERIES), $JOBS tâches, dans $CONTAINER"
# -i : le script passe par l'entrée standard ; sans lui, bash -s ne lit rien et
# sort aussitôt, sans erreur.
docker exec -i -e UPSTREAM="$UPSTREAM" -e BASE="$BASE" -e SERIES="$SERIES" -e JOBS="$JOBS" "$CONTAINER" bash -s <<'EOF'
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

# Un arbre d'une autre origine (l'ancien clone de la fourche) se reclone.
if [ -d /src/.git ] && [ "$(git -C /src remote get-url origin 2>/dev/null)" != "$UPSTREAM" ]; then
    echo "→ /src n'est pas un clone du dépôt officiel : nouveau clone"
    rm -rf /src
fi
if [ ! -d /src/.git ]; then
    git init -q /src
    git -C /src remote add origin "$UPSTREAM"
fi

cd /src
# Le commit de base et ses externals, puis la série — refaits seulement quand l'un ou l'autre change.
if [ "$(cat .git/gameflex-series 2>/dev/null)" != "$BASE $SERIES" ]; then
    git cat-file -e "$BASE^{commit}" 2>/dev/null || git fetch -q --depth 1 origin "$BASE"
    git checkout -q -f --detach "$BASE"
    git clean -q -fd -e build
    git submodule update -q --init --recursive --depth 1
    # En commits, d'une identité et à des dates fixes : la même série donne la même révision, celle que le journal
    # d'une partie et le titre de la fenêtre portent (`Revision`) — elle désigne la construction.
    git -c user.name=GameFlex -c user.email=gameflex@users.noreply.github.com am -q --keep-cr \
        --committer-date-is-author-date /patches/*.patch \
        || { git am --abort 2>/dev/null; echo "✗ la série ne s'applique pas sur $BASE" >&2; exit 1; }
    echo "$BASE $SERIES" > .git/gameflex-series
fi
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
echo "→ $OUT/shadps4 ($(du -h "$OUT/shadps4" | cut -f1) — officiel@${BASE:0:12} + série $SERIES ; à livrer sous le nom shadps4-gameflex-$VERSION)"
