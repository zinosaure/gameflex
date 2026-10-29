#!/usr/bin/env bash
# Construit KytyPS5 — l'étiquette que le serveur emploie (consoles/ps5/Dockerfile,
# KYTY_RELEASE) — avec les patchs de ce dossier, dans l'ordre de leur numéro :
# 0001, l'option CMake `KYTY_COMPACT_ADDRESS_SPACE`, qui range tout l'espace
# d'adresses du jeu sous 512 Gio pour qu'il tienne dans les 39 bits d'un
# téléphone (chantier 17, #367) ; 0002, l'extension Vulkan barycentrique rendue
# optionnelle, parce que Turnip ne l'a pas (#373) ; 0003, les modules chargés et
# le refus de la libc du jeu dits sur la console (#373) ; 0004, les choix GPU et
# la pile d'appels d'un planificateur utilisé trop tôt, sur la console (#373) ;
# 0005, un défaut « lecture » sur une page lisible traité en écriture (#373) ;
# 0006, le suivi GPU garde le droit d'exécution des pages (#373) ; 0007, les
# lectures formatées d'un tampon choisi par le GPU, de toute largeur (#431) ;
# 0008 à 0012, ce qui arrêtait ensuite Final Fantasy VII Rebirth (#431, README) ;
# 0013, le P010 d'un décodeur vidéo ouvert en 10 bits (FF7 Remake, #431).
# 0014, les constantes du découpeur GPU choisies par des branches (les tables
# de couleurs 3D de FF7 Remake, #431).
# 0015, `KYTY_OCCLUSION_QUERIES` dans l'environnement vaut `--occlusion-queries`
# (l'interrupteur de la fiche de Kyty, #445).
# 0016, `KYTY_SHADER_CACHE` vaut `--shader-cache`, et le cache porte la révision de
# Kyty (sous Box64, la date de /proc/self/exe est celle de Box64 — #487).
# 0017, le exit() d'un jeu sort par quick_exit : les fils du jeu tournent
# encore, et les destructeurs statiques faisaient planter Kyty (#522).
#
#   tools/kyty/build.sh [--server] [dossier de sortie]     (défaut : ./kyty-out)
#
# --server : la construction du serveur (cores/ps5/) — l'éditeur et les patchs
# 0007 et suivants, sans KYTY_COMPACT_ADDRESS_SPACE, dans /src-server. Avec les
# treize et l'espace compact, Kyty s'arrête sur la libc de FF7 Rebirth : les
# jeux Unreal PS5 réservent 512 Gio, plus que l'espace compact (README, #524).
#
# Même principe que tools/shadps4/build.sh : un conteneur Debian trixie gardé
# entre deux lancements, clang + lld comme la CI de Kyty, ses dépendances en
# submodules superficiels, pas de lanceur Qt (le serveur n'emploie que
# `kyty_emulator --game`). Le résultat est un x86_64 lié dynamiquement à trixie.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
SERVER=0
if [ "${1:-}" = --server ]; then SERVER=1; shift; fi
OUT=${1:-$PWD/kyty-out}
CONTAINER=${KYTY_BUILD_CONTAINER:-gameflex-kyty-build}
PATCHES=("$HERE"/[0-9][0-9][0-9][0-9]-*.patch)
SRC=/src COMPACT=ON
if [ "$SERVER" = 1 ]; then
    SRC=/src-server COMPACT=OFF
    for patch in "${PATCHES[@]}"; do
        number=$(basename "$patch"); number=${number%%-*}
        [ $((10#$number)) -ge 7 ] && SERVER_PATCHES+=("$patch")
    done
    PATCHES=("${SERVER_PATCHES[@]}")
fi
TAG=$(sed -n 's/^ARG KYTY_RELEASE=//p' "$ROOT/src/microservices/runtimes/streamex/consoles/ps5/Dockerfile")
JOBS=${JOBS:-$(( $(nproc) > 4 ? $(nproc) - 4 : 2 ))}

[ -n "$TAG" ] || { echo "✗ KYTY_RELEASE introuvable dans le Dockerfile de la PS5" >&2; exit 1; }
mkdir -p "$OUT"

if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker run -d --name "$CONTAINER" debian:trixie sleep infinity > /dev/null
fi
docker start "$CONTAINER" > /dev/null
docker exec "$CONTAINER" sh -c 'rm -rf /tmp/gameflex-patches && mkdir -p /tmp/gameflex-patches'
for patch in "${PATCHES[@]}"; do
    docker cp "$patch" "$CONTAINER:/tmp/gameflex-patches/"
done

echo "KytyPS5 $TAG + $(basename -a "${PATCHES[@]}" | tr '\n' ' '), $JOBS tâches, dans $CONTAINER"
# -i : le script passe par l'entrée standard.
docker exec -i -e TAG="$TAG" -e JOBS="$JOBS" -e SRC="$SRC" -e COMPACT="$COMPACT" "$CONTAINER" bash -s <<'EOF'
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

# Un arbre d'une autre version se reclone : le clone est superficiel, il n'a pas de quoi
# changer d'étiquette (et ses sous-modules non plus).
if [ -d "$SRC/.git" ] && [ "$(cat "$SRC/.gameflex-tag" 2>/dev/null)" != "$TAG" ]; then
    echo "→ $SRC n'est pas à $TAG : nouveau clone"
    rm -rf "$SRC"
fi
if [ ! -d "$SRC/.git" ]; then
    git clone -q --depth 1 --branch "$TAG" --recurse-submodules --shallow-submodules \
        https://github.com/KytyPS5/KytyPS5.git "$SRC"
    echo "$TAG" > "$SRC/.gameflex-tag"
fi

cd "$SRC"
git checkout -q -- src CMakeLists.txt
# Une nouvelle version de l'amont peut déplacer le contexte d'un patch : on tente
# alors la fusion à trois voies, et on le dit — ou on s'arrête, en le disant.
for patch in /tmp/gameflex-patches/*.patch; do
    if ! git apply "$patch" 2> /tmp/apply.err; then
        if git apply --3way "$patch" 2>> /tmp/apply.err; then
            echo "⚠ $(basename "$patch") appliqué par fusion à trois voies sur $TAG : relire (docker exec <conteneur> git -C $SRC diff)" >&2
        else
            echo "✗ $(basename "$patch") ne s'applique plus sur $TAG — à rebaser (README.md) :" >&2
            cat /tmp/apply.err >&2
            exit 1
        fi
    fi
done
mkdir -p build
cmake -S . -B build -G Ninja -DCMAKE_BUILD_TYPE=Release \
    -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ \
    -DKYTY_BUILD_LAUNCHER=OFF -DKYTY_COMPACT_ADDRESS_SPACE="$COMPACT" \
    > build/configure.log 2>&1 || { tail -20 build/configure.log; exit 1; }
cmake --build build --parallel "$JOBS" --target kyty_emulator
found=$(find build -maxdepth 3 -type f -name kyty_emulator | head -1)
[ -n "$found" ] || { echo "✗ kyty_emulator introuvable dans build/" >&2; exit 1; }
cp "$found" /tmp/kyty_emulator
EOF

docker cp "$CONTAINER:/tmp/kyty_emulator" "$OUT/kyty_emulator"
echo "→ $OUT/kyty_emulator ($(du -h "$OUT/kyty_emulator" | cut -f1) — KytyPS5 $TAG, KYTY_COMPACT_ADDRESS_SPACE=$COMPACT, ${#PATCHES[@]} patchs)"
