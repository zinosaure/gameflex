#!/usr/bin/env bash
# Assemble le bundle aarch64 de la PS4 du téléphone, « Box64 + notre shadPS4
# x86_64 » : downloads/shadps4-aarch64.zip — celui que GlibcGuest télécharge et
# que arm64-guest.sh lance —, dans la forme de nos invités (usr/bin, usr/lib,
# usr/VERSION), plus usr/x86_64/ : le binaire x86_64, les quelques bibliothèques
# x86_64 qu'il charge (prises du trixie où il a été construit), et x64lib, celles
# que Box64 fournit lui-même. Il remplace le shadPS4 arm64 de Bachata S4
# (chantier 17 : celui-ci ne démarrait que dans leur application).
#
#   tools/box64/bundle.sh <sortie de box64/build.sh> <shadps4 x86_64> [downloads]
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
BOX=$(readlink -f "${1:?la sortie de tools/box64/build.sh (box64, x64lib, COMMIT)}")
SHAD=$(readlink -f "${2:?le binaire x86_64 : shadps4 (tools/shadps4/build.sh) ou kyty_emulator (tools/kyty/build.sh)}")
DL=${3:-$ROOT/src/webapp/src/public/downloads}
CONTAINER=${SHADPS4_BUILD_CONTAINER:-gameflex-shadps4-build}   # un trixie x86_64 : ses bibliothèques
CONSOLES="$ROOT/src/microservices/runtimes/streamex/consoles"
# L'INVITÉ SE RECONNAÎT AU BINAIRE : le bundle porte son nom (<invité>-aarch64.zip,
# celui que GlibcGuest télécharge) et la version que le serveur emploie.
X86=$(basename "$SHAD")
case "$X86" in
    shadps4)       GUEST=shadps4; VERSION=$(sed -n 's/^ARG SHADPS4_VERSION=//p' "$CONSOLES/ps4/Dockerfile") ;;
    kyty_emulator) GUEST=kyty;    VERSION=$(sed -n 's/^ARG KYTY_RELEASE=//p' "$CONSOLES/ps5/Dockerfile") ;;
    *) echo "✗ binaire x86 inconnu : $X86 (shadps4 ou kyty_emulator)" >&2; exit 1 ;;
esac
# usr/VERSION : la version que l'APK attend pour cet invité (#519) — à monter
# dans GUESTS AVANT d'assembler, sinon le téléphone refuse le paquet.
STAMP=$("$ROOT/tools/phone-guests.sh" "$GUEST" | cut -d' ' -f3)

W=$(mktemp -d)
trap 'rm -rf "$W"' EXIT
mkdir -p "$W/usr/bin" "$W/usr/lib" "$W/usr/x86_64/bin" "$W/usr/x86_64/lib" "$W/usr/x86_64/box64lib"
cp "$BOX/box64" "$W/usr/bin/box64" && chmod 755 "$W/usr/bin/box64"
cp -r "$BOX/x64lib/." "$W/usr/x86_64/box64lib/"
cp "$SHAD" "$W/usr/x86_64/bin/$X86" && chmod 755 "$W/usr/x86_64/bin/$X86"
# Ce que shadPS4 charge (readelf -d) et que Box64 n'enveloppe pas (libstdc++),
# ou peut ne pas envelopper (libuuid, libudev) : les copies x86_64 de trixie.
# Box64 préfère ses propres enveloppes quand il en a ; le reste est chargé d'ici.
for lib in libstdc++.so.6 libgcc_s.so.1 libuuid.so.1 libudev.so.1; do
    docker cp -L "$CONTAINER:/usr/lib/x86_64-linux-gnu/$lib" "$W/usr/x86_64/lib/$lib"
done
# Les enveloppes de Box64 (libuuid, libudev, et libXtst que SDL3 charge à la
# demande) appellent les bibliothèques AARCH64 natives — que le runtime du
# téléphone n'a pas (sa clôture les ajoute par invité). Prises des paquets
# arm64 de trixie, sans rien exécuter.
docker run --rm -v "$W/usr/lib:/out" debian:trixie bash -c '
    set -e; export DEBIAN_FRONTEND=noninteractive
    dpkg --add-architecture arm64 && apt-get update -qq > /dev/null
    cd /tmp && apt-get download -qq libuuid1:arm64 libudev1:arm64 libxtst6:arm64 > /dev/null
    for deb in *.deb; do dpkg-deb -x "$deb" /tmp/x; done
    cp -L /tmp/x/usr/lib/aarch64-linux-gnu/libuuid.so.1 /tmp/x/usr/lib/aarch64-linux-gnu/libudev.so.1 \
          /tmp/x/usr/lib/aarch64-linux-gnu/libXtst.so.6 /out/
    chown "$(stat -c %u:%g /out)" /out/*' > /dev/null
if [ "$GUEST" = shadps4 ]; then
    # L'OUTIL de la PS4 : un jeu y est un .pkg, que le téléphone déballe lui-même
    # avec pkgextract — l'adresse du Dockerfile de la PS4, dans sa variante
    # aarch64 (la même version, publiée pour les deux), comme assemble-appimage.sh.
    TOOL=$(sed -n 's/^ARG PKGEXTRACT_URL=//p' "$CONSOLES/ps4/Dockerfile")
    TOOL=${TOOL/x86_64-unknown-linux-gnu/aarch64-unknown-linux-gnu}
    curl -fsSL --retry 3 -o "$W/tool.zip" "$TOOL"
    python3 - "$W/tool.zip" "$W/usr/bin/pkgextract" <<'PY'
import sys, zipfile
archive, out = zipfile.ZipFile(sys.argv[1]), sys.argv[2]
name = next(n for n in archive.namelist() if n.endswith("pkgextract"))
open(out, "wb").write(archive.read(name))
PY
    chmod 755 "$W/usr/bin/pkgextract"
    rm -f "$W/tool.zip"
fi
echo "$STAMP" > "$W/usr/VERSION"

mkdir -p "$DL"
OUT="$DL/${GUEST}-aarch64.zip"
rm -f "$OUT"
# python3 plutôt que zip : présent sur l'hôte, et il garde les droits d'exécution.
python3 - "$W" "$OUT" <<'PY'
import os, sys, zipfile
root, out = sys.argv[1], sys.argv[2]
with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as archive:
    for directory, _, files in os.walk(os.path.join(root, "usr")):
        for name in files:
            path = os.path.join(directory, name)
            archive.write(path, os.path.relpath(path, root))
PY
echo "→ $OUT ($(du -h "$OUT" | cut -f1)) : $STAMP (box64 $(cat "$BOX/COMMIT"), $GUEST $VERSION)"
