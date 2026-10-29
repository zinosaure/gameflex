#!/usr/bin/env bash
# Un émulateur de bureau AARCH64 du téléphone, lancé sans téléphone.
#
# Le bundle (downloads/<invité>-aarch64.zip) avec le runtime aarch64
# (downloads/glibc-aarch64.zip), comme GlibcGuest le lance — notre chargeur, le
# dossier de l'invité avant le runtime, le shim de chemins préchargé, le même
# environnement — mais sous qemu-aarch64, face à un Xvfb, avec des captures
# d'écran à intervalles. Le rendu est celui de lavapipe : Turnip n'a pas de
# KGSL ici (voir le chantier 11).
#
# L'ESPACE D'ADRESSES EST CELUI D'UN TÉLÉPHONE : 39 bits, 512 Gio (`-R`). Le
# noyau GKI d'Android (android15-6.6, le Galaxy S25) n'en donne pas plus avec
# des pages de 4 Ko ; un émulateur qui réserve au-delà (shadPS4 de l'amont,
# Kyty) y échoue comme il échouera sur l'appareil. Sans `-R`, qemu en
# prêterait 48 et le banc mentirait.
#
# Ce que le banc NE reproduit PAS : le filtre seccomp de l'application et son
# traceur (qemu-user ne sait pas tracer), le GPU, la vitesse — qemu traduit
# chaque instruction, un émulateur qui recompile y paie deux traductions.
#
#   src/mobileapp/glibc/bench/arm64-guest.sh <invité> [secondes] [jeu]
#
#   … arm64-guest.sh cemu 240              → l'interface de Cemu
#   … BIOS=…/bios/switch arm64-guest.sh eden 600 /roms/x.nsp
#   … QT_INI='[Core]\nuse_multi_core\\default=false\nuse_multi_core=false' \
#       BIOS=… arm64-guest.sh eden 360 /roms/x.nsp   → des lignes de plus au qt-config.ini
#   … DL=<dossier> arm64-guest.sh eden …   → un autre bundle (glibc-aarch64.zip à côté)
#
# Les captures et la sortie de l'invité vont dans OUT (par défaut un dossier
# temporaire, dit à la fin). Le jeu est monté en LECTURE SEULE.
set -euo pipefail

if [ "${1:-}" != "--inner" ]; then
    guest="${1:?usage: arm64-guest.sh <invité> [secondes] [jeu]}"
    seconds="${2:-180}"
    game="${3:-}"
    HERE=$(cd "$(dirname "$0")/../.." && pwd)                       # src/mobileapp
    # DL : un autre dossier de bundles — pour éprouver un build sans remplacer
    # celui que la pile sert au téléphone.
    DL="${DL:-${HERE}/../webapp/src/public/downloads}"
    for zip in glibc-aarch64.zip "${guest}-aarch64.zip"; do
        [ -f "${DL}/${zip}" ] || { echo "✗ ${DL}/${zip} absent — assemble-le d'abord" >&2; exit 2; }
    done
    # Le chargeur n'est pas dans le zip : l'APK le porte (jniLibs), et il vient
    # du MÊME assemblage que la libc du zip (voir glibc/README.md).
    LOADER="${HERE}/app/src/main/jniLibs/arm64-v8a/libglibcloader.so"
    [ -f "$LOADER" ] || { echo "✗ $LOADER absent — tools/assemble-glibc.sh aarch64" >&2; exit 2; }
    IMAGE=$(unzip -p "${DL}/glibc-aarch64.zip" RUNTIME)
    OUT="${OUT:-$(mktemp -d)}"
    mounts=(-v "${DL}":/dl:ro -v "${OUT}":/out -v "$(readlink -f "$0")":/bench.sh:ro
            -v "${LOADER}":/loader:ro)
    inner_game=""
    if [ -n "$game" ]; then
        mounts+=(-v "$(readlink -f "$game")":"/game/$(basename "$game")":ro)
        inner_game="/game/$(basename "$game")"
    fi
    # Le BIOS et les correctifs de la console (bios/<console>, patches/<console>
    # du stockage froid), en lecture seule, mis en scène comme GlibcGuest le
    # fait — pour Cemu (stageCemu), Citron (ses clés) et xemu (ses BIOS et
    # son disque dur), depuis le chantier 16.
    [ -n "${BIOS:-}" ] && mounts+=(-v "$(readlink -f "$BIOS")":/bios:ro)
    [ -n "${PATCHES:-}" ] && mounts+=(-v "$(readlink -f "$PATCHES")":/patches:ro)
    # VADDR_GB : la plage d'adresses du jeu pour box64-shadps4 (son config.json) ;
    # BOX64_* : ses réglages de diagnostic, transmis tels quels s'ils sont posés
    # (BOX64_DYNAREC=0 pour l'interpréteur, BOX64_SHOWSEGV=1, BOX64_SHOWBT=1, BOX64_LOG=2).
    docker run --rm -e VA="${VA:-39}" -e VADDR_GB="${VADDR_GB:-256}" -e QT_INI \
        -e BOX64_DYNAREC -e BOX64_LOG -e BOX64_SHOWSEGV -e BOX64_SHOWBT -e BOX64_LOAD_ADDR \
        "${mounts[@]}" "$IMAGE" bash /bench.sh --inner "$guest" "$seconds" "$inner_game"
    echo "→ captures et sortie : ${OUT}"
    exit 0
fi

guest="$2"; seconds="$3"; game="$4"
export DEBIAN_FRONTEND=noninteractive
apt-get update -qq > /dev/null 2>&1
apt-get install -y -qq qemu-user xvfb x11-apps imagemagick unzip > /dev/null 2>&1 \
    || { echo "✗ apt a refusé les outils du banc" >&2; exit 1; }

G=/tmp/rt; C=/tmp/guest; H=/tmp/home
mkdir -p "$G" "$C" "$H/shm"
unzip -q /dl/glibc-aarch64.zip -d "$G"
unzip -q "/dl/${guest}-aarch64.zip" -d "$C"

# Ce que GlibcGuest.start passe à chaque émulateur (base_args de son
# adapter.json), et le binaire que l'amont nomme.
args=()
case "$guest" in
    xemu)    binary=xemu;          args=(-full-screen); [ -n "$game" ] && args+=(-dvd_path "$game") ;;
    rpcs3)   binary=rpcs3;         [ -n "$game" ] && args=(--no-gui "$game") ;;
    cemu)    binary=Cemu;          args=(-f); [ -n "$game" ] && args+=(-g "$game") ;;
    eden)    binary=eden;          [ -n "$game" ] && args=(-f -g "$game") ;;
    # Un jeu PS4 déballé est un DOSSIER : shadPS4 prend son eboot.bin. En
    # aarch64 le bundle est « Box64 + NOTRE shadPS4 x86_64 » (tools/box64/
    # bundle.sh, chantier 17) : Box64 exécute le x86, la plage d'adresses du jeu
    # ramenée par son config.json (plus bas). Deux traductions ici (Box64 puis
    # qemu) : seul le chemin compte.
    shadps4) binary=box64;          args=("$C/usr/x86_64/bin/shadps4" --fullscreen true)
             [ -d "$game" ] && game="$game/eboot.bin"
             [ -n "$game" ] && args+=(-g "$game") ;;
    # La PS5 aussi : Box64 et notre Kyty x86_64 (option compacte).
    kyty)    binary=box64;          args=("$C/usr/x86_64/bin/kyty_emulator" --fullscreen)
             [ -n "$game" ] && args+=(--game "$game") ;;
    *) echo "✗ invité inconnu : $guest" >&2; exit 2 ;;
esac

# L'ENVIRONNEMENT DE GlibcGuest.chain + builderFor, lavapipe.
export HOME="$H" DISPLAY=:0 XDG_RUNTIME_DIR=/tmp
export XDG_CONFIG_HOME="$H/.config" XDG_DATA_HOME="$H/.local/share" XDG_CACHE_HOME="$H/.cache"
export XDG_DATA_DIRS="$C/usr/share:$G/share"
export GAMEFLEX_PATHMAP="/proc/sys/vm/overcommit_memory=$G/overcommit_memory;/dev/shm=$H/shm;/tmp=/tmp"
export GAMEFLEX_EXE="$C/usr/bin/$binary"
export LIBGL_ALWAYS_SOFTWARE=1 LIBGL_DRIVERS_PATH="$G" __GLX_VENDOR_LIBRARY_NAME=mesa
export VK_ICD_FILENAMES="$G/lvp_icd.aarch64.json"
export FONTCONFIG_FILE="$G/fonts.conf"
export SDL_VIDEODRIVER=x11 SDL_AUDIODRIVER=dummy SDL_AUDIO_DRIVER=dummy
case "$guest" in
    rpcs3|eden) export QT_QPA_PLATFORM=xcb ;;
    cemu)
        U="$C/usr"
        export GDK_BACKEND=x11 GTK_THEME=Adwaita:dark GTK_DATA_PREFIX="$C" GTK_EXE_PREFIX="$U"
        export GTK_PATH="$U/lib/gtk-3.0" GTK_IM_MODULE_FILE="$U/lib/gtk-3.0/3.0.0/immodules.cache"
        export GSETTINGS_SCHEMA_DIR="$U/share/glib-2.0/schemas"
        export GDK_PIXBUF_MODULE_FILE="$U/lib/gdk-pixbuf-2.0/2.10.0/loaders.cache"
        export GDK_PIXBUF_MODULEDIR="$U/lib/gdk-pixbuf-2.0/2.10.0/loaders"
        export LC_ALL=C
        # stageCemu : le gabarit du serveur, sans lequel Cemu ouvre son
        # assistant de premier lancement ; les clés ; les correctifs.
        mkdir -p "$XDG_CONFIG_HOME/Cemu/controllerProfiles" "$XDG_DATA_HOME/Cemu"
        cp "$U/share/gameflex/settings.xml" "$XDG_CONFIG_HOME/Cemu/settings.xml"
        [ -f /bios/keys.txt ] && cp /bios/keys.txt "$XDG_DATA_HOME/Cemu/keys.txt"
        if [ -d /patches ]; then
            mkdir -p "$XDG_DATA_HOME/Cemu/graphicPacks/gameflex"
            cp -r /patches/. "$XDG_DATA_HOME/Cemu/graphicPacks/gameflex/"
        fi ;;
esac
case "$guest" in
    eden)
        # stageSwitchKeys et stageSwitchConfig : les clés là où Eden les lit,
        # et sa question d'arrêt désarmée (chantier 16).
        mkdir -p "$XDG_DATA_HOME/$guest/keys" "$XDG_CONFIG_HOME/$guest"
        [ -d /bios ] && cp /bios/*.keys "$XDG_DATA_HOME/$guest/keys/" 2> /dev/null
        printf '[UI]\nconfirmStop\\default=false\nconfirmStop=2\n' > "$XDG_CONFIG_HOME/$guest/qt-config.ini"
        # QT_INI : des lignes de plus pour ce fichier, sections comprises, lues
        # par printf %b — « [Cpu]\ncpu_backend\\default=false\ncpu_backend=0 »
        # impose Dynarmic comme le fait le téléphone (#358).
        if [ -n "${QT_INI:-}" ]; then
            printf '%b\n' "$QT_INI" >> "$XDG_CONFIG_HOME/$guest/qt-config.ini"
        fi ;;
    xemu)
        # stageBios et stageXemuConfig : les BIOS, le disque dur recopié (xemu
        # l'écrit), et le xemu.toml que l'application écrit sans Turnip.
        X="$XDG_DATA_HOME/xemu/xemu"
        mkdir -p "$X"
        [ -f /bios/xbox_hdd.qcow2 ] && cp /bios/xbox_hdd.qcow2 "$X/xbox_hdd.qcow2"
        printf '%s\n' '[general]' 'show_welcome = false' 'skip_boot_anim = true' \
            '[input.bindings]' 'port1 = "keyboard"' '[sys.files]' \
            'bootrom_path = "/bios/mcpx_1.0.bin"' 'flashrom_path = "/bios/Complex_4627.bin"' \
            "eeprom_path = \"$X/eeprom.bin\"" "hdd_path = \"$X/xbox_hdd.qcow2\"" \
            '[display]' 'renderer = "OPENGL"' > "$X/xemu.toml" ;;
    shadps4|kyty)
        # Les bibliothèques x86_64 du bundle (libstdc++ et ce que Box64
        # n'enveloppe pas), puis celles que Box64 fournit. BOX64_LOG=1 : ce
        # qu'il charge et où il bute, dans la sortie.
        export BOX64_LD_LIBRARY_PATH="$C/usr/x86_64/lib:$C/usr/x86_64/box64lib"
        export BOX64_LOG="${BOX64_LOG:-1}" BOX64_NOBANNER=0
        # Le x86 AU-DESSUS des plages du jeu : shadPS4 réserve 0x400000 → sa
        # plage utilisateur d'un mmap(MAP_FIXED), et Box64 chargeait son PIE à
        # 0x100000000, dedans. En natif le noyau le met en 0x5555… ; ici 192 Gio
        # (au-dessus de VADDR_GB=96, sous les 512 Gio du banc et hors du tas de
        # qemu, qui part du tiers de l'espace).
        if [ "$guest" = kyty ]; then
            # Kyty (option compacte) range sa région étendue de 256 à 384 Gio :
            # le x86 au-dessus, à 416 Gio.
            export BOX64_LOAD_ADDR="${BOX64_LOAD_ADDR:-0x6800000000}"
            mkdir -p "$XDG_DATA_HOME/kyty"
        else
            export BOX64_LOAD_ADDR="${BOX64_LOAD_ADDR:-0x3000000000}"
            # Comme l'image PS4 du serveur : home/1000 avant le premier lancement
            # (sinon la boîte modale « Save Migration »), et le réglage du chantier 17.
            P="$XDG_DATA_HOME/shadPS4"
            mkdir -p "$P/home/1000/savedata" "$P/home/1000/trophy" "$P/home/1000/inputs"
            printf '{"General": {"user_vaddr_max_in_gbytes": %s}}\n' "${VADDR_GB:-256}" > "$P/config.json"
        fi ;;
esac
if [ "$guest" != cemu ] && [ "$guest" != eden ] && [ "$guest" != xemu ] && { [ -d /bios ] || [ -d /patches ]; }; then
    echo "  (BIOS et correctifs : seuls Cemu, Eden et xemu sont mis en scène par ce banc — ignorés pour ${guest})"
fi

Xvfb :0 -ac -nolisten tcp -screen 0 1280x720x24 > /dev/null 2>&1 &
for _ in $(seq 1 30); do [ -S /tmp/.X11-unix/X0 ] && break; sleep 0.3; done
[ -S /tmp/.X11-unix/X0 ] || { echo "✗ Xvfb :0 ne s'est pas levé" >&2; exit 1; }

echo "── ${guest} : $(cat "$C/usr/VERSION"), runtime $(cat "$G/RUNTIME"), ${VA:-39} bits d'adresses"
# VA=48 : sans la limite d'un téléphone — pour savoir si un échec vient des
# 39 bits ou d'ailleurs (comparaison seulement : le téléphone, lui, a 39 bits).
reserve=(-R 0x8000000000)
# PAS DE CORE DUMP : qemu-user écrit celui de l'invité EN ENTIER, réservations
# comprises — l'arène fastmem d'un émulateur Switch (2³⁹ octets) a rempli le
# disque de la machine, 55 Go en quelques minutes (Eden à 48 bits, 2026-10-13).
ulimit -c 0
[ "${VA:-39}" = 48 ] && reserve=()
qemu-aarch64 "${reserve[@]}" /loader --inhibit-cache \
    --library-path "$C/usr/lib:$G" --preload "$G/libpathshim.so" \
    "$C/usr/bin/$binary" "${args[@]}" > /out/sortie.txt 2>&1 &
pid=$!

# Une capture toutes les 30 s ; on s'arrête tôt si l'invité meurt.
shots=0
for ((t = 30; t <= seconds; t += 30)); do
    sleep 30
    xwd -root -display :0 2> /dev/null | convert xwd:- "/out/${guest}-${t}s.png" 2> /dev/null && shots=$((shots + 1))
    kill -0 "$pid" 2> /dev/null || { echo "  l'invité s'est arrêté avant ${t} s"; break; }
done
alive=no; kill -0 "$pid" 2> /dev/null && alive=yes
kill "$pid" 2> /dev/null || true
# UN INVITÉ QUI NE S'ARRÊTE PAS AU SIGTERM — Eden, dont la partie a planté,
# l'ignore (2026-10-16, #358) — part au SIGKILL vingt secondes plus tard :
# sans ça, `wait` attendait sans fin et ni la sortie ni le journal ne
# revenaient.
for _ in $(seq 1 20); do kill -0 "$pid" 2> /dev/null || break; sleep 1; done
kill -9 "$pid" 2> /dev/null || true
code=0; wait "$pid" 2> /dev/null || code=$?
echo "  vivant à la fin : ${alive} (code ${code}), ${shots} capture(s)"
echo "── la fin de sa sortie :"
tail -25 /out/sortie.txt | sed 's/^/  | /'
# Le journal de l'émulateur lui-même, quand il en écrit un (Eden, Cemu).
for log in "$XDG_DATA_HOME/eden/log/eden_log.txt" "$XDG_DATA_HOME/Cemu/log.txt" "$XDG_DATA_HOME/shadPS4/log/shad_log.txt"; do
    [ -f "$log" ] && cp "$log" /out/
done
chown -R "$(stat -c '%u:%g' /out)" /out
