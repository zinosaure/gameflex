#!/usr/bin/env bash
# Assemble le bundle de l'émulateur de bureau d'une console — tel que son projet
# le publie, complété de ce qu'il suppose du système.
#
#   tools/assemble-appimage.sh <console> [x86_64|aarch64]
#
#   tools/assemble-appimage.sh xbox              → downloads/xemu-x86_64.zip
#   tools/assemble-appimage.sh ps3 aarch64       → downloads/rpcs3-aarch64.zip
#   tools/assemble-appimage.sh wiiu              → downloads/cemu-x86_64.zip
#   tools/assemble-appimage.sh switch aarch64    → downloads/citron-aarch64.zip
#   tools/assemble-appimage.sh switch-eden aarch64 → downloads/eden-aarch64.zip
#   tools/assemble-appimage.sh ps4               → downloads/shadps4-x86_64.zip
#   tools/assemble-appimage.sh ps5               → downloads/kyty-x86_64.zip
#
#   → Xserver.ensureGuest le pose sous guests/<invité> en gardant l'arborescence
#     (RUNPATH $ORIGIN/../lib, qt.conf à côté du binaire), et GlibcGuest lance
#     usr/bin/<binaire> sous le traceur. Le runtime d'abord : la fermeture se
#     calcule CONTRE lui (tools/assemble-glibc.sh <arch>).
#
# LA VERSION EST CELLE DU SERVEUR. En x86_64, le fichier vient de l'adresse que
# déclare `runtimes/streamex/consoles/emulators.sh` — celle que lisent les images
# de console (#145), dont l'en-tête dit déjà : « le téléphone épingle ses
# versions sur celles d'ici ». Il ne le faisait pas : cet assembleur prenait la
# DERNIÈRE release de GitHub, si bien que le RPCS3 de l'appareil (build 19985)
# n'était pas celui du serveur (19749). Le serveur ne tourne pas en aarch64 :
# ces adresses-là sont déclarées ici, à la même version quand l'amont la publie,
# sinon à la plus proche (dit à côté de chacune).
#
# RIEN DE L'ÉMULATEUR N'EST EXÉCUTÉ : l'hôte ne sait pas lancer d'arm64, et
# `--appimage-extract` est une exécution. Une AppImage est un runtime ELF suivi
# d'une image — squashfs (xemu, Cemu, shadPS4) ou DwarFS (RPCS3, Eden) — et
# l'extraction se fait sans jamais la lancer, pour les deux architectures pareil.
# C'est le CONTENU qui décide de l'outil, pas l'en-tête : la table de sections
# du runtime de RPCS3 pointe au-delà de l'image, et son code contient lui-même
# la chaîne « hsqs » (un faux superbloc, mesuré le 2026-09-13).
#
# TROIS MISES EN PAGE, RAMENÉES À UNE SEULE (usr/bin/<binaire> + usr/lib) :
#  - linuxdeploy (xemu, RPCS3, Cemu, shadPS4) : déjà usr/bin + usr/lib ;
#  - sharun (Eden) : shared/bin + shared/lib, la glibc d'Arch comprise ;
#  - archive ordinaire (Kyty) : le binaire à la racine.
#
# Ce qu'un émulateur n'embarque PAS, et qu'aucun Android n'a :
#  - des bibliothèques « de base » de l'hôte (libgmp, xcb-*, dbus, GTK 3 pour
#    Cemu…) → CLÔTURE des NEEDED de tout ce qu'il contient contre le Debian du
#    runtime (lu dans le zip, fichier RUNTIME), et tout ce que ni lui ni le
#    runtime ne fournit rejoint usr/lib ;
#  - un libstdc++ récent : linuxdeploy le laisse hors de usr/lib et le range
#    sous checkrt/, avec un hook qui le préfixe au chemin SI le système est
#    plus vieux. On fait la même comparaison ici : le plus récent des deux —
#    celui de l'invité ou celui du runtime — est le seul gardé, et l'ordre du
#    --library-path (l'invité avant le runtime) fait le reste.
# Et ce qu'il embarque mais ne doit JAMAIS apporter : la PLATEFORME — la libc
# et la pile graphique. Voir « LA PLATEFORME VIENT DU RUNTIME » plus bas.
set -euo pipefail

# L'invité d'une console : son nom (le zip, le dossier sur l'appareil, celui que
# GlibcGuest emploie), le binaire à lancer, et l'adresse aarch64 — « - » quand
# l'amont n'en publie aucune. GlibcGuest porte la même liste des architectures :
# une console sans bundle pour l'ABI de l'appareil n'y est jamais proposée.
guest_of() {
    case "$1" in
        xbox)   echo "xemu xemu https://github.com/xemu-project/xemu/releases/download/v0.8.139/xemu-0.8.139-aarch64.AppImage" ;;
        # La même build que le serveur : 20404 est publiée pour les deux
        # architectures (19749 ne l'était pas, d'où 19751 jusqu'au 2026-10-08).
        ps3)    echo "rpcs3 rpcs3 https://github.com/RPCS3/rpcs3-binaries-linux-arm64/releases/download/build-e8979d31b3a22c8bc9a6eae6bdc31756eac8ee89/rpcs3-v0.0.43-20404-e8979d31_linux_aarch64.AppImage" ;;
        # Cemu ne publie aucune RELEASE aarch64, et la 2.6 du serveur n'a pas
        # de recompilateur AArch64 (seulement PPCRecompilerX64*) : elle
        # n'interpréterait le PowerPC qu'instruction par instruction. Sa CI, si :
        # l'AppImage arm64 que `build-appimage-arm` construit à chaque commit
        # (même script que la release x86_64), le backend AArch64 compris.
        # Commit fced55a du 2026-10-06. Un artefact de CI exige un jeton GitHub,
        # même d'un dépôt public (GITHUB_TOKEN), et EXPIRE (2027-01-04) : passé
        # cette date, épingler l'artefact d'une exécution plus récente.
        wiiu)   echo "cemu Cemu https://api.github.com/repos/cemu-project/Cemu/actions/artifacts/11441387544/zip" ;;
        # LA SWITCH A DEUX ÉMULATEURS (chantier 26), une clé chacun, comme ses
        # deux images au serveur (emulators.sh). CITRON, le premier : sa release
        # du 27 avril, celle que le S25 a jouée. EDEN, le second : sa nocturne
        # officielle, la même que le serveur — seule à lire les jeux du
        # firmware 22 (ZBIC, #433).
        switch) echo "citron citron https://github.com/citron-neo/emulator/releases/download/2026-04-27/citron_nightly-0237a9b88-linux-aarch64.AppImage" ;;
        # NOTRE construction de la même nocturne (tools/eden/) : le HeapTracker
        # de l'Eden d'Android sur Linux arm64, qui garde ses zones mémoire sous
        # le plafond d'Android (#434). Un AppDir archivé, rangé dans downloads/.
        switch-eden) echo "eden eden file:///dl/eden-de7e6e01d4-gameflex-aarch64.AppDir.tar.gz" ;;
        # La PS4 en aarch64 n'est pas une AppImage de l'amont : c'est « Box64 +
        # NOTRE shadPS4 x86_64 », assemblé par tools/box64/bundle.sh (chantier
        # 17) — shadPS4 construit avec un réglage qui ramène sa plage d'adresses
        # sous les 39 bits d'un noyau Android, et Box64 pour exécuter le x86.
        ps4)    echo "shadps4 shadps4 -" ;;
        ps5)    echo "kyty kyty_emulator -" ;;
        *)      echo "" ;;
    esac
}

# UNE EXCEPTION À LA PARITÉ, imposée par la plateforme et non choisie.
#
# shadPS4, depuis la 0.19.0, range ses tampons dans des arènes CLAIRSEMÉES de 4 Gio
# (buffer_cache.h : ARENA_PAGE_BITS = 32, eSparseBinding | eSparseResidency),
# sans réglage pour revenir en arrière. Sous lavapipe — le seul Vulkan de
# l'appareil tant qu'il n'a pas de voie GPU (#200) — leur réservation échoue :
# vk::Result=ErrorOutOfDeviceMemory au premier calcul, puis SIGSEGV. Mesuré sur
# l'AVD le 2026-10-08 avec Sonic Mania Plus, qui joue sa cinématique en 0.18.0.
# Le serveur, qui a un GPU, est passé à la 0.19.0 (0.20.0 le 2026-10-26) ;
# l'émulateur Android reste en 0.18.0.
# Aucun vrai téléphone n'est concerné : il n'y a que l'émulateur en x86_64.
#
# Vide pour une console : l'adresse du serveur (emulators.sh), comme toujours.
phone_x86_64_of() {
    case "$1" in
        ps4) echo "https://github.com/shadps4-emu/shadPS4/releases/download/v.0.18.0/shadps4-linux-sdl-0.18.0.zip" ;;
        # La nocturne amd64 d'Eden du serveur réclame la glibc 2.44, le runtime
        # x86_64 n'a que la 2.43 (mesuré le 2026-10-22) ; l'aarch64, elle, y
        # tient. L'émulateur Android prend donc la 0.2.1, qui y tenait déjà le
        # 2026-10-14 — sans les jeux du firmware 22 (ZBIC).
        switch-eden) echo "https://stable.eden-emu.dev/v0.2.1/Eden-Linux-v0.2.1-amd64-gcc-standard.AppImage" ;;
        *)   echo "" ;;
    esac
}

if [ "${1:-}" != "--inner" ]; then
    console="${1:?usage: assemble-appimage.sh <console> [x86_64|aarch64]}"
    arch="${2:-x86_64}"
    HERE=$(cd "$(dirname "$0")/.." && pwd)                 # src/mobileapp
    CONSOLES="${HERE}/../microservices/runtimes/streamex/consoles"
    # shellcheck source=../../microservices/runtimes/streamex/consoles/emulators.sh
    . "${CONSOLES}/emulators.sh"
    read -r guest binary arm_url <<< "$(guest_of "$console")"
    [ -n "${guest:-}" ] || { echo "✗ « $console » n'a pas d'émulateur de bureau" >&2; exit 2; }
    case "$arch" in
        x86_64)  url=$(phone_x86_64_of "$console"); [ -n "$url" ] || url=$(emulator_url "$console") ;;
        aarch64) url="$arm_url" ;;
        *) echo "✗ architecture inconnue : $arch" >&2; exit 2 ;;
    esac
    [ -n "$url" ] && [ "$url" != "-" ] || { echo "✗ $console : aucun build $arch publié" >&2; exit 2; }
    DL="${HERE}/../webapp/src/public/downloads"
    [ -f "${DL}/glibc-${arch}.zip" ] || { echo "✗ assemble d'abord le runtime : tools/assemble-glibc.sh ${arch}" >&2; exit 1; }
    IMAGE=$(unzip -p "${DL}/glibc-${arch}.zip" RUNTIME 2>/dev/null || true)
    [ -n "$IMAGE" ] || { echo "✗ glibc-${arch}.zip ne dit pas son image (RUNTIME) — réassemble-le" >&2; exit 1; }
    # CE QUI ACCOMPAGNE L'ÉMULATEUR, pris là où le serveur le déclare :
    #  - l'OUTIL de la PS4 : un jeu y est un `.pkg`, déballé par `pkgextract`
    #    avant le lancement — son adresse est l'ARG du Dockerfile de la PS4 ;
    #  - les GABARITS que l'appareil rejoue comme l'entrypoint du serveur : le
    #    `settings.xml` de Cemu (sans lui, son assistant modal de premier
    #    lancement), rangé sous usr/share/gameflex/. Une seule source.
    tool="-"; templates="-"
    case "$console" in
        ps4)  tool=$(sed -n 's/^ARG PKGEXTRACT_URL=//p' "${CONSOLES}/ps4/Dockerfile")
              # La même version, publiée aussi pour aarch64.
              [ "$arch" = aarch64 ] && tool=${tool/x86_64-unknown-linux-gnu/aarch64-unknown-linux-gnu} ;;
        wiiu) templates="settings.xml" ;;
    esac
    # Un artefact de CI ne se télécharge qu'authentifié : le jeton passe au
    # conteneur PAR SON NOM (-e GITHUB_TOKEN), jamais en valeur sur une ligne de
    # commande.
    case "$url" in
        https://api.github.com/*)
            [ -n "${GITHUB_TOKEN:-}" ] || { echo "✗ $console $arch vient d'un artefact de CI : exporte GITHUB_TOKEN (n'importe quel jeton GitHub)" >&2; exit 1; } ;;
    esac
    # usr/VERSION : la version que l'APK attend pour cet invité (#519), celle de
    # sa table GUESTS — à monter AVANT d'assembler, sinon le téléphone refuse le
    # paquet. Elle nomme l'appariement avec l'APK, pas l'émulateur : le x86_64 de
    # la PS4 reste un shadPS4 0.18.0 sous la version de l'aarch64 (plus haut).
    stamp=$("${HERE}/../../tools/phone-guests.sh" "$guest" | cut -d' ' -f3)
    OUT=$(mktemp -d)
    # Le dossier de la console au serveur, quand il y en a un (un -v vers un
    # chemin absent le créerait, vide, à root).
    console_mount=()
    [ -d "${CONSOLES}/${console}" ] && console_mount=(-v "${CONSOLES}/${console}":/console:ro)
    docker run --rm -e GITHUB_TOKEN -v "${OUT}":/out -v "${DL}":/dl:ro "${console_mount[@]}" \
        -v "$(readlink -f "$0")":/assemble.sh:ro "$IMAGE" \
        bash /assemble.sh --inner "$guest" "$binary" "$url" "$arch" "${tool:--}" "$templates" "$stamp"
    cp "${OUT}/${guest}-${arch}.zip" "${DL}/${guest}-${arch}.zip"
    rm -rf "${OUT}"
    echo "→ ${DL}/${guest}-${arch}.zip en place"
    exit 0
fi

guest="$2"; binary="$3"; url="$4"; arch="$5"; tool="$6"; templates="$7"; stamp="$8"
export DEBIAN_FRONTEND=noninteractive
case "$arch" in
    x86_64)  SUFFIX=""; LIBDIR=/usr/lib/x86_64-linux-gnu; MACHINE="Advanced Micro Devices X86-64" ;;
    aarch64) SUFFIX=":arm64"; LIBDIR=/usr/lib/aarch64-linux-gnu; MACHINE="AArch64"
             dpkg --add-architecture arm64 ;;
    *) echo "✗ architecture inconnue : $arch" >&2; exit 1 ;;
esac
apt-get update -qq > /dev/null 2>&1
# Le Debian du runtime : ce que les invités connus ont réclamé à un système
# (xemu : libusb ; RPCS3 : Qt sur xcb, dbus, udev, evdev, son ; Cemu : GTK 3,
# SDL2, libzip, curl, bluez, et libgcrypt — qu'elle embarque — sans son
# libgpg-error). La clôture ne prend que ce qui est réellement
# NEEDED. Pour aarch64, les mêmes paquets en multiarch : on n'en exécute aucun,
# on y lit des bibliothèques. Les noms sont ceux d'APRÈS la transition « t64 »
# de Debian (libasound2t64…) : forky n'a plus de candidat sous les anciens.
PKGS="libusb-1.0-0 libgl1 libegl1 libgles2 libglvnd0 libopengl0
    libxcb-cursor0 libxcb-icccm4 libxcb-image0 libxcb-keysyms1 libxcb-randr0 libxcb-render-util0
    libxcb-shape0 libxcb-xinerama0 libxcb-xkb1 libxcb-xfixes0 libxcb-sync1 libxcb-shm0 libxcb-glx0
    libxkbcommon-x11-0 libxkbcommon0 libfontconfig1 libfreetype6 libglib2.0-0t64 libdbus-1-3
    libasound2t64 libpulse0 libudev1 libevdev2 libpng16-16t64 libjpeg62-turbo libx11-xcb1 libxi6
    libxrender1 libxrandr2 libxext6 libsm6 libice6 libxcursor1 libxfixes3 libxinerama1 libharfbuzz0b
    libgtk-3-0t64 libsdl2-2.0-0 libzip5 libcurl4t64 libbluetooth3 libglu1-mesa
    libgpg-error0 libgcrypt20 libgmp10 libkrb5-3 libgssapi-krb5-2 libssl3t64"
if ! apt-get install -y -qq curl ca-certificates zip unzip binutils squashfs-tools \
        $(for p in $PKGS; do printf '%s%s ' "$p" "$SUFFIX"; done) > /tmp/apt.log 2>&1; then
    echo "✗ apt a refusé le lot :" >&2; tail -15 /tmp/apt.log >&2; exit 1
fi
# Pour une autre architecture, les bibliothèques DE BASE de l'image (libgmp,
# libcom-err, libgpg-error, libkrb5… — celles qu'une AppImage suppose partout)
# n'existent qu'en natif : on installe leurs jumelles :arm64, la liste venant
# de dpkg, pas d'une énumération. Une par une : apt refuse tout un lot pour un
# seul paquet sans version arm64.
if [ -n "$SUFFIX" ]; then
    for p in $(dpkg-query -W -f='${Package}\n' | grep -E '^lib' | grep -v ':'); do
        apt-get install -y -qq "${p}${SUFFIX}" > /dev/null 2>&1 || true
    done
fi
cd /tmp

echo "  $url"
case "$url" in
    https://api.github.com/*)
        # Le jeton par l'entrée standard de curl (-K -) : ni argv, ni journal.
        # curl ne le renvoie pas à l'hôte de la redirection (le stockage signé).
        printf 'header = "Authorization: Bearer %s"\n' "$GITHUB_TOKEN" \
            | curl -fsSL --retry 3 -K - -o payload "$url" \
            || { echo "✗ téléchargement impossible : $url (artefact expiré, ou jeton refusé)" >&2; exit 1; } ;;
    *)
        curl -fsSL --retry 3 -o payload "$url" || { echo "✗ téléchargement impossible : $url" >&2; exit 1; } ;;
esac
VERSION=$(basename "$url"); VERSION=${VERSION%.AppImage}; VERSION=${VERSION%.zip}; VERSION=${VERSION%.tar.gz}; VERSION=${VERSION%.apk}; VERSION=${VERSION%.AppDir}

# LE CONTENU DÉCIDE DE L'OUTIL, pas la convention (voir l'en-tête) :
#  - squashfs : chaque magie « hsqs » du fichier est essayée, on garde la
#    première dont unsquashfs -s accepte le superbloc ;
#  - sinon DwarFS : l'outil statique universel du projet, un x86_64 qui lit une
#    image quelle que soit l'architecture qu'elle contient (les images sont des
#    données), trouve l'en-tête lui-même (--image-offset=auto).
extract_appimage() {
    readelf -h "$1" | grep -q "$MACHINE" || { echo "✗ l'AppImage n'est pas $MACHINE" >&2; exit 1; }
    local offset="" candidate
    for candidate in $(grep -obUaP 'hsqs' "$1" | cut -d: -f1); do
        if unsquashfs -s -offset "$candidate" "$1" > /dev/null 2>&1; then offset="$candidate"; break; fi
    done
    if [ -n "$offset" ]; then
        echo "  squashfs à l'offset $offset"
        unsquashfs -q -n -offset "$offset" -d squashfs-root "$1" > /dev/null 2>&1 \
            || { echo "✗ unsquashfs a échoué à l'offset $offset" >&2; exit 1; }
    elif grep -qUa 'DWARFS' "$1"; then
        local dwurl
        dwurl=$(curl -s https://api.github.com/repos/mhx/dwarfs/releases/latest \
            | grep -oE '"browser_download_url": *"[^"]*"' | grep -oE 'https://[^"]*' \
            | grep -E '/dwarfs-universal-[0-9.]+-Linux-x86_64$' | head -1)
        [ -n "$dwurl" ] || { echo "✗ pas d'outil dwarfs-universal x86_64 dans la dernière release" >&2; exit 1; }
        curl -sL -o dwarfs-universal "$dwurl" && chmod +x dwarfs-universal
        echo "  image DwarFS — $(basename "$dwurl")"
        mkdir -p squashfs-root      # dwarfsextract exige un dossier de sortie EXISTANT
        ./dwarfs-universal --tool=dwarfsextract -i "$1" -o squashfs-root --image-offset=auto 2> dwarfs.err \
            || { echo "✗ dwarfsextract a échoué : $(head -c 300 dwarfs.err)" >&2; exit 1; }
    else
        echo "✗ ni squashfs ni DwarFS dans l'AppImage" >&2; exit 1
    fi
}

# LA LIVRAISON : une AppImage nue, un zip qui en CONTIENT une (shadPS4, et
# l'artefact de CI de Cemu — dont l'adresse finit par « /zip »), un AppDir
# archivé (un émulateur construit sous émulation, comme notre Citron du
# 2026-10-19 : ce que contiendrait son AppImage, déjà déballé), ou
# une archive ordinaire (Kyty).
case "$url" in
    *.AppDir.tar.gz)
        mkdir -p squashfs-root && tar xzf payload -C squashfs-root
        echo "  AppDir archivé" ;;
    *.tar.gz)
        mkdir -p tree && tar xzf payload -C tree ;;
    *.zip|*/zip)
        unzip -q payload -d zipped
        inner=$(find zipped -maxdepth 2 -name '*.AppImage' | head -1)
        [ -n "$inner" ] || { echo "✗ le zip ne contient aucune AppImage" >&2; exit 1; }
        echo "  $(basename "$inner") dans le zip"
        # Un artefact n'a pas de nom de version dans son adresse : c'est
        # l'AppImage qu'il contient qui le porte (Cemu-fced55a-aarch64).
        [ "$VERSION" = "zip" ] && VERSION=$(basename "$inner" .AppImage)
        extract_appimage "$inner" ;;
    *)
        extract_appimage payload ;;
esac

# LA NORMALISATION — une seule forme pour l'appareil.
mkdir -p norm/usr/bin norm/usr/lib
if [ -f "squashfs-root/usr/bin/${binary}" ]; then
    # linuxdeploy : la forme visée, à un détail près (checkrt, plus bas).
    cp -a squashfs-root/usr/. norm/usr/
    if [ -d squashfs-root/checkrt ]; then
        # checkrt : le libstdc++/libgcc récents, fusionnés dans usr/lib.
        for f in $(find squashfs-root/checkrt -type f -name "*.so*"); do
            case "$f" in */exec.so) continue ;; esac   # le hook d'exec de linuxdeploy : sans objet ici
            cp "$(readlink -f "$f")" "norm/usr/lib/$(basename "$f")"
            echo "  checkrt → usr/lib/$(basename "$f")"
        done
    fi
elif [ -f "squashfs-root/shared/bin/${binary}" ]; then
    # sharun : le binaire RÉEL est sous shared/bin (bin/ ne porte que le
    # lanceur statique de sharun, qu'on n'emploie pas : c'est NOTRE chargeur qui
    # lance). Tout shared/lib suit ; le qt.conf de sharun visait
    # « ../shared/lib/qt6 » depuis bin/, il vise « ../lib/qt6 » depuis usr/bin.
    cp -a "squashfs-root/shared/bin/${binary}" norm/usr/bin/
    cp -a squashfs-root/shared/lib/. norm/usr/lib/
    if [ -f squashfs-root/bin/qt.conf ]; then
        sed 's@\.\./shared/lib@../lib@' squashfs-root/bin/qt.conf > norm/usr/bin/qt.conf
    fi
    # LE SHARUN RÉCENT ne pose plus de qt.conf dans bin/ : son lanceur règle
    # QT_PLUGIN_PATH lui-même, et NOTRE chargeur ne passe pas par lui. Les
    # greffons de Qt restent alors sous le chemin du build (notre Citron :
    # lib/root/.cache/cpm/qt-bin/6.9.3/gcc_arm64/plugins), et Qt ne trouverait
    # pas sa plateforme xcb. Ce Qt se range sous usr/lib/qt6, comme dans
    # l'AppImage de l'amont, avec le qt.conf qui y mène.
    if [ ! -f norm/usr/bin/qt.conf ]; then
        qxcb=$(find norm/usr/lib -path '*/plugins/platforms/libqxcb.so' | head -1)
        if [ -n "$qxcb" ]; then
            qtprefix=$(dirname "$(dirname "$(dirname "$qxcb")")")
            if [ "$qtprefix" != norm/usr/lib/qt6 ]; then
                mkdir -p norm/usr/lib/qt6
                mv "$qtprefix"/* norm/usr/lib/qt6/
                # Le chemin du build, vidé : seulement lui.
                top="norm/usr/lib/$(echo "${qtprefix#norm/usr/lib/}" | cut -d/ -f1)"
                find "$top" -type d -empty -delete 2>/dev/null || true
            fi
            printf '[Paths]\nPrefix = ../lib/qt6\nPlugins = plugins\n' > norm/usr/bin/qt.conf
            echo "  Qt ${qtprefix#norm/usr/lib/} → usr/lib/qt6, qt.conf écrit"
        fi
    fi
    echo "  sharun → usr/bin/${binary} + usr/lib"
elif [ -f "tree/${binary}" ]; then
    # Archive ordinaire : le binaire seul. Kyty y joint le lanceur Qt et ses
    # bibliothèques (lib/, plugins/) — `kyty_emulator` n'en réclame aucune
    # (NEEDED : libc, libm, libz, libstdc++, libgcc_s), le reste lui sert par
    # dlopen (SDL, Vulkan), que le runtime fournit.
    cp -a "tree/${binary}" norm/usr/bin/
    echo "  archive → usr/bin/${binary}"
else
    echo "✗ extraction sans ${binary} (ni usr/bin, ni shared/bin, ni à la racine)" >&2; exit 1
fi
# UN LIEN VERS UN DOSSIER PARENT FAIT UNE BOUCLE. sharun pose
# `lib/aarch64-linux-gnu -> .` pour les chemins multiarch de Debian ; zip suit
# les liens, et l'archive redescendait des dizaines de fois dans le même
# dossier — 5,8 Go pour 340 Mo de bibliothèques (notre Citron, le 2026-10-19).
# Notre chargeur trouve les bibliothèques par --library-path : ces liens ne lui
# servent à rien.
find norm -type l | while IFS= read -r link; do
    [ -d "$link" ] || continue
    parent=$(readlink -f "$(dirname "$link")")
    case "$parent/" in
        "$(readlink -f "$link")"/*) echo "  boucle retirée : ${link#norm/} -> $(readlink "$link")"; rm "$link" ;;
    esac
done
if [ "$tool" != "-" ]; then
    # L'outil rejoint usr/bin AVANT la clôture : ses NEEDED sont clos comme
    # ceux de l'émulateur, et il tourne sous le même traceur.
    curl -fsSL --retry 3 -o tool.zip "$tool" || { echo "✗ téléchargement impossible : $tool" >&2; exit 1; }
    unzip -q -o tool.zip -d tool
    found=$(find tool -type f -name 'pkgextract' | head -1)
    [ -n "$found" ] || { echo "✗ pas de pkgextract dans $(basename "$tool")" >&2; exit 1; }
    cp "$found" norm/usr/bin/pkgextract && chmod 755 norm/usr/bin/pkgextract
    echo "  outil → usr/bin/pkgextract ($(basename "$tool"))"
fi
if [ "$templates" != "-" ]; then
    mkdir -p norm/usr/share/gameflex
    for t in ${templates//,/ }; do
        cp "/console/$t" "norm/usr/share/gameflex/$t" || { echo "✗ gabarit absent : $t" >&2; exit 1; }
        echo "  gabarit du serveur → usr/share/gameflex/$t"
    done
fi
cd norm

# LA PLATEFORME VIENT DU RUNTIME, JAMAIS DE L'INVITÉ.
#
# La libc d'abord : une libc.so.6 d'une autre construction, chargée par NOTRE
# chargeur, partage avec lui des symboles GLIBC_PRIVATE qui ne sont garantis
# qu'entre les fichiers d'une même construction. Citron (sharun, Arch) embarque
# la sienne ; elle ne doit jamais être chargée.
#
# La pile graphique ensuite : glvnd, Mesa (libgallium, les pilotes DRI/GBM), le
# chargeur Vulkan et ses couches. Sur l'appareil, le runtime EST la pile — c'est
# lui que désignent LIBGL_DRIVERS_PATH, VK_ICD_FILENAMES et
# __GLX_VENDOR_LIBRARY_NAME. Un libGLX_mesa d'Arch avec le libgallium de Debian
# (ou l'inverse), c'est deux versions de Mesa dans un même processus. libLLVM
# suit Mesa : l'invité n'en a pas d'autre usage.
#
# Le client du son enfin, depuis le 2026-10-14 : le runtime porte PulseAudio
# — son démon ET sa bibliothèque cliente, d'une même construction Debian. Le
# libpulse.so.0 de l'AppImage d'Eden (une autre construction de la 17.0) passait
# devant celui du runtime et réclamait `pa_in_valgrind` au libpulsecommon du
# runtime, qui ne l'a pas : SDL ne chargeait pas PulseAudio, et Eden jouait
# muet (« Failed loading libpulse.so.0 … undefined symbol », mesuré sur
# l'émulateur Android).
PLATFORM='^(ld-linux|libc\.so|libm\.so|libmvec\.so|libpthread\.so|libdl\.so|librt\.so|libresolv\.so|libutil\.so|libanl\.so|libnsl\.so|libBrokenLocale\.so|libnss_|libthread_db\.so|libc_malloc_debug\.so|libmemusage\.so|libpcprofile\.so|libGL\.so|libGLX|libGLdispatch\.so|libOpenGL\.so|libEGL|libGLESv[12]|libgallium|libgbm\.so|libvulkan\.so|libVkLayer_|libLLVM|libpulse\.so|libpulse-simple\.so|libpulse-mainloop-glib\.so|libpulsecommon)'
dropped=0

# LE RUNTIME C++ LE PLUS RÉCENT GAGNE — la règle de checkrt, enfin appliquée
# comme checkrt l'applique : son hook ne préfixe le libstdc++/libgcc_s de
# l'AppImage que si le SYSTÈME est plus vieux. On fusionnait toujours, parce
# que le runtime (Debian 12) l'était toujours. Avec forky (GLIBCXX_3.4.36),
# c'est l'inverse pour Cemu (3.4.30) : son libstdc++, placé devant celui du
# runtime, privait libz3 (que LLVM 21 tire) de GLIBCXX_3.4.31 — lavapipe ne se
# chargeait pas, et Cemu n'avait « ni VK_KHR_surface ni VK_KHR_xlib_surface »
# (mesuré sur l'émulateur puis rejoué au banc par witness-vk, 2026-10-02).
newest_tag() { strings "$1" | grep -oE "^$2_[0-9.]+$" | sort -V | tail -1; }
for pair in "libstdc++.so.6:GLIBCXX" "libgcc_s.so.1:GCC"; do
    so=${pair%%:*}; tag=${pair##*:}
    { [ -e "usr/lib/$so" ] && [ -e "$LIBDIR/$so" ]; } || continue
    mine=$(newest_tag "$(readlink -f "usr/lib/$so")" "$tag")
    theirs=$(newest_tag "$(readlink -f "$LIBDIR/$so")" "$tag")
    if [ "$(printf '%s\n%s\n' "$mine" "$theirs" | sort -V | tail -1)" = "$theirs" ]; then
        rm -f "usr/lib/$so"; dropped=$((dropped + 1))
        echo "  $so : celui du runtime ($theirs) ≥ celui de l'invité ($mine) — l'invité prend le runtime"
    else
        echo "  $so : celui de l'invité ($mine) > celui du runtime ($theirs) — il reste devant"
    fi
done

for f in usr/lib/*; do
    [ -e "$f" ] || [ -L "$f" ] || continue
    if basename "$f" | grep -qE "$PLATFORM"; then rm -f "$f"; dropped=$((dropped + 1)); fi
done
for d in dri gbm vdpau; do
    [ -d "usr/lib/$d" ] && { rm -rf "usr/lib/$d"; dropped=$((dropped + 1)); }
done
echo "  plateforme retirée de l'invité : $dropped entrée(s)"

# La complétion.
RUNTIME=$(unzip -Z1 "/dl/glibc-${arch}.zip" | xargs -n1 basename)
needed() { readelf -d "$1" 2>/dev/null | grep -oE "Shared library: \[[^]]+\]" | sed -E 's/.*\[([^]]+)\]/\1/'; }
elves() { find usr/bin usr/lib usr/plugins usr/optional -type f 2>/dev/null | while read -r f; do
    [ "$(head -c4 "$f" 2>/dev/null)" = "$(printf '\177ELF')" ] && echo "$f"; done; }
# Ce qu'un invité porte DANS UN SOUS-DOSSIER de son arbre : sharun range des
# bibliothèques sous usr/lib/<sous-dossier> (pulseaudio/libpulsecommon…) et
# les rend trouvables par son lanceur (lib.path), que nous n'employons pas.
# Notre chargeur ne cherche qu'au PREMIER niveau de usr/lib : une telle
# bibliothèque, réclamée par NEEDED, y remonte donc. Mesuré le 2026-10-02 :
# Citron mourait sur « libpulsecommon-17.0.so: cannot open shared object ».
inside() { find usr/lib -mindepth 2 -name "$1" -print -quit 2>/dev/null; }
added=""
while :; do
    more=0
    for f in $(elves); do
        for so in $(needed "$f"); do
            case "$so" in ld-linux-*) continue ;; esac
            [ -e "usr/lib/$so" ] && continue
            echo "$RUNTIME" | grep -qx "$so" && continue
            deep=$(inside "$so")
            if [ -n "$deep" ]; then
                cp "$(readlink -f "$deep")" "usr/lib/$so"; added="$added $so(${deep#usr/lib/})"; more=1
                continue
            fi
            # UN GREFFON ALSA EST FACULTATIF : ALSA ne le charge que si sa
            # configuration le nomme, et le son des invités passe par
            # PulseAudio (SDL). Celui dont une dépendance manque part, au lieu
            # d'arrêter l'assemblage — la nocturne d'Eden du 2026-10-18 porte
            # le greffon de rééchantillonnage, qui réclame libsamplerate.
            if [ ! -e "$LIBDIR/$so" ] && [ "${f#usr/lib/alsa-lib/}" != "$f" ]; then
                rm -f "$f"; echo "  greffon ALSA retiré : ${f#usr/lib/} (sans $so)"; more=1
                continue 2
            fi
            [ -e "$LIBDIR/$so" ] || { echo "✗ $so, réclamée par $f, introuvable dans le Debian du runtime" >&2; exit 1; }
            cp "$(readlink -f "$LIBDIR/$so")" "usr/lib/$so"; added="$added $so"; more=1
        done
    done
    [ "$more" = 0 ] && break
done

# Verdict avant d'empaqueter : la bonne machine partout.
for f in $(elves); do
    readelf -h "$f" | grep -q "$MACHINE" || { echo "✗ $f n'est pas $MACHINE" >&2; exit 1; }
done
# Et la glibc que l'invité réclame tient dans celle du runtime.
want=$(for f in $(elves); do objdump -T "$f" 2>/dev/null; done | grep -oE 'GLIBC_[0-9]+\.[0-9]+' | sort -t. -k2 -n -u | tail -1)
have=$(dpkg -s "libc6${SUFFIX}" | sed -n 's/^Version: \([0-9]*\.[0-9]*\).*/\1/p')
[ "$(printf '%s\n%s\n' "${want#GLIBC_}" "$have" | sort -V | tail -1)" = "$have" ] \
    || { echo "✗ l'invité réclame $want, le runtime n'a que $have" >&2; exit 1; }

echo "$stamp" > usr/VERSION
# Sans -y : un lien symbolique part comme une COPIE de sa cible. Le zip est lu
# par ZipInputStream (Xserver.ensureGuest), qui ne voit pas les attributs Unix —
# un lien y deviendrait un fichier texte portant le nom de sa cible.
zip -q -r -X "/out/${guest}-${arch}.zip" usr
echo "===== $guest — $stamp ($VERSION, $arch) ====="
echo "  glibc réclamée : ${want:-aucune}, runtime : $have"
echo "  ajoutées depuis Debian :${added:- rien}"
echo "  $(find usr/lib -name '*.so*' | wc -l) bibliothèques, $(du -sh usr | cut -f1), zip $(stat -c %s "/out/${guest}-${arch}.zip") o"
chown -R "$(stat -c '%u:%g' /out)" /out
