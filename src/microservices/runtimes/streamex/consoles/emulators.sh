# D'OÙ VIENT L'ÉMULATEUR D'UNE CONSOLE — la source unique (#145).
#
# Ce fichier se SOURCE depuis les scripts de l'hôte (launch.sh,
# tools/publish.sh, tools/streamex-emulators.sh).
# En shell et non en JSON parce que ses deux seuls lecteurs sont des scripts
# shell : pas de python au milieu, et surtout des commentaires possibles — le
# pourquoi d'une URL vaut d'être écrit à côté d'elle.
#
# POURQUOI LE FICHIER VIT DANS LA BIBLIOTHÈQUE, ET PAS DANS L'IMAGE
#
#   - Licence. On ne redistribue pas un émulateur. C'était déjà écrit pour le
#     téléphone : livrer la bibliothèque d'un émulateur (GPL-3.0) dans notre
#     artefact « en ferait une œuvre combinée, et
#     engagerait tout ce qu'il y a autour », là où « l'appareil qui va la
#     chercher chez son éditeur ne distribue rien ». Une image de console ne
#     se publie donc que SANS son émulateur (chantier 29).
#   - C'est GameFlex qui télécharge, jamais la personne : c'est ce qui garantit
#     que le fichier atterrit sous le bon identifiant de console.
#   - Parité. Le téléphone épingle ses versions sur celles d'ici pour que les
#     sauvegardes et le comportement soient les mêmes des deux côtés. Une seule
#     déclaration tient cette parité au lieu de la confier à la discipline.
#
# CE QUI EST DANS L'IMAGE, ET CE QUI NE L'EST PAS (chantier 29, 2026-10-24)
#
# RIEN DE L'ÉMULATEUR N'EST DANS L'IMAGE. Seul le fichier de l'éditeur vit dans
# `cores/<consoleId>/` ; le kernel le monte en lecture seule dans le conteneur
# de partie, et l'entrypoint le déballe au lancement (unpack-emulator.sh) —
# moins d'une seconde, mesuré. Jusque-là le Dockerfile le déballait au build,
# depuis un contexte nommé : l'image tenait l'émulateur, elle ne pouvait donc
# pas se publier, ni se construire chez quelqu'un qui n'a pas les sources.
#
# L'image dit ce qu'elle attend par deux étiquettes, tirées des valeurs
# ci-dessous (le test de tools/publicdist vérifie qu'elles concordent) :
# `gameflex.emulator.file`, le nom du fichier, et `gameflex.emulator.url`, son
# adresse chez l'éditeur. Le kernel les lit : il refuse une partie sans le
# fichier, et Réglages › Émulateurs le télécharge à cette adresse.
#
# `ensure_emulator` le dépose depuis l'hôte (launch.sh, tools/streamex-emulators.sh) : déjà là, rien ne se télécharge. Le kernel ne
# remplace jamais un fichier présent. C'est la bibliothèque qui le garde — et
# c'est par elle, et par un miroir, qu'un ami prend la dernière version (le
# porteur : « on ne fournit pas d'émulateurs dans le repo git, mais rien
# n'empêche de les fournir en fédération ou par un miroir »). Les cœurs
# libretro, eux, ne se téléchargent que par l'admin, depuis Réglages →
# Émulateurs (2026-10-16).
#
# Les Dockerfile reprennent ces valeurs en ARG : changer de version, c'est
# changer les deux, et le test de tools/publicdist le rappelle.

# UNE IMAGE PAR ÉMULATEUR, ET PARFOIS DEUX PAR CONSOLE (chantier 26) : la
# Switch a Citron (`switch`, le premier) et Eden (`switch-eden`, le second).
# Les fonctions ci-dessous prennent le nom de l'IMAGE ; son fichier se range
# sous la console (emulator_console).

# emulator_file <image> — le nom du fichier téléchargeable, tel qu'il est
# rangé dans cores/<console>/. C'est lui que l'image cherche au lancement.
emulator_file() {
    case "$1" in
        switch) echo "citron_nightly-0237a9b88-linux-x86_64_v3.AppImage" ;;
        switch-eden) echo "Eden-Linux-de7e6e01d4-amd64-clang-pgo.AppImage" ;;
        xbox)   echo "xemu-0.8.139-x86_64.AppImage" ;;
        wiiu)   echo "Cemu-2.6-x86_64.AppImage" ;;
        ps3)    echo "rpcs3-v0.0.43-20404-e8979d31_linux64.AppImage" ;;
        # La PS4 publie un ZIP qui CONTIENT l'AppImage : le fichier gardé est
        # donc le zip, et le déballage fait les deux étages (unzip, puis
        # --appimage-extract). Le pkgextract, lui, est un outil et non
        # l'émulateur : il reste dans l'image.
        ps4)    echo "shadps4-linux-sdl-0.20.0.zip" ;;
        # Kyty publie une archive ordinaire, pas une AppImage.
        ps5)    echo "KytyPS5-2026-10-26-597137e-Linux-x86_64.tar.gz" ;;
        # Xenia publie une AppImage par commit, sous un nom SANS version :
        # celui rangé ici la porte, pour qu'une montée de version ne
        # retrouve pas l'ancienne sous le même nom.
        xbox360) echo "xenia_canary_linux-291f0c9.AppImage" ;;
        *)      echo "" ;;
    esac
}

# emulator_url <image> — où l'éditeur le publie.
emulator_url() {
    case "$1" in
        # Citron, sa release du 2026-04-27 : celle que le téléphone a jouée.
        switch) echo "https://github.com/citron-neo/emulator/releases/download/2026-04-27/citron_nightly-0237a9b88-linux-x86_64_v3.AppImage" ;;
        # La nocturne officielle d'Eden, épinglée par son étiquette (chantier 25) :
        # la seule qui lit les jeux du firmware 22 (ZBIC).
        switch-eden) echo "https://nightly.eden-emu.dev/v1792353251.de7e6e01d4/Eden-Linux-de7e6e01d4-amd64-clang-pgo.AppImage" ;;
        xbox)   echo "https://github.com/xemu-project/xemu/releases/download/v0.8.139/xemu-0.8.139-x86_64.AppImage" ;;
        wiiu)   echo "https://github.com/cemu-project/Cemu/releases/download/v2.6/Cemu-2.6-x86_64.AppImage" ;;
        ps3)    echo "https://github.com/RPCS3/rpcs3-binaries-linux/releases/download/build-e8979d31b3a22c8bc9a6eae6bdc31756eac8ee89/rpcs3-v0.0.43-20404-e8979d31_linux64.AppImage" ;;
        ps4)    echo "https://github.com/shadps4-emu/shadPS4/releases/download/v.0.20.0/shadps4-linux-sdl-0.20.0.zip" ;;
        ps5)    echo "https://github.com/KytyPS5/KytyPS5/releases/download/KytyPS5-2026-10-26-597137e/KytyPS5-2026-10-26-597137e-Linux-x86_64.tar.gz" ;;
        xbox360) echo "https://github.com/xenia-canary/xenia-canary/releases/download/291f0c9/xenia_canary_linux.AppImage" ;;
        *)      echo "" ;;
    esac
}

# emulator_tested <image> — LES VERSIONS ÉPROUVÉES avec cette image, la plus
# récente en dernier : emulator_file, celle qu'on télécharge, est toujours la
# dernière de la liste.
#
# LA LISTE INFORME, ELLE NE BLOQUE PAS (le porteur, 2026-10-24). Toute version
# que reconnaît le motif de l'image se lance — la dernière déposée dans
# cores/<console>/ —, éprouvée ou non : un ami ou le miroir partagent souvent
# une nightly plus récente que la nôtre, et la refuser nous rattacherait à nos
# publications. Réglages › Émulateurs marque chaque fichier « testé » ou non, et
# le journal de la partie signale une version non éprouvée. Le Dockerfile en
# fait l'étiquette gameflex.emulator.tested ; le test de tools/publicdist
# vérifie qu'ils concordent.
#
# N'Y ENTRE QU'UNE VERSION JOUÉE AVEC CETTE IMAGE : une ancienne qui marchait
# avec une image d'avant n'en dit rien.
emulator_tested() {
    emulator_file "$1"
}

# emulator_consoles — les images qui embarquent un émulateur de bureau. Les
# consoles rétro n'en ont pas : leur « émulateur » est un cœur libretro, que
# l'admin télécharge depuis Réglages → Émulateurs.
emulator_consoles() {
    echo "switch switch-eden xbox wiiu ps3 ps4 ps5 xbox360"
}

# emulator_console <image> — la console d'une image : le dossier cores/<console>/
# où son fichier se range.
emulator_console() {
    case "$1" in
        switch-eden) echo "switch" ;;
        *) echo "$1" ;;
    esac
}

# ensure_emulator <image> <racine de bibliothèque> — le fichier est-il là ?
# Sinon, le déposer. Rend son dossier, cores/<console>/.
#
# Écrit sur la sortie d'ERREUR : sa sortie standard est le chemin, que
# l'appelant capture.
ensure_emulator() {
    local console=$1 library=$2
    local file url dir
    file=$(emulator_file "$console")
    url=$(emulator_url "$console")
    if [ -z "$file" ] || [ -z "$url" ]; then
        echo "ensure_emulator: console inconnue : $console" >&2

        return 1
    fi
    dir="$library/cores/$(emulator_console "$console")"
    if [ -f "$dir/$file" ]; then
        echo "   $console : $file déjà dans la bibliothèque" >&2
        echo "$dir"

        return 0
    fi
    echo "   $console : $file absent, téléchargement" >&2
    mkdir -p "$dir" || return 1
    # Fichier partiel d'abord : une coupure réseau ne doit pas laisser un
    # fichier tronqué qui passerait pour présent à la prochaine partie.
    if ! curl -fL --retry 3 -o "$dir/$file.part" "$url"; then
        rm -f "$dir/$file.part"
        echo "ensure_emulator: téléchargement impossible : $url" >&2

        return 1
    fi
    mv "$dir/$file.part" "$dir/$file"
    echo "$dir"
}
