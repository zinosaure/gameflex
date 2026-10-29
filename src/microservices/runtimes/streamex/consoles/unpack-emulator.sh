#!/bin/sh
# DÉBALLER L'ÉMULATEUR DE CETTE CONSOLE, AU LANCEMENT (chantier 29).
#
# L'image ne porte pas son émulateur. On ne redistribue pas un émulateur
# (#145), et c'est ce qui permet de publier les images des consoles : elles
# ne contiennent que notre code. Le kernel monte `cores/` du dossier en lecture
# seule, sous /gameflex/cores, et ce script y prend l'émulateur au début de
# l'entrypoint, pour le déballer à la place que l'adaptateur connaît.
#
# LE DERNIER DÉPOSÉ (le porteur, 2026-10-24). Le dossier peut tenir plusieurs
# versions : la partie prend celle qu'on a ajoutée en dernier, parmi les
# fichiers qui répondent à GAMEFLEX_EMULATOR_PATTERN — le nom de l'éditeur, sa
# version remplacée par `*`. C'est la date du fichier qui départage, pas son
# nom : Citron, Eden et Xenia nomment leurs versions par un hash, qui ne se
# classe pas. Revenir à une version, c'est supprimer la plus récente.
# GAMEFLEX_EMULATOR_FILE est celle testée avec cette image (emulators.sh) :
# celle que l'écran Émulateurs télécharge quand le dossier n'en a aucune.
#
# Jusqu'au 2026-10-24, le Dockerfile le déballait au build, depuis un contexte
# nommé : l'image tenait l'émulateur, elle ne pouvait donc pas se publier, et
# une installation publique, qui n'a pas les sources, ne pouvait pas non plus
# la construire.
#
# À CHAQUE PARTIE, SANS CACHE. Mesuré le 2026-10-24 dans l'image PS3 : RPCS3
# (94 Mo, 325 Mo déballés) en 0,3 s, Eden en 0,3 s, Cemu en 0,3 s, l'archive de
# Kyty en 1,1 s. Un cache partagé entre parties coûterait un verrou, un
# nettoyage des anciennes versions et une entrée de plus dans l'écran du cache,
# pour gagner une seconde sur un démarrage qui en prend vingt. Le déballé vit
# dans la couche du conteneur, et part avec lui.
#
#   unpack-emulator <dossier>
#
# <dossier> reçoit l'arbre de l'émulateur : le contenu de l'AppImage (son
# AppRun à la racine), ou celui de l'archive. La forme se lit au nom du
# fichier :
#
#   *.AppImage   déballée par --appimage-extract (pas de FUSE dans un conteneur)
#   *.zip        décompressée ; l'AppImage qu'elle contient est déballée à son
#                tour (shadPS4 publie un zip qui la porte)
#   *.tar.gz     décompressée telle quelle (Kyty)
#
# GAMEFLEX_EMULATOR_TESTED — les versions éprouvées avec cette image
# (emulator_tested). La liste INFORME, elle ne bloque pas (le porteur,
# 2026-10-24) : une autre version se lance aussi, et le journal le signale.
#
# GAMEFLEX_EMULATOR_DIR — où le chercher : cores/<console>/ (celui de la
# Switch pour Eden comme pour Citron).
#
# GAMEFLEX_EMULATOR_NAME — son nom pour le panneau Performance : /opt/emulator.txt
# reçoit « <nom> <version> », la version étant ce que `*` couvre dans le nom du
# fichier choisi. Écrit ici et non au build : c'est le dossier qui décide.
#
# GAMEFLEX_EMULATOR_BUILD, facultatif : NOTRE construction du binaire
# (tools/shadps4/build.sh, tools/kyty/build.sh), un motif aussi, dont `*` prend
# la version du fichier choisi. Présente dans le même dossier, elle remplace
# celui de l'éditeur à GAMEFLEX_EMULATOR_BUILD_TARGET, et le panneau
# Performance le dit ; absente, c'est celui de l'éditeur, et le journal le dit.
set -eu

dest=${1:?usage: unpack-emulator <dossier>}
tested=${GAMEFLEX_EMULATOR_FILE:?cette image ne déclare pas GAMEFLEX_EMULATOR_FILE}
pattern=${GAMEFLEX_EMULATOR_PATTERN:?cette image ne déclare pas GAMEFLEX_EMULATOR_PATTERN}
dir=${GAMEFLEX_EMULATOR_DIR:?cette image ne déclare pas GAMEFLEX_EMULATOR_DIR}
marker="$dest/.gameflex-emulator"

# Le plus récent d'abord. Le motif se développe ici, sans guillemets : c'est
# voulu. Aucun nom de fichier d'émulateur ne porte d'espace (le kernel les
# refuse, retro_files.IsEmulatorFile).
# shellcheck disable=SC2012,SC2086
file=$(cd "$dir" 2>/dev/null && ls -t -- $pattern 2>/dev/null | head -n 1) || true

# LE KERNEL REFUSE DÉJÀ LA PARTIE quand aucun ne s'y trouve (requireEmulator) :
# ce message ne se lit que si l'on lance l'image à la main, ou si le fichier a
# disparu entre les deux.
if [ -z "$file" ]; then
    echo "[entry] ERROR: no $pattern in ${dir#/gameflex/} — download $tested from Settings › Emulators" >&2
    exit 1
fi
source="$dir/$file"
case " ${GAMEFLEX_EMULATOR_TESTED:-} " in
    *" $file "*) ;;
    *) echo "[entry] WARNING: $file is not a version tested with this image (tested: ${GAMEFLEX_EMULATOR_TESTED:-none}) — if the game misbehaves, try $tested" >&2 ;;
esac

# La version : ce que `*` couvre dans le nom choisi.
prefix=${pattern%%\**}
suffix=${pattern#*\*}
version=${file#"$prefix"}
version=${version%"$suffix"}

# Un conteneur relancé l'a déjà : rien à refaire.
if [ -f "$marker" ] && [ "$(cat "$marker")" = "$file" ]; then
    echo "[entry] emulator: $file already unpacked"
    exit 0
fi

# Le travail se fait à côté de la destination : même système de fichiers, donc
# un `mv` et non une seconde copie.
parent=$(dirname "$dest")
mkdir -p "$parent"
work=$(mktemp -d "$parent/.unpack-XXXXXX")
trap 'rm -rf "$work"' EXIT

# extract_appimage <AppImage> — son arbre, à sa place. Une AppImage récente
# (uruntime, DwarFS) s'extrait dans `AppDir` et ne laisse qu'un lien
# `squashfs-root` : c'est l'arbre réel qu'on déplace.
extract_appimage() {
    chmod +x "$1"
    ( cd "$work" && "$1" --appimage-extract >/dev/null )
    rm -rf "$dest"
    mv "$(readlink -f "$work/squashfs-root")" "$dest"
}

started=$(date +%s)
case "$file" in
    *.AppImage)
        cp "$source" "$work/emulator.AppImage"
        extract_appimage "$work/emulator.AppImage"
        ;;
    *.zip)
        unzip -q "$source" -d "$work/zip"
        appimage=$(find "$work/zip" -name '*.AppImage' | head -n 1)
        if [ -n "$appimage" ]; then
            extract_appimage "$appimage"
        else
            rm -rf "$dest"
            mv "$work/zip" "$dest"
        fi
        ;;
    *.tar.gz)
        # La destination peut exister : le kernel y monte les sauvegardes de
        # Kyty (/opt/kyty/_SaveData), qui écrit dans son dossier de travail.
        mkdir -p "$dest"
        tar xzf "$source" -C "$dest"
        ;;
    *)
        echo "[entry] ERROR: $file: unknown emulator format (expected .AppImage, .zip or .tar.gz)" >&2
        exit 1
        ;;
esac
echo "[entry] emulator: $file unpacked in $(( $(date +%s) - started )) s"

echo "${GAMEFLEX_EMULATOR_NAME:-emulator} $version" > /opt/emulator.txt

if [ -n "${GAMEFLEX_EMULATOR_BUILD:-}" ]; then
    build="${GAMEFLEX_EMULATOR_BUILD%%\**}$version${GAMEFLEX_EMULATOR_BUILD#*\*}"
    if [ -f "$dir/$build" ]; then
        # -p : la date du fichier suit. Kyty marque son cache de shaders de
        # celle de son binaire (BuildStamp) : recopié à neuf, il le
        # jetait à chaque partie.
        cp -p "$dir/$build" "$GAMEFLEX_EMULATOR_BUILD_TARGET"
        chmod +x "$GAMEFLEX_EMULATOR_BUILD_TARGET"
        sed -i 's/$/ (GameFlex)/' /opt/emulator.txt
        echo "[entry] emulator: GameFlex build $build"
    else
        echo "[entry] emulator: publisher build ($build not in ${dir#/gameflex/})"
    fi
fi

echo "$file" > "$marker"
