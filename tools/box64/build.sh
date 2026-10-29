#!/usr/bin/env bash
# Construit Box64 pour aarch64 — le traducteur x86-64 → arm64 qui doit faire
# tourner notre shadPS4 et notre Kyty (des x86_64) sur le téléphone (chantier 17,
# #367, étape 3). Compilation croisée dans un conteneur Debian trixie gardé :
# gcc-aarch64-linux-gnu et sa glibc 2.41 (le runtime du téléphone a 2.43).
#
#   tools/box64/build.sh [dossier de sortie]     (défaut : ./box64-out)
#
# Depuis `main` et non la release v0.4.4 (2026-08-02) : celle-ci n'enveloppe pas
# encore `__cxa_at_quick_exit`, que shadPS4 appelle à son initialisation — le
# Box64 0.4.3 de Debian avortait là (chantier 11, phase 2). Le commit construit
# est relevé dans la sortie, pour l'épingler quand ça joue (BOX64_REF=<commit>).
#
# Avec les patchs de ce dossier, dans l'ordre de leur numéro : 0001, le bit
# « écriture » d'un défaut de page pris dans l'ESR d'ARM64 et non deviné de
# l'opcode x86 (Kyty, shadPS4 suivent la mémoire par défauts de page — #373) ;
# 0002, une protection avec exécution implique la lecture, comme sur x86 : un
# noyau arm64 fait d'un PROT_EXEC seul une page illisible, que Box64 ne peut
# plus traduire (le gestionnaire de TLS de Kyty — #373) ; 0003, l'option
# BOX64_DYNAREC_TRUSTSHARED : le code d'un mappage partagé inscriptible protégé
# comme la mémoire privée, au lieu d'une somme de contrôle à chaque bloc exécuté
# — shadPS4 mappe toute la mémoire du jeu d'un memfd, et cette somme prenait
# 62 % de son temps sur le S25 (#506).
#
# Produit <sortie>/box64 (aarch64) et <sortie>/x64lib/ — les bibliothèques
# x86_64 que Box64 fournit lui-même, à installer sous le nom
# box64-x86_64-linux-gnu dans le lib du runtime cible (docs/COMPILE.md).
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
OUT=${1:-$PWD/box64-out}
CONTAINER=${BOX64_BUILD_CONTAINER:-gameflex-box64-build}
REF=${BOX64_REF:-main}
JOBS=${JOBS:-$(( $(nproc) > 4 ? $(nproc) - 4 : 2 ))}
mkdir -p "$OUT"

if ! docker ps -a --format '{{.Names}}' | grep -qx "$CONTAINER"; then
    docker run -d --name "$CONTAINER" debian:trixie sleep infinity > /dev/null
fi
docker start "$CONTAINER" > /dev/null
docker exec "$CONTAINER" sh -c 'rm -rf /tmp/gameflex-patches && mkdir -p /tmp/gameflex-patches'
for patch in "$HERE"/[0-9][0-9][0-9][0-9]-*.patch; do
    docker cp "$patch" "$CONTAINER:/tmp/gameflex-patches/"
done

echo "Box64 $REF, $JOBS tâches, dans $CONTAINER"
# -i : le script passe par l'entrée standard.
docker exec -i -e REF="$REF" -e JOBS="$JOBS" "$CONTAINER" bash -s <<'EOF'
set -euo pipefail
export DEBIAN_FRONTEND=noninteractive
if ! command -v aarch64-linux-gnu-gcc > /dev/null; then
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends git ca-certificates cmake ninja-build python3 \
        gcc-aarch64-linux-gnu libc6-dev-arm64-cross > /tmp/apt.log 2>&1 || { tail -5 /tmp/apt.log; exit 1; }
fi
if [ ! -d /src/.git ]; then
    git clone -q https://github.com/ptitSeb/box64.git /src
fi
cd /src
git fetch -q origin
git reset -q --hard   # les patchs d'une construction précédente
git checkout -q --detach "$REF" 2> /dev/null || git checkout -q --detach "origin/$REF"
# Une nouvelle version de l'amont peut déplacer le contexte d'un patch : on tente
# alors la fusion à trois voies, et on le dit — ou on s'arrête, en le disant.
for patch in /tmp/gameflex-patches/*.patch; do
    if ! git apply "$patch" 2> /tmp/apply.err; then
        if git apply --3way "$patch" 2>> /tmp/apply.err; then
            echo "⚠ $(basename "$patch") appliqué par fusion à trois voies : relire (git -C /src diff)" >&2
        else
            echo "✗ $(basename "$patch") ne s'applique plus sur $REF :" >&2
            cat /tmp/apply.err >&2
            exit 1
        fi
    fi
done
echo "commit : $(git rev-parse --short HEAD) ($(git log -1 --format=%cd --date=short))"
grep -q "GOM(__cxa_at_quick_exit" src/wrapped/wrappedlibc_private.h \
    && echo "__cxa_at_quick_exit : enveloppé" \
    || { echo "✗ ce Box64 n'enveloppe pas __cxa_at_quick_exit : shadPS4 avortera" >&2; exit 1; }
mkdir -p build
# NOLOADADDR=ON : Box64 se lie par défaut à une adresse FIXE (0x34800000),
# en plein dans la région « system managed » de shadPS4 (0x400000 → 32 Gio),
# que celui-ci réserve d'un mmap(MAP_FIXED) — il écrasait le code de Box64.
# En PIE, le chargeur le pose tout en haut, hors des plages du jeu (le x86,
# lui, se place par BOX64_LOAD_ADDR : voir arm64-guest.sh et GlibcGuest).
cmake -S . -B build -G Ninja -DARM64=1 -DARM_DYNAREC=ON -DNOLOADADDR=ON \
    -DCMAKE_BUILD_TYPE=RelWithDebInfo -DCMAKE_C_COMPILER=aarch64-linux-gnu-gcc \
    > build/configure.log 2>&1 || { tail -20 build/configure.log; exit 1; }
cmake --build build --parallel "$JOBS"
# readelf du compilateur croisé : `file` n'est pas dans l'image.
aarch64-linux-gnu-readelf -h build/box64 | grep -q "AArch64" \
    || { echo "✗ build/box64 n'est pas un aarch64" >&2; aarch64-linux-gnu-readelf -h build/box64 | grep Machine; exit 1; }
aarch64-linux-gnu-readelf -h build/box64 | grep -q "Type:.*DYN" \
    || { echo "✗ build/box64 n'est pas un PIE : il se ferait écraser par la réservation de shadPS4" >&2; exit 1; }
rm -rf /tmp/box64-out && mkdir -p /tmp/box64-out
cp build/box64 /tmp/box64-out/box64
cp -r x64lib /tmp/box64-out/x64lib
git rev-parse --short HEAD > /tmp/box64-out/COMMIT
EOF

docker cp "$CONTAINER:/tmp/box64-out/." "$OUT/"
echo "→ $OUT/box64 ($(du -h "$OUT/box64" | cut -f1), commit $(cat "$OUT/COMMIT")) + x64lib/ ($(ls "$OUT/x64lib" | wc -l) fichiers)"
