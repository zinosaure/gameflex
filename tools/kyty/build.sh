#!/usr/bin/env bash
# Construit KytyPS5 depuis NOTRE FOURCHE — https://github.com/zinosaure/KytyPS5 —, à la révision
# épinglée ci-dessous (FORK_REV, FORK_REV_PHONE). Deux branches : `gameflex`, le serveur (l'éditeur à l'étiquette
# que le serveur emploie — consoles/ps5/Dockerfile, KYTY_RELEASE — plus nos commits 0007 à 0060
# puis les refontes), et `gameflex-phone-2`, le téléphone (la même, plus les commits 0001 à 0006 :
# l'espace d'adresses compact et le chemin sans extension barycentrique). Chaque commit est décrit
# dans README.md, par son numéro ou son titre.
#
# Plus de fichiers de patch depuis le 2026-11-09 (le porteur : « il faut du vrai changement de
# code »). Le code se lit, se corrige et se construit dans la fourche comme n'importe quel code ;
# pour changer Kyty : un commit sur la branche, poussé sur la fourche, puis FORK_REV avancé ici.
#
#   tools/kyty/build.sh [--server] [dossier de sortie]     (défaut : ./kyty-out)
#
# --server : la construction du serveur (cores/ps5/) — la branche gameflex, sans
# KYTY_COMPACT_ADDRESS_SPACE, dans /src-server. Sans option, le téléphone : gameflex-phone-2, avec
# l'espace compact, dans /src. Avec les treize et l'espace compact, Kyty s'arrête sur la libc de
# FF7 Rebirth : les jeux Unreal PS5 réservent 512 Gio, plus que l'espace compact (README, #524).
#
# Même principe que tools/shadps4/build.sh : un conteneur Debian trixie gardé entre deux
# lancements, clang + lld comme la CI de Kyty, ses dépendances en submodules superficiels, pas de
# lanceur Qt (le serveur n'emploie que `kyty_emulator --game`). Le résultat est un x86_64 lié
# dynamiquement à trixie.
set -euo pipefail

FORK=https://github.com/zinosaure/KytyPS5.git
# La révision construite de chaque branche ; à avancer avec la fourche (README.md).
FORK_REV=31dcca8753ccac211ebc159b77c09346bf108462
FORK_REV_PHONE=5497652698a42d5d5a9c74b83ce642fd28176591

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
SERVER=0
if [ "${1:-}" = --server ]; then SERVER=1; shift; fi
OUT=${1:-$PWD/kyty-out}
CONTAINER=${KYTY_BUILD_CONTAINER:-gameflex-kyty-build}
SRC=/src COMPACT=ON BRANCH=gameflex-phone-2 REV=$FORK_REV_PHONE
if [ "$SERVER" = 1 ]; then
    SRC=/src-server COMPACT=OFF BRANCH=gameflex REV=$FORK_REV
fi
TAG=$(sed -n 's/^ARG KYTY_RELEASE=//p' "$ROOT/src/microservices/runtimes/streamex/consoles/ps5/Dockerfile")
JOBS=${JOBS:-$(( $(nproc) > 4 ? $(nproc) - 4 : 2 ))}

[ -n "$TAG" ] || { echo "✗ KYTY_RELEASE introuvable dans le Dockerfile de la PS5" >&2; exit 1; }
mkdir -p "$OUT"

if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker run -d --name "$CONTAINER" debian:trixie sleep infinity > /dev/null
fi
docker start "$CONTAINER" > /dev/null

echo "KytyPS5 $TAG, fourche $BRANCH@${REV:0:12}, $JOBS tâches, dans $CONTAINER"
# -i : le script passe par l'entrée standard.
docker exec -i -e FORK="$FORK" -e FORK_REV="$REV" -e BRANCH="$BRANCH" -e JOBS="$JOBS" -e SRC="$SRC" -e COMPACT="$COMPACT" "$CONTAINER" bash -s <<'EOF2'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive

if ! command -v clang > /dev/null; then
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends build-essential clang lld git ca-certificates \
        cmake ninja-build pkg-config glslang-tools libasound2-dev libdbus-1-dev libgl1-mesa-dev \
        libpulse-dev libudev-dev libwayland-dev wayland-protocols libx11-dev libxcursor-dev \
        libxext-dev libxfixes-dev libxi-dev libxkbcommon-dev libxrandr-dev libxss-dev libxtst-dev \
        libvulkan-dev > /tmp/apt.log 2>&1 || { tail -5 /tmp/apt.log; exit 1; }
fi

# Un arbre d'une autre origine (l'ancien clone de l'éditeur plus ses patchs) se reclone.
if [ -d "$SRC/.git" ] && [ "$(git -C "$SRC" remote get-url origin 2>/dev/null)" != "$FORK" ]; then
    echo "→ $SRC n'est pas un clone de la fourche : nouveau clone"
    rm -rf "$SRC"
fi
if [ ! -d "$SRC/.git" ]; then
    git clone -q --branch "$BRANCH" --recurse-submodules --shallow-submodules "$FORK" "$SRC"
fi
cd "$SRC"
if [ "$(git rev-parse HEAD)" != "$FORK_REV" ]; then
    git fetch -q origin "$BRANCH"
    git checkout -q --detach "$FORK_REV"
    git submodule update -q --init --recursive --depth 1
fi
git status --short | grep -q . && { echo "✗ $SRC a des modifications locales : les ranger avant de construire" >&2; git status --short | head; exit 1; }
mkdir -p build
cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ \
    -DKYTY_BUILD_LAUNCHER=OFF -DKYTY_COMPACT_ADDRESS_SPACE="$COMPACT" \
    > build/configure.log 2>&1 || { tail -20 build/configure.log; exit 1; }
cmake --build build --parallel "$JOBS" --target kyty_emulator
found=$(find build -maxdepth 3 -type f -name kyty_emulator | head -1)
[ -n "$found" ] || { echo "✗ kyty_emulator introuvable dans build/" >&2; exit 1; }
cp "$found" /tmp/kyty_emulator
EOF2

docker cp "$CONTAINER:/tmp/kyty_emulator" "$OUT/kyty_emulator"
echo "→ $OUT/kyty_emulator ($(du -h "$OUT/kyty_emulator" | cut -f1) — KytyPS5 $TAG, fourche $BRANCH@${REV:0:12}, KYTY_COMPACT_ADDRESS_SPACE=$COMPACT)"
