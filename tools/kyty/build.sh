#!/usr/bin/env bash
# Construit KytyPS5 depuis le DÉPÔT OFFICIEL — https://github.com/KytyPS5/KytyPS5 —, au commit de
# base épinglé ci-dessous (BASE : l'étiquette que le serveur emploie, consoles/ps5/Dockerfile,
# KYTY_RELEASE), plus NOS PATCHS. Deux jeux de patchs : `patches/`, le serveur (nos commits 0007 et
# suivants, puis les refontes et les reprises de l'amont, un patch par commit, décrits dans README.md
# par leur numéro ou leur titre), et `patches-phone/gameflex-phone.patch`, le téléphone (un seul
# patch : sa branche reçoit celle du serveur par fusion, une série ne s'y rejoue pas ; l'espace
# d'adresses compact et le chemin sans extension barycentrique en plus).
#
# PLUS DE FOURCHE depuis le 2026-11-18 (chantier 63 ; le porteur : « on travaille que par des patchs
# […] Ce que je ne veux pas c'est d'avoir des repos avec le code source des émulateurs »). Le code se
# change dans le clone de travail (/home/gixi/Github/KytyPS5, branches `gameflex` et
# `gameflex-phone-2`), puis les patchs se réexportent : tools/emulator-patches.sh export kyty
# (README.md). Le fond ne change pas (refaire ce qui est mal conçu, 2026-11-09) : seule la forme de
# livraison change.
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

UPSTREAM=https://github.com/KytyPS5/KytyPS5.git
# Le commit officiel sur lequel les patchs s'appliquent (celui de KYTY_RELEASE) ; à avancer avec eux (README.md).
BASE=a0b083c85293d55a48b4f8774285415c29d78c8b

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
SERVER=0
if [ "${1:-}" = --server ]; then SERVER=1; shift; fi
OUT=${1:-$PWD/kyty-out}
CONTAINER=${KYTY_BUILD_CONTAINER:-gameflex-kyty-build}
SRC=/src COMPACT=ON PATCHES=$HERE/patches-phone WHAT=téléphone
if [ "$SERVER" = 1 ]; then
    SRC=/src-server COMPACT=OFF PATCHES=$HERE/patches WHAT=serveur
fi
TAG=$(sed -n 's/^ARG KYTY_RELEASE=//p' "$ROOT/src/microservices/runtimes/streamex/consoles/ps5/Dockerfile")
JOBS=${JOBS:-$(( $(nproc) > 4 ? $(nproc) - 4 : 2 ))}

[ -n "$TAG" ] || { echo "✗ KYTY_RELEASE introuvable dans le Dockerfile de la PS5" >&2; exit 1; }
mkdir -p "$OUT"
ls "$PATCHES"/*.patch > /dev/null 2>&1 || { echo "✗ aucun patch dans $PATCHES" >&2; exit 1; }
# L'empreinte des patchs : un arbre déjà patché par les mêmes n'est pas refait (ninja reprend).
SERIES=$(cat "$PATCHES"/*.patch | sha256sum | cut -c1-16)

if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker run -d --name "$CONTAINER" debian:trixie sleep infinity > /dev/null
fi
docker start "$CONTAINER" > /dev/null
docker exec "$CONTAINER" rm -rf /patches
docker cp "$PATCHES" "$CONTAINER:/patches"

echo "KytyPS5 $TAG ($WHAT), officiel@${BASE:0:12} + $(ls "$PATCHES"/*.patch | wc -l) patch(s) ($SERIES), $JOBS tâches, dans $CONTAINER"
# -i : le script passe par l'entrée standard.
docker exec -i -e UPSTREAM="$UPSTREAM" -e BASE="$BASE" -e SERIES="$SERIES" -e JOBS="$JOBS" -e SRC="$SRC" -e COMPACT="$COMPACT" "$CONTAINER" bash -s <<'EOF2'
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

# Un arbre d'une autre origine (l'ancien clone de la fourche) se reclone.
if [ -d "$SRC/.git" ] && [ "$(git -C "$SRC" remote get-url origin 2>/dev/null)" != "$UPSTREAM" ]; then
    echo "→ $SRC n'est pas un clone du dépôt officiel : nouveau clone"
    rm -rf "$SRC"
fi
if [ ! -d "$SRC/.git" ]; then
    git init -q "$SRC"
    git -C "$SRC" remote add origin "$UPSTREAM"
fi
cd "$SRC"
# Le commit de base et ses dépendances, puis les patchs — refaits seulement quand l'un ou l'autre change. En commits,
# d'une identité et à des dates fixes : les mêmes patchs donnent la même révision.
if [ "$(cat .git/gameflex-series 2>/dev/null)" != "$BASE $SERIES" ]; then
    git cat-file -e "$BASE^{commit}" 2>/dev/null || git fetch -q --depth 1 origin "$BASE"
    git checkout -q -f --detach "$BASE"
    git clean -q -fd -e build
    git submodule update -q --init --recursive --depth 1
    export GIT_AUTHOR_NAME=GameFlex GIT_AUTHOR_EMAIL=gameflex@users.noreply.github.com
    export GIT_COMMITTER_NAME=GameFlex GIT_COMMITTER_EMAIL=gameflex@users.noreply.github.com
    if head -1 /patches/*.patch | grep -q '^From '; then
        git am -q --keep-cr --committer-date-is-author-date /patches/*.patch \
            || { git am --abort 2>/dev/null; echo "✗ les patchs ne s'appliquent pas sur $BASE" >&2; exit 1; }
    else
        # Un patch unique (le téléphone) : un commit daté comme la base.
        git apply --index /patches/*.patch || { echo "✗ le patch ne s'applique pas sur $BASE" >&2; exit 1; }
        date=$(git log -1 --format=%cI "$BASE")
        GIT_AUTHOR_DATE=$date GIT_COMMITTER_DATE=$date git commit -q -m "gameflex: the phone's patch"
    fi
    echo "$BASE $SERIES" > .git/gameflex-series
fi
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
echo "→ $OUT/kyty_emulator ($(du -h "$OUT/kyty_emulator" | cut -f1) — KytyPS5 $TAG ($WHAT), officiel@${BASE:0:12} + $SERIES, KYTY_COMPACT_ADDRESS_SPACE=$COMPACT)"
