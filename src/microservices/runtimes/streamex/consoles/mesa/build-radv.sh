#!/bin/bash
# build-radv.sh : le libvulkan_radeon.so que l'étape « mesa » du Dockerfile d'une
# console vient d'installer (mesa-vulkan-drivers de trixie-backports),
# reconstruit de SON paquet source avec la configuration de Debian, plus nos
# correctifs. Seule l'étape « radv » de ces Dockerfile le lance, au-dessus de
# l'étape « mesa ».
#
#   build-radv.sh <dossier de sortie> <correctif>…
#
# PARTAGÉ PAR LES CONSOLES QUI CORRIGENT RADV (2026-11-11) : la PS5 (ses deux
# correctifs, consoles/ps5/Dockerfile) et la Switch 2 (le correctif des mesh
# shaders, consoles/switch2/Dockerfile). Chacune passe SA liste ; ceux que
# plusieurs consoles prennent vivent ici, à côté du script, ceux d'une seule dans
# le dossier mesa/ de sa console. L'en-tête de chaque correctif dit d'où il
# vient, sur quoi il est validé, quand le retirer et ce qui part avec lui.
#
# Sortie :
#   - libvulkan_radeon.so, corrigé et passé au strip de Debian ;
#   - mesa-version, la version Debian construite ;
#   - libvulkan_radeon.so.sha256, son empreinte au chemin où l'étape finale le
#     pose, pour sha256sum --check ;
#   - les correctifs appliqués, recopiés tels quels (*.patch), pour que l'image
#     les garde à côté de la bibliothèque.
# L'étape finale vérifie la version et l'empreinte.
#
# Dans l'ordre :
#   1. la version que l'étape « mesa » a installée. La bibliothèque du paquet est
#      mise de côté AVANT toute autre installation : c'est la référence du témoin ;
#   2. le deb-src de trixie-backports ;
#   3. les dépendances de construction MINIMALES, prises dans trixie comme chez
#      le buildd (Installed-Build-Depends du .buildinfo). C'est un sous-ensemble
#      du Build-Depends, pour que la détection automatique de meson voie ce
#      qu'elle voit chez Debian ;
#   4. le paquet source EXACT, apt-get source mesa=<version>, dont apt contrôle
#      les empreintes par ses index signés. dpkg-source applique les correctifs
#      de Debian, et déballe la source là où le buildd la déballe : les chemins
#      entrent dans le binaire (Build-Path du .buildinfo) ;
#   5. VERSION, les drapeaux et LLVM, tels que debian/rules les donne ;
#   6. meson avec les options de debian/rules qui atteignent libvulkan_radeon.so,
#      ninja sur cette seule cible, puis le strip de dh_strip ;
#   7. LE TÉMOIN D'ABORD. Sans correctif, la bibliothèque doit avoir exactement
#      les sections chargées de celle du paquet : code, données, symboles
#      (compare-so.py). Les options de meson ne sont pas littéralement celles du
#      buildd (les autres pilotes en moins) : l'équivalence se prouve ici, à
#      chaque construction ;
#   8. les correctifs, dans l'ordre donné, chacun sans flou (--fuzz=0), puis une
#      seule reconstruction incrémentale. Le résultat doit avoir le SONAME, les
#      NEEDED et les symboles exportés du paquet, et un build-id différent de
#      celui du témoin.
#
# Éprouvé le 2026-11-10 sur 26.1.6-1~bpo13+1. Avec les en-têtes que trixie avait
# au passage du buildd, le témoin a le build-id même du paquet. Avec ceux de
# trixie mis à jour depuis, seul le débogage diffère. D'où la comparaison des
# sections chargées, et pas des fichiers entiers.
set -euo pipefail
OUT=${1:?dossier de sortie}
shift
[ $# -gt 0 ] || { echo "build-radv.sh : au moins un correctif" >&2; exit 1; }
PATCHES=()
for patch in "$@"; do
    PATCHES+=("$(readlink -f "$patch")")
done
HERE=$(cd "$(dirname "$0")" && pwd)
JOBS=${JOBS:-$(nproc)}
ARCH=x86_64-linux-gnu
INSTALLED=/usr/lib/$ARCH/libvulkan_radeon.so
REFERENCE=/build/reference.so
LOG=$OUT/log
export DEBIAN_FRONTEND=noninteractive
mkdir -p "$LOG" /build/src /build/reproducible-path

# 1. La version de l'étape « mesa », celle que l'étape finale tient aussi. La
# version source diffère de la version binaire après une reconstruction binNMU
# (« +b1 ») : apt-get source veut la première ; VERSION, la seconde, parce que le
# buildd ajoute l'entrée binNMU au changelog avant de lancer debian/rules.
MESA_VERSION=$(dpkg-query -W -f='${Version}' mesa-vulkan-drivers)
SOURCE_VERSION=$(dpkg-query -W -f='${source:Version}' mesa-vulkan-drivers)
UPSTREAM=${SOURCE_VERSION#*:}
UPSTREAM=${UPSTREAM%%-*}
SOURCE=/build/reproducible-path/mesa-$UPSTREAM
cp "$INSTALLED" "$REFERENCE"
echo "$MESA_VERSION" > "$OUT/mesa-version"
echo "== mesa $MESA_VERSION (source $SOURCE_VERSION)"

# 2. Le deb-src, à côté de la ligne de l'étape « mesa ». Error-Mode=any : sans
# lui, apt-get update rend 0 même quand un index ne se télécharge pas.
echo "deb-src http://deb.debian.org/debian trixie-backports main" >> /etc/apt/sources.list.d/backports.list
apt-get update -qq -o APT::Update::Error-Mode=any > "$LOG/apt-update.log" 2>&1 || {
    tail "$LOG/apt-update.log"
    echo "apt-get update a échoué : deb.debian.org doit être joignable pendant la construction" >&2
    exit 1
}

# 3. Toutes dans l'environnement du buildd. dpkg-dev sert aussi à apt-get
# source et à dpkg-buildflags ; xz-utils et patch, à déballer et à corriger.
# LLVM et valgrind sont nécessaires : sans eux, le code n'est plus celui du
# paquet.
#
# Ce que l'étape « mesa » a pris dans les backports à côté de Mesa (son -t vaut
# aussi pour les dépendances : libelf1t64 0.196 le 2026-11-10) redescend ici à
# sa version de trixie, celle du buildd : libelf-dev de trixie l'exige. Seule
# cette étape le voit, l'étape finale garde la version de l'étape « mesa ».
DEPENDENCIES="dpkg-dev gcc g++ xz-utils patch
    meson ninja-build pkgconf python3-mako python3-yaml
    glslang-tools spirv-tools llvm-19-dev valgrind
    libdrm-dev libelf-dev libexpat1-dev zlib1g-dev libzstd-dev libdisplay-info-dev
    libx11-dev libx11-xcb-dev libxcb-dri3-dev libxcb-present-dev libxcb-randr0-dev
    libxcb-shm0-dev libxcb-sync-dev libxrandr-dev libxshmfence-dev
    libwayland-dev wayland-protocols"
FROM_TRIXIE=$(dpkg-query -W -f='${Package} ${Version} ${source:Package}\n' | awk '$2 ~ /~bpo/ && $3 != "mesa" {print $1 "/trixie"}')
echo "== redescendus à trixie : ${FROM_TRIXIE:-aucun}"
apt-get install -y -qq --no-install-recommends --allow-downgrades $DEPENDENCIES $FROM_TRIXIE > "$LOG/apt-install.log" 2>&1 \
    || { tail -30 "$LOG/apt-install.log"; exit 1; }

# 4. Le paquet source. --no-check : la signature du .dsc n'est pas relue, apt a
# déjà contrôlé ses empreintes.
cd /build/src
apt-get source --download-only mesa="$SOURCE_VERSION" > "$LOG/apt-source.log" 2>&1 || {
    tail "$LOG/apt-source.log"
    echo "la source de mesa $SOURCE_VERSION, la version de l'étape « mesa », n'est plus dans trixie-backports :" >&2
    echo "les backports ont avancé depuis que l'étape « mesa » est en cache. Reconstruire sans cache :" >&2
    echo "docker compose build --no-cache streamex-<console>" >&2
    exit 1
}
dpkg-source --no-check -x "mesa_${SOURCE_VERSION#*:}.dsc" "$SOURCE" > "$LOG/dpkg-source.log"

# 5. debian/rules : VERSION (override_dh_auto_configure, voir 1), les drapeaux
# (DEB_BUILD_MAINT_OPTIONS et -Wall), LLVM_VERSION en tête du PATH.
cd "$SOURCE"

if ! grep -q '^LLVM_VERSION = 19$' debian/rules; then
    echo "debian/rules ne demande plus LLVM 19 : la liste des dépendances est à revoir" >&2
    exit 1
fi

echo "$MESA_VERSION" > VERSION
eval "$(DEB_BUILD_MAINT_OPTIONS=optimize=-lto DEB_CFLAGS_MAINT_APPEND=-Wall \
    DEB_CXXFLAGS_MAINT_APPEND=-Wall dpkg-buildflags --export=sh)"
export PATH=/usr/lib/llvm-19/bin:$PATH

# 6. Les arguments de dh_auto_configure (meson), puis ceux de debian/rules qui
# atteignent libvulkan_radeon.so. Retirés : les autres pilotes et couches,
# GL/GLX/EGL/GBM/glvnd, rusticl, VA, lm-sensors, drm-shim, les tests. Aucun
# n'entre dans la bibliothèque : le témoin le vérifie.
mkdir build
cd build
LC_ALL=C.UTF-8 meson setup .. --wrap-mode=nodownload --buildtype=plain --prefix=/usr \
    --sysconfdir=/etc --localstatedir=/var --libdir=lib/$ARCH -Dpython.bytecompile=-1 \
    -Ddri-drivers-path=/usr/lib/$ARCH/dri \
    -Dplatforms=x11,wayland -Dvulkan-drivers=amd -Dvulkan-layers= \
    -Dvulkan-manifest-per-architecture=false -Db_ndebug=true -Dvideo-codecs=all \
    -Dllvm=enabled -Dvalgrind=enabled \
    -Dgallium-drivers= -Dopengl=false -Dgles1=disabled -Dgles2=disabled -Degl=disabled \
    -Dglx=disabled -Dgbm=disabled -Dglvnd=disabled -Dgallium-va=disabled -Dlmsensors=disabled \
    -Dteflon=false -Dtools= -Dbuild-tests=false > "$LOG/meson-setup.log" 2>&1 \
    || { tail -40 "$LOG/meson-setup.log"; exit 1; }

build_library() {   # build_library <journal> <sortie>
    LC_ALL=C.UTF-8 ninja -j "$JOBS" src/amd/vulkan/libvulkan_radeon.so > "$LOG/ninja-$1.log" 2>&1 \
        || { grep -B2 -A12 'FAILED\|error:' "$LOG/ninja-$1.log" | head -60; exit 1; }
    # Le strip de dh_strip pour une bibliothèque partagée (Debian::Debhelper::Dh_Lib).
    strip --remove-section=.comment --remove-section=.note --strip-unneeded \
        -o "$2" src/amd/vulkan/libvulkan_radeon.so
}

library_shape() {   # library_shape <so> : SONAME, NEEDED, symboles dynamiques définis
    readelf -dW "$1" | awk '/SONAME|NEEDED/ {print $2, $NF}'
    nm -D --defined-only "$1" | awk '{print $2, $3}' | sort
}

build_id() {
    readelf -n "$1" | awk '/Build ID/ {print $3}'
}

# 7. Le témoin.
echo "== témoin, sans correctif"
build_library witness /build/witness.so

if ! python3 -I "$HERE/compare-so.py" "$REFERENCE" /build/witness.so; then
    echo "LE TÉMOIN N'EST PAS LE PILOTE DU PAQUET $MESA_VERSION. Deux causes possibles :" >&2
    echo "  - la chaîne de construction de trixie a changé depuis le buildd (gcc-14, binutils, llvm-19," >&2
    echo "    spirv-tools, valgrind, glslang, libdrm) : comparer avec les Installed-Build-Depends du" >&2
    echo "    .buildinfo de cette version. Le prochain envoi de mesa dans les backports sera construit avec elle ;" >&2
    echo "  - notre configuration a dérivé de debian/rules : relire ses options de meson." >&2
    exit 1
fi

# 8. Les correctifs, dans l'ordre donné. Chacun dit, en tête, ce qu'il faut faire
# quand il ne s'applique plus : le retirer si Mesa a corrigé le défaut, le porter
# sinon.
cd "$SOURCE"

for patch in "${PATCHES[@]}"; do
    echo "== correctif $(basename "$patch")"

    if ! patch -p1 --fuzz=0 --forward --no-backup-if-mismatch < "$patch"; then
        echo "LE CORRECTIF $(basename "$patch") NE S'APPLIQUE PAS À MESA $MESA_VERSION : ce qu'il change, ou son" >&2
        echo "contexte, a bougé dans cette version. Lire son en-tête (consoles/mesa/ ou consoles/<console>/mesa/) :" >&2
        echo "  - si Mesa a corrigé le défaut, sous quelque forme que ce soit, retirer le correctif selon sa liste ;" >&2
        echo "  - sinon, le porter à cette version, puis l'éprouver sur le GPU avant de livrer l'image." >&2
        exit 1
    fi

    cp "$patch" "$OUT/"
done

cd build
build_library patched "$OUT/libvulkan_radeon.so"

if ! diff <(library_shape "$REFERENCE") <(library_shape "$OUT/libvulkan_radeon.so"); then
    echo "le pilote corrigé n'a pas la forme du paquet (SONAME, NEEDED ou symboles exportés)" >&2
    exit 1
fi

if [ "$(build_id "$OUT/libvulkan_radeon.so")" = "$(build_id /build/witness.so)" ]; then
    echo "le pilote corrigé a le build-id du témoin : les correctifs n'ont rien changé" >&2
    exit 1
fi

printf '%s  %s\n' "$(sha256sum < "$OUT/libvulkan_radeon.so" | cut -d' ' -f1)" "$INSTALLED" > "$OUT/libvulkan_radeon.so.sha256"
echo "== paquet $(build_id "$REFERENCE"), témoin $(build_id /build/witness.so), corrigé $(build_id "$OUT/libvulkan_radeon.so")"
