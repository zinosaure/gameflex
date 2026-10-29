#!/usr/bin/env bash
# Assemble le runtime glibc d'une architecture — ce que l'application télécharge
# pour faire vivre un binaire glibc (voir glibc/README.md) :
#
#   glibc-<arch>.zip        libc.so.6 (Debian forky, glibc 2.43), les témoins
#                           (witness, witness-mt), le client X (draw-xcb) et
#                           la fermeture de libxcb — posé dans les téléchargements
#                           de la webapp, choisi par ABI (Xserver.glibcZipPath) ;
#   libglibcloader.so       le chargeur (ld-linux-*), posé dans jniLibs/<abi>/ —
#                           le seul endroit d'où l'application peut l'exécuter.
#
# Tout est compilé dans un Debian jetable (RUNTIME_IMAGE), contre la libc qu'il
# livre : aucune discordance de version possible entre le chargeur, la libc et
# ce qui tourne dessus. aarch64 par multiarch + cross-gcc, sans rien exécuter
# d'arm.
#
#   tools/assemble-glibc.sh x86_64      (l'émulateur)
#   tools/assemble-glibc.sh aarch64     (l'appareil)
#
# Reproductible : relancer après un changement des sources du banc, puis bumper
# GLIBC_VERSION dans Xserver.java pour que l'appareil re-télécharge — et
# réassembler les invités (tools/assemble-appimage.sh), dont la fermeture a été
# calculée contre le runtime précédent.
set -euo pipefail

# LE DEBIAN DU RUNTIME — une seule déclaration, et elle VOYAGE DANS LE ZIP
# (fichier RUNTIME) : tools/assemble-appimage.sh y lit l'image pour clore les
# NEEDED d'un émulateur contre la MÊME distribution que le runtime qu'il
# trouvera sur l'appareil. Deux déclarations finiraient par diverger.
#
# FORKY, ET NON PLUS DEBIAN 12 — ni trixie. Mesuré le 2026-10-02 par
# `objdump -T`, sur les fichiers mêmes que le serveur emploie
# (consoles/emulators.sh) :
#
#   Cemu 2.6                       GLIBC_2.34   (Debian 12, 2.36 : suffisait)
#   shadPS4 0.18.0, KytyPS5        GLIBC_2.38   (Debian 12 : refusé)
#   Citron nightly 0237a9b88       GLIBC_2.43   (trixie, 2.41 : refusé)
#
# Et le 2026-10-22, Eden (qui remplace Citron, chantier 25) : sa nocturne
# aarch64 réclame GLIBC_2.43 et y tient ; l'amd64, GLIBC_2.44 — l'émulateur
# Android reste donc sur Eden 0.2.1 (assemble-appimage.sh, phone_x86_64_of).
#
# Citron était une AppImage « sharun » construite sur Arch, comme Eden : elle embarque sa
# propre glibc 2.43, mais son chargeur vivrait dans les données de
# l'application, où Android interdit d'exécuter — seul jniLibs le permet. C'est
# donc NOTRE chargeur qui doit être au moins aussi récent. Forky (testing) livre
# 2.43 ; l'étiquette datée fige l'image de départ, et le zip produit reste
# l'artefact qui fait foi.
RUNTIME_IMAGE="debian:forky-20260918-slim"

if [ "${1:-}" != "--inner" ]; then
    arch="${1:?usage: assemble-glibc.sh x86_64|aarch64}"
    HERE=$(cd "$(dirname "$0")/.." && pwd)                 # src/mobileapp
    case "$arch" in
        x86_64)  abi=x86_64 ;;
        aarch64) abi=arm64-v8a ;;
        *) echo "architecture inconnue : $arch" >&2; exit 2 ;;
    esac
    OUT=$(mktemp -d)
    docker run --rm -e RUNTIME_IMAGE="$RUNTIME_IMAGE" -v "${HERE}":/src:ro -v "${OUT}":/out "$RUNTIME_IMAGE" \
        bash /src/tools/assemble-glibc.sh --inner "$arch"
    DL="${HERE}/../webapp/src/public/downloads"
    JNI="${HERE}/app/src/main/jniLibs/${abi}"
    mkdir -p "${DL}" "${JNI}"
    cp "${OUT}/glibc-${arch}.zip" "${DL}/glibc-${arch}.zip"
    cp "${OUT}/libglibcloader-${arch}.so" "${JNI}/libglibcloader.so"
    chmod 755 "${JNI}/libglibcloader.so"
    rm -rf "${OUT}"
    echo "→ ${DL}/glibc-${arch}.zip et ${JNI}/libglibcloader.so en place"
    exit 0
fi

arch="$2"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq > /dev/null 2>&1
# Mesa CLIENT (llvmpipe : GL 4.5 en logiciel, dans le processus de l'invité) et
# libusb — ce que l'AppImage de xemu attend du système. Ces bibliothèques sont
# chargées par NOM (dlopen de glvnd et de Mesa), pas par NEEDED : on les amorce
# explicitement, la fermeture tire le reste (libLLVM en tête, ~110 Mo).
MESA_PKGS="libgl1-mesa-dri libglx-mesa0 libgl1 libegl1 libegl-mesa0 libglvnd0 libopengl0 libusb-1.0-0"
SEED_LIBS="libGL.so.1 libGLX.so.0 libGLdispatch.so.0 libOpenGL.so.0 libEGL.so.1 libGLX_mesa.so.0 libEGL_mesa.so.0 libusb-1.0.so.0"
# Vulkan logiciel (lavapipe — le même LLVM) : le chargeur, l'ICD, et son
# registre. L'invité le désigne par VK_ICD_FILENAMES=<dossier>/lvp_icd.*.json.
#
# ⚠️ CE COMMENTAIRE A MENTI, ET C'EST CE QUI A CACHÉ LE DÉFAUT. Il affirmait
# que « le json nomme la bibliothèque par son seul nom ». C'est FAUX : Debian y
# met un chemin ABSOLU de la machine de construction, qui n'existe sur aucun
# appareil — d'où « Found no drivers! » et un invité SANS aucun périphérique
# Vulkan pendant des mois (2026-09-23). Le champ est réécrit plus bas ; voir le
# bloc devant la boucle qui produit les manifestes.
VULKAN_PKGS="libvulkan1 mesa-vulkan-drivers"
VULKAN_LIBS="libvulkan.so.1 libvulkan_lvp.so"
# Polices : un processus glibc n'en a AUCUNE sur Android, et Qt (RPCS3) ne
# dessine rien sans. DejaVu, et un fonts.conf à préfixe RELATIF au fichier lui-
# même (fontconfig ≥ 2.14) — l'invité le désigne par FONTCONFIG_FILE.
FONT_PKGS="fonts-dejavu-core libfontconfig1 libfreetype6"
FONT_LIBS="libfontconfig.so.1 libfreetype.so.6"
# Les extensions X11 qu'un client CHARGE PAR NOM : SDL (xemu, shadPS4, Kyty,
# Cemu, Citron) et Qt les ouvrent par dlopen — XRandR pour le plein écran,
# XInput2 et XFixes pour le pointeur, XCursor, xkbcommon pour le clavier.
# Aucun NEEDED ne les désigne, donc aucune fermeture ne les trouverait : un
# émulateur lié statiquement à SDL (Kyty) n'aurait que ce que le runtime porte.
X11_PKGS="libxrandr2 libxcursor1 libxi6 libxfixes3 libxinerama1 libxss1 libxrender1 libxkbcommon0 libxkbcommon-x11-0"
X11_LIBS="libXrandr.so.2 libXcursor.so.1 libXi.so.6 libXfixes.so.3 libXinerama.so.1 libXss.so.1 libXrender.so.1 libxkbcommon.so.0 libxkbcommon-x11.so.0"
# LA BASE MIME de freedesktop (shared-mime-info), compilée : GIO y reconnaît le
# type d'un fichier, et gdk-pixbuf s'en sert pour choisir son chargeur
# d'images. Sans elle, TOUT est « application/octet-stream » — Cemu (GTK 3)
# mourait au premier dessin sur « Failed to load …/image-missing.png:
# Unrecognized image file format » alors que son chargeur PNG est intégré
# (mesuré sur l'émulateur, 2026-10-02). Qt (RPCS3, Citron) la lit aussi. Une
# donnée de PLATEFORME : un Linux l'a sous /usr/share/mime, Android non.
# NATIVE dans les deux cas : c'est une donnée, la même pour toute architecture, et
# sa version :arm64 exécuterait update-mime-database (arm64) à l'installation —
# apt rendait 100, sans un mot (2026-10-02).
MIME_PKGS="shared-mime-info"
MESA_PKGS="$MESA_PKGS $VULKAN_PKGS $FONT_PKGS $X11_PKGS"
SEED_LIBS="$SEED_LIBS $VULKAN_LIBS $FONT_LIBS $X11_LIBS"
case "$arch" in
    x86_64)
        # libc6-dev NOMMÉ : forky ne le tire plus avec gcc (« unistd.h: No such
        # file or directory », 2026-10-02), Debian 12 le tirait.
        apt-get install -y -qq gcc libc6-dev binutils libxcb1-dev libvulkan-dev zip $MESA_PKGS $MIME_PKGS > /dev/null 2>&1
        CC=gcc; RE=readelf; LIBDIR=/usr/lib/x86_64-linux-gnu
        LOADER=/usr/lib/x86_64-linux-gnu/ld-linux-x86-64.so.2
        MACHINE="Advanced Micro Devices X86-64" ;;
    aarch64)
        dpkg --add-architecture arm64
        apt-get update -qq > /dev/null 2>&1
        apt-get install -y -qq gcc-aarch64-linux-gnu binutils libxcb1-dev:arm64 libvulkan-dev:arm64 zip $MIME_PKGS \
            $(for p in $MESA_PKGS; do printf '%s:arm64 ' "$p"; done) > /dev/null 2>&1
        CC=aarch64-linux-gnu-gcc; RE=aarch64-linux-gnu-readelf; LIBDIR=/usr/lib/aarch64-linux-gnu
        LOADER=/usr/lib/aarch64-linux-gnu/ld-linux-aarch64.so.1
        MACHINE="AArch64" ;;
esac

# LA SORTIE AUDIO DES INVITÉS — un PulseAudio à puits nul, ce que chaque image
# de console du serveur lève avant son émulateur (consoles/*/system.pa).
#
# Ce n'est pas un confort : un jeu Wii U avance au rythme de sa sortie audio.
# Sans elle, New Super Mario Bros. U restait sur son écran de démarrage, tous
# ses fils endormis ; avec un puits nul il atteint « Save data has been
# created » (mesuré au banc, même runtime, même bundle, le 2026-10-02). Le
# démon voyage dans le runtime (pulse/), GlibcGuest le lance sous le même
# traceur. Seuls les trois modules qu'il charge sont gardés. Sans les
# recommandés : on veut le démon, pas rtkit ni le bus de session.
case "$arch" in x86_64) PULSE_PKG=pulseaudio ;; aarch64) PULSE_PKG=pulseaudio:arm64 ;; esac
if ! apt-get install -y -qq --no-install-recommends "$PULSE_PKG" > /tmp/apt-pulse.log 2>&1; then
    echo "✗ apt a refusé $PULSE_PKG :" >&2; tail -8 /tmp/apt-pulse.log >&2; exit 1
fi

W=/tmp/glibc-$arch; rm -rf "$W"; mkdir -p "$W"
# TOUT le jeu de bibliothèques de libc6, pas seulement libc.so.6. glibc 2.36
# n'a fusionné dans libc que pthread, dl, rt et util — libm.so.6 reste un
# fichier à part (xemu mourait dessus : « libm.so.6: cannot open shared object
# file », émulateur, 2026-09-13), libresolv et les libnss_* aussi ; et les
# stubs libpthread.so.0/libdl.so.2/librt.so.1 doivent EXISTER par leur nom pour
# un binaire lié sur une glibc plus ancienne, comme l'AppImage de xemu.
case "$arch" in x86_64) LIBC_PKG=libc6 ;; aarch64) LIBC_PKG=libc6:arm64 ;; esac
# dpkg liste ces fichiers sous /lib/<triplet>/ (/usr fusionné) : on filtre sur
# le triplet, sans présumer du préfixe.
TRIPLET=$(basename "$LIBDIR")
for f in $(dpkg -L "$LIBC_PKG" | grep -E "/lib/$TRIPLET/lib[^/]*\.so(\.[0-9]+)*$"); do
    cp "$(readlink -f "$f")" "$W/$(basename "$f")"
done
[ -e "$W/libm.so.6" ] || { echo "✗ libm.so.6 absent du jeu libc6" >&2; exit 1; }
$CC -O2 -o "$W/witness"    /src/glibc/bench/witness.c
$CC -O2 -pthread -o "$W/witness-mt" /src/glibc/bench/witness_mt.c
$CC -O2 -o "$W/draw-xcb"   /src/glibc/bench/draw_xcb.c -lxcb
# LE TÉMOIN VULKAN — il prouve que le chargeur de l'invité trouve un pilote.
#
# Il n'existait pas, et c'est pour ça qu'un manifeste ICD cassé a survécu des
# mois : `witness` prouve la libc, `witness-mt` les fils, `draw-xcb` X11, et
# personne ne regardait Vulkan. Les invités réels sont de mauvais instruments —
# RPCS3 affiche son accueil AVANT d'initialiser le moindre moteur de rendu, donc
# son silence ne prouve rien. Désormais : `witness-vk` répond en une ligne.
#
# `libvulkan-dev` n'est installé que pour les EN-TÊTES (branches ci-dessus) ;
# rien de plus n'est embarqué, le binaire se lie au `libvulkan.so.1` du bundle.
$CC -O2 -o "$W/witness-vk"  /src/glibc/bench/witness_vk.c -lvulkan
# Le LD_PRELOAD de traduction de chemins (voir glibc/pathshim.c), et le
# /proc/sys/vm/overcommit_memory qu'Android refuse de laisser lire : « 0 »,
# le mode heuristique d'un Linux ordinaire — celui que RPCS3 attend.
$CC -O2 -shared -fPIC -o "$W/libpathshim.so" /src/glibc/pathshim.c -ldl
printf '0\n' > "$W/overcommit_memory"
for so in $SEED_LIBS; do cp "$(readlink -f "$LIBDIR/$so")" "$W/$so"; done
# Le pilote DRI, à plat : l'invité le trouve par LIBGL_DRIVERS_PATH=<dossier>.
cp "$(readlink -f "$LIBDIR/dri/swrast_dri.so")" "$W/swrast_dri.so"
# Le registre de vendeur EGL de glvnd (référence libEGL_mesa par son nom) : un
# invité en EGL le désigne par __EGL_VENDOR_LIBRARY_FILENAMES.
cp /usr/share/glvnd/egl_vendor.d/50_mesa.json "$W/50_mesa.json"
# L'ICD Vulkan de lavapipe, et les polices avec leur configuration.
#
# LE CHEMIN DU MANIFESTE EST RÉÉCRIT, ET CE N'EST PAS COSMÉTIQUE.
#
# Le json de Debian porte un chemin ABSOLU de la machine de construction :
#
#     "library_path": "/usr/lib/x86_64-linux-gnu/libvulkan_lvp.so"
#
# Ce dossier n'existe sur AUCUN appareil : le runtime est extrait à plat dans
# les données de l'application. Le chargeur Vulkan répondait donc « Found no
# drivers! » et `vkCreateInstance` rendait -9 — autrement dit **l'invité n'a
# jamais eu le moindre périphérique Vulkan**, pas même lavapipe, depuis que ce
# bundle existe. xemu et RPCS3 n'ont tourné qu'en OpenGL, et toute conclusion
# antérieure sur « le rendu Vulkan logiciel » est à reprendre (constaté le
# 2026-09-23, en lisant le json livré dans le zip).
#
# La spécification du chargeur tranche la forme : un `library_path` qui
# contient un séparateur est résolu RELATIVEMENT au dossier du manifeste — et
# c'est exactement là que `libvulkan_lvp.so` est posé, à côté. On écrit donc
# `./libvulkan_lvp.so` plutôt qu'un nom nu, qui lui dépendrait du chemin de
# recherche du système.
#
# ET LE NOM DU FICHIER EST CELUI QUE L'APPAREIL ATTEND, quelle que soit la
# version de Mesa. Debian 12 livrait `lvp_icd.<arch>.json` avec un chemin
# absolu ; Mesa 26 (forky) livre `lvp_icd.json`, sans architecture, avec un nom
# NU (« libvulkan_lvp.so ») — c'est ce qui permet aux deux architectures de
# partager le fichier en multiarch. GlibcGuest désigne
# `lvp_icd.<abi>.json` : on écrit donc ce nom-là, et le chemin devient
# `./libvulkan_lvp.so` dans les deux cas (le préfixe est facultatif au motif).
for icd in /usr/share/vulkan/icd.d/lvp_icd*.json; do
    sed -E 's@"library_path"[[:space:]]*:[[:space:]]*"([^"]*/)?(libvulkan_[a-z0-9]*\.so)"@"library_path": "./\2"@' \
        "$icd" > "$W/lvp_icd.$arch.json"
done
[ -s "$W/lvp_icd.$arch.json" ] || { echo "✗ aucun manifeste lavapipe dans /usr/share/vulkan/icd.d" >&2; exit 1; }
# TURNIP, LE GPU D'UN ADRENO — aarch64 seulement, compilé pour KGSL par
# tools/build-turnip.sh (celui de Debian ne parle qu'au DRM d'un Linux de
# bureau). Son manifeste prend le `api_version` de celui de Debian, même Mesa,
# et le même chemin RELATIF que lavapipe. GlibcGuest le désigne quand
# /dev/kgsl-3d0 existe (useTurnip) ; ailleurs ce fichier dort.
if [ "$arch" = aarch64 ]; then
    TURNIP=/src/build/turnip/libvulkan_freedreno_kgsl.so
    [ -s "$TURNIP" ] || { echo "✗ Turnip KGSL absent : lance d'abord tools/build-turnip.sh" >&2; exit 1; }
    cp "$TURNIP" "$W/libvulkan_freedreno_kgsl.so"
    sed -E 's@"library_path"[[:space:]]*:[[:space:]]*"[^"]*"@"library_path": "./libvulkan_freedreno_kgsl.so"@' \
        /usr/share/vulkan/icd.d/freedreno_icd.json > "$W/turnip_icd.$arch.json"
    grep -q '"./libvulkan_freedreno_kgsl.so"' "$W/turnip_icd.$arch.json" \
        || { echo "✗ manifeste Turnip non réécrit" >&2; exit 1; }
fi
# La base MIME COMPILÉE seule (mime.cache et ses compagnons, quelques Mo) : les
# milliers de XML sources sous image/, application/… ne servent qu'à la
# recompiler. Rangée sous share/mime, l'invité la trouve par XDG_DATA_DIRS.
mkdir -p "$W/share/mime"
find /usr/share/mime -maxdepth 1 -type f -exec cp {} "$W/share/mime/" \;
[ -s "$W/share/mime/mime.cache" ] || { echo "✗ pas de mime.cache — update-mime-database n'a pas tourné" >&2; exit 1; }
# Le démon PulseAudio et ses modules (voir PULSE_PKG plus haut). Ses
# bibliothèques privées — libpulsecore, libpulsecommon (RUNPATH
# …/pulseaudio), libprotocol-native et libprotocol-simple (dans le dossier des
# modules) — vont À PLAT, là où le --library-path du lancement les trouve ; les
# modules, eux, se chargent par --dl-search-path.
#
# simple-protocol-unix rend le moniteur du puits sur une socket : c'est ce que
# l'application joue au haut-parleur (GuestSound, 2026-10-11). sine sert à
# l'essayer sans jeu. Ajoutés ce jour-là à chaud aux deux zips existants, pris
# du MÊME paquet (17.0+dfsg1-3) — ce script les produit désormais d'office.
PULSE_MODS=$(dirname "$(dpkg -L "$PULSE_PKG" | grep '/module-null-sink\.so$')")
mkdir -p "$W/pulse/modules"
cp "$(readlink -f /usr/bin/pulseaudio)" "$W/pulse/pulseaudio"
for m in module-null-sink module-native-protocol-unix module-always-sink \
         module-simple-protocol-unix module-sine; do
    cp "$PULSE_MODS/$m.so" "$W/pulse/modules/$m.so"
done
cp "$PULSE_MODS/libprotocol-native.so" "$W/libprotocol-native.so"
cp "$PULSE_MODS/libprotocol-simple.so" "$W/libprotocol-simple.so"
for so in "$LIBDIR"/pulseaudio/*.so; do cp "$(readlink -f "$so")" "$W/$(basename "$so")"; done
mkdir -p "$W/fonts"
cp /usr/share/fonts/truetype/dejavu/DejaVuSans.ttf /usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf \
   /usr/share/fonts/truetype/dejavu/DejaVuSansMono.ttf "$W/fonts/"
cat > "$W/fonts.conf" <<'XML'
<?xml version="1.0"?>
<!DOCTYPE fontconfig SYSTEM "fonts.dtd">
<!-- Le runtime glibc de GameFlex : les polices sont à côté de ce fichier
     (prefix="relative"), le cache aussi — le dossier est le nôtre, il est
     inscriptible. FONTCONFIG_FILE désigne ce fichier. -->
<fontconfig>
  <dir prefix="relative">fonts</dir>
  <cachedir prefix="relative">fontcache</cachedir>
  <alias><family>sans-serif</family><prefer><family>DejaVu Sans</family></prefer></alias>
  <alias><family>monospace</family><prefer><family>DejaVu Sans Mono</family></prefer></alias>
</fontconfig>
XML

# La fermeture transitive de TOUT le dossier : on itère jusqu'à ce que plus
# aucun NEEDED ne manque. Ce que libc fournit déjà n'est pas embarqué ; tout le
# reste vient du même Debian, même glibc.
# Seul le chargeur n'est pas dans le zip : il vit dans jniLibs, d'où l'app
# peut l'exécuter. Tout le reste — libc comprise — est embarqué.
SYS="ld-linux-x86-64.so.2 ld-linux-aarch64.so.1"
needed() { $RE -d "$1" 2>/dev/null | grep -oE "Shared library: \[[^]]+\]" | sed -E 's/.*\[([^]]+)\]/\1/'; }
while :; do
    added=0
    for f in "$W"/*.so* "$W"/witness "$W"/witness-mt "$W"/draw-xcb "$W"/witness-vk "$W"/pulse/pulseaudio "$W"/pulse/modules/*.so; do
        for so in $(needed "$f"); do
            echo "$SYS" | grep -qw "$so" && continue
            [ -e "$W/$so" ] && continue
            src="$LIBDIR/$so"
            [ -e "$src" ] || { echo "✗ bibliothèque introuvable : $so (réclamée par $(basename "$f"))" >&2; exit 1; }
            cp "$(readlink -f "$src")" "$W/$so"; added=1
        done
    done
    [ "$added" = 0 ] && break
done

# Verdict AVANT d'empaqueter : la bonne machine partout, plus aucun NEEDED
# non satisfait. Un bundle qui ment coûte un tour d'appareil.
for f in "$W"/* "$W"/pulse/pulseaudio "$W"/pulse/modules/*.so; do
    case "$f" in *.json|*.conf|*/fonts|*/share|*/pulse|*/overcommit_memory) continue ;; esac
    $RE -h "$f" | grep -q "$MACHINE" || { echo "✗ $(basename "$f") n'est pas $MACHINE" >&2; exit 1; }
    for n in $(needed "$f"); do
        echo "$SYS" | grep -qw "$n" && continue
        [ -e "$W/$n" ] || { echo "✗ $(basename "$f") réclame $n, absent" >&2; exit 1; }
    done
done
$RE -h "$LOADER" | grep -q "$MACHINE" || { echo "✗ chargeur : mauvaise machine" >&2; exit 1; }

# L'image d'où vient ce runtime, posée APRÈS le verdict (ce n'est pas un ELF) :
# tools/assemble-appimage.sh la lit dans le zip pour clore un invité contre la
# même distribution. Sur l'appareil, ce n'est qu'un fichier texte de plus.
echo "$RUNTIME_IMAGE" > "$W/RUNTIME"

# À plat, sauf fonts/ et share/ (des sous-dossiers, que ensureGlibc garde tels quels).
( cd "$W" && zip -q -r -X "/out/glibc-$arch.zip" . )
cp "$LOADER" "/out/libglibcloader-$arch.so"
chown -R "$(stat -c '%u:%g' /out)" /out

echo "===== glibc $arch — $(dpkg -s "$LIBC_PKG" | grep -m1 '^Version') ====="
for f in "$W"/*; do [ -f "$f" ] && printf "  %-16s %8s o\n" "$(basename "$f")" "$(stat -c %s "$f")"; done
echo "  fonts/ : $(ls "$W/fonts" | wc -l) polices"
printf "  %-16s %8s o  (→ jniLibs)\n" "$(basename "$LOADER")" "$(stat -c %s "$LOADER")"
echo "  zip : $(stat -c %s "/out/glibc-$arch.zip") o"
