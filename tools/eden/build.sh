#!/usr/bin/env bash
# Construit l'Eden aarch64 du téléphone : la nocturne du serveur, plus nos
# patchs (0001-…), sous émulation — et le livre en AppDir archivé, que
# src/mobileapp/tools/assemble-appimage.sh prend tel quel.
#
#   tools/eden/build.sh    → src/webapp/src/public/downloads/eden-<commit>-gameflex-aarch64.AppDir.tar.gz
#
# LA MÊME CONSTRUCTION QUE L'ÉDITEUR, à nos patchs près. Sa CI (dépôt
# mirror/Workflow de git.eden-emu.dev) bâtit l'AppImage aarch64 dans l'image
# ghcr.io/pkgforge-dev/archlinux, avec clang, son profil PGO
# (github.com/Eden-CI/PGO) et sans LTO. On reprend l'image, les paquets
# (.ci/deps/arch.sh), le profil et les options de cmake (.ci/build/*.sh). Seul
# le binaire `eden` change : il remplace celui de l'AppImage officielle, dont
# les bibliothèques restent — et la fin du script vérifie que chacune de ses
# dépendances y est.
#
# L'AARCH64 SE CONSTRUIT ÉMULÉ, sans rien changer à l'hôte (même voie que notre
# Citron du 2026-10-19) : l'image arm64 est EXPORTÉE — Docker ne l'exécute
# jamais —, puis tournée sous proot + qemu-user, sans binfmt ni privilège. En
# priorité basse (nice 19), pour ne pas voler les parties du serveur. Le volume
# eden-arm64 garde le système, la source et les objets : une seconde passe ne
# recompile que ce qui a changé.
set -euo pipefail

COMMIT=de7e6e01d43e8cf2ec193d74fa053b0d7949955f
NIGHTLY=https://nightly.eden-emu.dev/v1792353251.de7e6e01d4/Eden-Linux-de7e6e01d4-aarch64-clang-pgo.AppImage
JOBS=${JOBS:-$(( $(nproc) > 4 ? $(nproc) - 2 : 2 ))}
HERE=$(cd "$(dirname "$0")" && pwd)
ROOT=$(cd "$HERE/../.." && pwd)
DL="$ROOT/src/webapp/src/public/downloads"
OUT="eden-${COMMIT:0:10}-gameflex-aarch64.AppDir.tar.gz"

docker volume create eden-arm64 > /dev/null
ROOTFS=$(mktemp -d)/rootfs-arm64.tar
docker pull -q --platform linux/arm64 ghcr.io/pkgforge-dev/archlinux:latest > /dev/null
id=$(docker create --platform linux/arm64 ghcr.io/pkgforge-dev/archlinux:latest true)
docker export "$id" > "$ROOTFS"
docker rm "$id" > /dev/null

docker run --rm -e JOBS="$JOBS" -e COMMIT="$COMMIT" -e NIGHTLY="$NIGHTLY" -e OUT="$OUT" \
    -v eden-arm64:/work -v "$ROOTFS":/rootfs-arm64.tar:ro -v "$HERE":/patches:ro -v "$DL":/dl \
    debian:forky-slim bash -c '
    set -e
    export DEBIAN_FRONTEND=noninteractive PROOT_NO_SECCOMP=1
    apt-get update -qq > /dev/null && apt-get install -y -qq qemu-user proot curl ca-certificates binutils > /dev/null
    R=/work/rootfs
    if [ ! -x $R/usr/bin/pacman ]; then
        mkdir -p $R && tar -xf /rootfs-arm64.tar -C $R
        # proot ne sait pas dire la place libre des montages, et le bac à
        # sable de pacman (landlock) ne passe pas sous qemu-user.
        sed -i "s/^CheckSpace/#CheckSpace/; s/^#DisableSandbox/DisableSandbox/" $R/etc/pacman.conf
        grep -q "^DisableSandbox" $R/etc/pacman.conf || sed -i "/^\[options\]/a DisableSandbox" $R/etc/pacman.conf
    fi
    cp /etc/resolv.conf $R/etc/resolv.conf
    # Le verrou qu une passe précédente aurait laissé (voir plus bas).
    rm -f $R/var/lib/pacman/db.lck
    P="nice -n 19 proot -q qemu-aarch64 -0 -r $R -b /dev -b /proc -b /sys -b /patches -w /root"

    # Les paquets de .ci/deps/arch.sh qui servent à compiler (ni les thèmes
    # Qt, ni Xvfb, ni zsync) — en un seul appel.
    $P /bin/bash -c "pacman -Syu --noconfirm --needed --overwrite \"*\" base-devel boost boost-libs catch2 clang \
        cmake curl enet ffnvcodec-headers fmt git glslang jq libva libvdpau libvpx libxcursor libxss lld llvm \
        mbedtls nasm ninja nlohmann-json patchelf pulseaudio python-requests qt6-base qt6-tools qt6-charts \
        spirv-headers spirv-tools unzip vulkan-headers vulkan-utility-libraries wget xcb-util-cursor \
        xcb-util-image xcb-util-renderutil xcb-util-wm zip 2>&1 | tail -3" || true
    # proot meurt parfois dans les crochets de fin de pacman (systemd-tmpfiles :
    # « compare_paths2: Assertion `length2 > 0` failed »), la transaction déjà
    # faite : le verrou reste. On le retire, et on vérifie ce qui est là.
    rm -f $R/var/lib/pacman/db.lck
    $P /bin/bash -c "pacman -T clang lld cmake ninja git qt6-base qt6-tools qt6-charts boost fmt enet         glslang spirv-tools vulkan-headers nlohmann-json mbedtls > /dev/null" \
        || { echo "✗ des paquets manquent après pacman" >&2; exit 1; }

    # La source au commit de la nocturne (git : la version d Eden se lit dans
    # son historique), puis nos patchs, dans l ordre de leur numéro.
    $P /bin/bash -c "set -e
        if [ ! -d /root/eden/.git ]; then
            git init -q /root/eden && cd /root/eden
            git remote add origin https://git.eden-emu.dev/eden-emu/eden.git
            git fetch -q --depth 1 origin $COMMIT && git checkout -q FETCH_HEAD
        fi
        cd /root/eden && git reset -q --hard $COMMIT && git clean -qfdx -e build -e eden.profdata
        for p in /patches/0*.patch; do git apply \"\$p\" && echo \"== \$(basename \"\$p\")\"; done
        [ -s eden.profdata ] || curl -fsSL -o eden.profdata https://github.com/Eden-CI/PGO/releases/latest/download/eden.profdata"

    FLAGS="-march=armv8-a -mtune=generic -O3 -pipe -fprofile-use=/root/eden/eden.profdata -Wno-backend-plugin -Wno-profile-instr-unprofiled -Wno-profile-instr-out-of-date"
    $P /bin/bash -c "cd /root/eden && cmake -S . -B build -G Ninja \
        -DCMAKE_C_COMPILER=clang -DCMAKE_CXX_COMPILER=clang++ -DCMAKE_EXE_LINKER_FLAGS=-fuse-ld=lld \
        -DCMAKE_C_FLAGS=\"$FLAGS\" -DCMAKE_CXX_FLAGS=\"$FLAGS\" \
        -DBUILD_TESTING=OFF -DCMAKE_BUILD_TYPE=Release -DYUZU_USE_BUNDLED_QT=OFF -DYUZU_USE_QT_MULTIMEDIA=OFF \
        -DYUZU_USE_QT_WEB_ENGINE=OFF -DENABLE_QT_TRANSLATION=ON -DENABLE_UPDATE_CHECKER=OFF -DUSE_CCACHE=OFF \
        -DUSE_DISCORD_PRESENCE=ON -DENABLE_LTO=OFF -DYUZU_USE_BUNDLED_SIRIT=ON -DYUZU_USE_BUNDLED_FFMPEG=ON \
        -DYUZU_DISABLE_LLVM=ON -DYUZU_STATIC_BUILD=OFF -DCMAKE_INSTALL_PREFIX=/usr -DYUZU_CMD=OFF \
        -DYUZU_ROOM_STANDALONE=OFF -DNIGHTLY_BUILD=ON -DBUILD_ID=aarch64 -DYUZU_USE_BUNDLED_SDL3=ON \
        -DYUZU_USE_BUNDLED_SDL2=ON -DYUZU_USE_BUNDLED_OPENSSL=ON > /root/configure.log 2>&1 || { tail -30 /root/configure.log; exit 1; }
        cmake --build build -j $JOBS > /root/build.log 2>&1 || grep -E -A5 \"error:|FAILED\" /root/build.log | tail -40"
    B=$R/root/eden/build/bin/eden
    [ -x $B ] || { echo "✗ pas de binaire eden : la compilation a échoué" >&2; exit 1; }

    # L AppDir de la nocturne officielle, sans l exécuter (DwarFS lu par
    # l outil statique x86_64 du projet), où notre binaire prend la place du sien.
    cd /work && rm -rf appdir && mkdir appdir
    curl -fsSL -o nightly.AppImage "$NIGHTLY"
    dwurl=$(curl -s https://api.github.com/repos/mhx/dwarfs/releases/latest | grep -oE "https://[^\"]*/dwarfs-universal-[0-9.]+-Linux-x86_64" | head -1)
    curl -fsSL -o dwarfs-universal "$dwurl" && chmod +x dwarfs-universal
    ./dwarfs-universal --tool=dwarfsextract -i nightly.AppImage -o appdir --image-offset=auto
    # Ce que notre binaire réclame et que ni l AppDir ni le binaire officiel
    # (dont la plateforme vient du runtime du téléphone) n apportent.
    needed() { readelf -d "$1" | sed -n "s/.*NEEDED.*\[\(.*\)\]/\1/p"; }
    official=$(needed appdir/shared/bin/eden)
    missing=""
    for lib in $(needed $B); do
        [ -e appdir/shared/lib/$lib ] || echo "$official" | grep -qx "$lib" || missing="$missing $lib"
    done
    [ -z "$missing" ] || { echo "✗ absentes de l AppDir officiel :$missing" >&2; exit 1; }
    cp $B appdir/shared/bin/eden
    tar -czf /dl/$OUT -C appdir .
    ls -la /dl/$OUT'
rm -f "$ROOTFS"
