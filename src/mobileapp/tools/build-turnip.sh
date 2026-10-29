#!/usr/bin/env bash
# Turnip — le pilote Vulkan libre des GPU Adreno — compilé pour ANDROID.
#
#   tools/build-turnip.sh          → build/turnip/libvulkan_freedreno_kgsl.so (aarch64)
#
# POURQUOI LE COMPILER : Debian le livre (mesa-vulkan-drivers, arm64), mais pour
# le noyau d'un Linux de bureau — le backend `msm`, par /dev/dri. Android
# n'expose pas le GPU d'un Snapdragon par DRM : il passe par KGSL, le pilote
# noyau de Qualcomm (/dev/kgsl-3d0), celui qu'ouvre le pilote propriétaire dans
# chaque application. Le Turnip de Debian ne porte aucune trace de
# /dev/kgsl-3d0 (mesuré le 2026-10-08) ; celui-ci est compilé avec
# `-Dfreedreno-kmds=kgsl`, comme les builds de Winlator.
#
# LA CIBLE : le Galaxy S25 — Snapdragon 8 Elite, Adreno 830. Mesa 26.1 le
# déclare (« Adreno (TM) 830 », puce 0x44050001, annotée « KGSL » dans
# src/freedreno/common/freedreno_devices.py).
#
# 26.2.5 DEPUIS LE 2026-10-19 (#388), et non plus la version du runtime glibc
# (le Mesa 26.1.6 de Debian forky). Le support de l'a8xx n'est arrivé qu'avec
# Mesa 26.1, et 26.2 en corrige des défauts : couleur de bordure lue dans la
# mémoire des échantillonneurs, registres de tranches, synchronisation des
# requêtes (26.2.3), événement Vulkan mal initialisé (26.1.7). La règle d'avant
# — la version du runtime, pour ne pas mêler deux Mesa dans un processus quand
# Zink se charge depuis son libgallium — ne protège plus rien : Zink ne démarre
# pas sur notre écran X (voir GlibcGuest, la Xbox), et Turnip est autonome.
#
# Compilation CROISÉE, sur l'hôte x86_64, dans l'image du runtime : rien
# d'arm64 n'est exécuté (pas de qemu), et les en-têtes viennent des paquets
# :arm64 de la même Debian que le runtime.
set -euo pipefail

if [ "${1:-}" != "--inner" ]; then
    HERE=$(cd "$(dirname "$0")/.." && pwd)                 # src/mobileapp
    RUNTIME_IMAGE=$(sed -n 's/^RUNTIME_IMAGE="\(.*\)"$/\1/p' "${HERE}/tools/assemble-glibc.sh" | head -1)
    [ -n "$RUNTIME_IMAGE" ] || { echo "✗ RUNTIME_IMAGE introuvable dans assemble-glibc.sh" >&2; exit 1; }
    OUT="${HERE}/build/turnip"
    mkdir -p "$OUT"
    docker run --rm -e MESA_VERSION="${MESA_VERSION:-26.2.5}" \
        -v "$OUT":/out -v "$(readlink -f "$0")":/build-turnip.sh:ro \
        "$RUNTIME_IMAGE" bash /build-turnip.sh --inner
    echo "→ ${OUT}/libvulkan_freedreno_kgsl.so"
    exit 0
fi

export DEBIAN_FRONTEND=noninteractive
dpkg --add-architecture arm64
apt-get update -qq > /dev/null
apt-get install -y -qq --no-install-recommends \
    build-essential crossbuild-essential-arm64 meson ninja-build pkg-config \
    python3-mako python3-yaml python3-packaging bison flex glslang-tools \
    curl ca-certificates xz-utils file \
    libdrm-dev:arm64 zlib1g-dev:arm64 libzstd-dev:arm64 libexpat1-dev:arm64 \
    libx11-dev:arm64 libx11-xcb-dev:arm64 libxext-dev:arm64 libxfixes-dev:arm64 \
    libxrandr-dev:arm64 libxshmfence-dev:arm64 libxcb1-dev:arm64 libxcb-dri3-dev:arm64 \
    libxcb-present-dev:arm64 libxcb-shm0-dev:arm64 libxcb-randr0-dev:arm64 \
    libxcb-sync-dev:arm64 libxcb-xfixes0-dev:arm64 > /dev/null

cd /tmp
curl -fsSL "https://archive.mesa3d.org/mesa-${MESA_VERSION}.tar.xz" | tar -xJ
cd "mesa-${MESA_VERSION}"
cat > /tmp/aarch64.ini <<'EOF'
[binaries]
c = 'aarch64-linux-gnu-gcc'
cpp = 'aarch64-linux-gnu-g++'
ar = 'aarch64-linux-gnu-ar'
strip = 'aarch64-linux-gnu-strip'
pkg-config = 'pkg-config'

[properties]
# Les .pc des paquets :arm64 — forky n'a pas d'enveloppe
# aarch64-linux-gnu-pkg-config (mesuré : « Pkg-config for machine host machine
# not found »).
pkg_config_libdir = ['/usr/lib/aarch64-linux-gnu/pkgconfig', '/usr/share/pkgconfig']

[host_machine]
system = 'linux'
cpu_family = 'aarch64'
cpu = 'aarch64'
endian = 'little'
EOF
# Le SEUL pilote Vulkan d'Adreno, par KGSL, présenté sur X11 (l'écran de
# l'invité est un Xvfb). Ni OpenGL, ni LLVM, ni Wayland : le runtime porte déjà
# Zink (dans son libgallium) pour l'OpenGL par-dessus Vulkan.
meson setup build --cross-file /tmp/aarch64.ini \
    -Dbuildtype=release -Db_ndebug=true \
    -Dplatforms=x11 \
    -Dvulkan-drivers=freedreno -Dfreedreno-kmds=kgsl \
    -Dgallium-drivers= -Dopengl=false -Dglx=disabled -Degl=disabled -Dgbm=disabled \
    -Dgles1=disabled -Dgles2=disabled -Dllvm=disabled \
    -Dvideo-codecs= -Dvulkan-layers= -Dtools= > /tmp/meson.log 2>&1 \
    || { tail -40 /tmp/meson.log; exit 1; }
ninja -C build src/freedreno/vulkan/libvulkan_freedreno.so > /tmp/ninja.log 2>&1 \
    || { tail -40 /tmp/ninja.log; exit 1; }

lib=build/src/freedreno/vulkan/libvulkan_freedreno.so
aarch64-linux-gnu-strip --strip-unneeded "$lib"
file "$lib" | grep -q "ARM aarch64" || { echo "✗ $lib n'est pas aarch64" >&2; exit 1; }
# Le témoin : la voie KGSL est-elle bien compilée ?
grep -a -q "/dev/kgsl-3d0" "$lib" || { echo "✗ aucune trace de /dev/kgsl-3d0 dans $lib" >&2; exit 1; }
cp "$lib" /out/libvulkan_freedreno_kgsl.so
echo "$MESA_VERSION" > /out/MESA_VERSION
echo "===== Turnip ${MESA_VERSION} (KGSL) : $(stat -c %s /out/libvulkan_freedreno_kgsl.so) o"
aarch64-linux-gnu-readelf -d /out/libvulkan_freedreno_kgsl.so | grep NEEDED
chown -R "$(stat -c '%u:%g' /out)" /out
