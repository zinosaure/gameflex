#!/usr/bin/env bash
# Construit shadPS4 avec les patchs de ce dossier, dans l'ordre de leur numéro. La
# source : le commit de l'amont écrit dans COMMIT s'il existe — `main`, plus récent
# que la dernière version publiée (le porteur, 2026-10-27 : le rendu de la 0.20
# arrêtait FIFA 22) —, sinon l'étiquette que le serveur emploie (consoles/ps4/
# Dockerfile, SHADPS4_VERSION). Le binaire remplace celui de l'AppImage de cette
# version, et garde son nom (shadps4-gameflex-<version>) :
#   0001 le réglage `user_vaddr_max_in_gbytes`, qui borne la plage d'adresses du
#        jeu pour qu'elle tienne dans les 39 bits d'un téléphone (chantier 17, #367) ;
#   0002 la souris en mode pavé tactile ne bloque plus que ses propres boutons
#        (elle avalait le stick gauche vertical, le stick droit vertical, rond et
#        triangle, #424) ;
#   0003 la passe statique des patchs CPU n'a lieu que si l'un d'eux peut s'appliquer :
#        sur un AMD avec SSE4a (Box64 en BOX64_CPUTYPE=1, comme la console), elle
#        parcourait tout le code du jeu pour rien — 108 s pour Final Fantasy VII
#        Remake sur le S25 (#503) ;
#   0004 un signal du jeu n'est délivré qu'à un point sûr — dans son code, ou dans
#        une attente émulée qui ne tient aucun verrou de l'hôte —, et relancé jusque-là
#        sinon : les jeux Unity (IL2CPP) se figeaient au premier arrêt de leurs fils par
#        le ramasse-miettes, suspendus au milieu du journal ou de malloc (#510) ;
#   0005 le lecteur vidéo (AvPlayerState) relâche son verrou avant de prévenir le jeu :
#        le rappel du jeu, sur le même fil, le reprenait (EDEADLK), et shadPS4
#        s'arrêtait à la première vidéo de Kingdom Hearts Melody of Memory (#511) ;
#   0006 les manettes d'un serveur DSU (cemuhook), GAMEFLEX_DSU_SERVER=<ip>:<port>,
#        deviennent quatre manettes virtuelles de SDL, données aux joueurs 1 à 4 :
#        celles du téléphone, dont le clavier X n'avait de touches que pour un
#        joueur (#509). Sans la variable, rien ne change (le serveur) ;
#   0007 le fil de service du noyau prend ses demandes AVANT de faire tourner
#        io_context, au lieu de les effacer après : une minuterie programmée au
#        moment où run() finissait était perdue, et le jeu l'attendait sans fin —
#        FIFA 22 et FC 27 au démarrage, une fois sur deux (#512). Défaut de l'amont,
#        encore sur main le 2026-10-27 ;
#   0008 (repris de l'amont, b3d46e9) n'a plus lieu d'être : main l'a ;
#   0009 une tâche audio (AJM) pour une instance que le jeu vient de détruire
#        échoue (paramètre invalide) au lieu d'arrêter l'émulateur : un lot lancé
#        avant la destruction arrivait après — FIFA 22, quand un menu coupe ses
#        sons (#513). Encore sur main le 2026-10-27.
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

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
OUT=${1:-$PWD/shadps4-out}
CONTAINER=${SHADPS4_BUILD_CONTAINER:-gameflex-shadps4-build}
PATCHES=("$HERE"/0*.patch)
VERSION=$(sed -n 's/^ARG SHADPS4_VERSION=//p' "$ROOT/src/microservices/runtimes/streamex/consoles/ps4/Dockerfile")
TAG="v.${VERSION}"
REF=$TAG
[ -f "$HERE/COMMIT" ] && REF=$(tr -d '[:space:]' < "$HERE/COMMIT")
# Quelques cœurs laissés à la machine : la pile de dev et une partie en cours
# tournent à côté.
JOBS=${JOBS:-$(( $(nproc) > 4 ? $(nproc) - 4 : 2 ))}

[ -n "$VERSION" ] || { echo "✗ SHADPS4_VERSION introuvable dans le Dockerfile de la PS4" >&2; exit 1; }
mkdir -p "$OUT"

if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker run -d --name "$CONTAINER" debian:trixie sleep infinity > /dev/null
fi
docker start "$CONTAINER" > /dev/null
# Un seul fichier pour `git apply` : les patchs touchent des fichiers distincts.
cat "${PATCHES[@]}" | docker exec -i "$CONTAINER" sh -c 'cat > /tmp/gameflex.patch'

echo "shadPS4 $REF + $(cd "$HERE" && echo 0*.patch), $JOBS tâches, dans $CONTAINER"
# -i : le script passe par l'entrée standard ; sans lui, bash -s ne lit rien et
# sort aussitôt, sans erreur.
docker exec -i -e TAG="$TAG" -e REF="$REF" -e JOBS="$JOBS" "$CONTAINER" bash -s <<'EOF'
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

if [ ! -d /src/.git ]; then
    git clone -q --depth 1 --branch "$TAG" --recurse-submodules --shallow-submodules \
        https://github.com/shadps4-emu/shadPS4.git /src
fi

cd /src
# Un arbre vierge avant le patch : une relance ne doit pas l'appliquer deux fois.
# Tout l'arbre suivi (0006 touche CMakeLists.txt, à la racine), et les fichiers
# qu'un patch ajoute (0006 : src/input/gameflex_dsu.*) — pas build/, hors de src.
git reset -q
git checkout -q -- .
git clean -fdq -- src
# La source voulue (COMMIT ou l'étiquette), et ses externals : un clone superficiel
# ne la connaît pas forcément, on va la chercher.
git fetch -q --depth 1 origin "$REF"
if [ "$(git rev-parse HEAD)" != "$(git rev-parse FETCH_HEAD)" ]; then
    git checkout -q FETCH_HEAD
    git submodule update -q --init --recursive --depth 1
fi
# Une nouvelle version de l'amont peut déplacer le contexte du patch : on tente
# alors la fusion à trois voies, et on le dit — ou on s'arrête, en le disant.
if ! git apply /tmp/gameflex.patch 2> /tmp/apply.err; then
    if git apply --3way /tmp/gameflex.patch 2>> /tmp/apply.err; then
        echo "⚠ patch appliqué par fusion à trois voies sur $REF : relire (docker exec <conteneur> git -C /src diff)" >&2
    else
        echo "✗ le patch ne s'applique plus sur $REF — à rebaser (README.md) :" >&2
        cat /tmp/apply.err >&2
        exit 1
    fi
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
echo "→ $OUT/shadps4 ($(du -h "$OUT/shadps4" | cut -f1) — shadPS4 $REF et ses patchs)"
