# D'OÙ VIENT L'ÉMULATEUR D'UNE CONSOLE — la source unique (#145).
#
# Ce fichier se SOURCE depuis les scripts de l'hôte (launch.sh,
# tools/publish.sh, tools/streamex-emulators.sh).
# En shell et non en JSON parce que ses lecteurs sont des scripts shell, et
# pour garder des commentaires : le pourquoi d'une version vaut d'être écrit à
# côté d'elle.
#
# D'OÙ IL VIENT : UN PAQUET STREAMEX (chantier 63, 2026-11-18). Le porteur :
# « Nous on ne fournit pas d'émulateurs, on fournit un moteur qui peut utiliser
# les émulateurs. » La version validée de chaque image est dans un paquet de la
# Release `streamex` de zinosaure/gameflex-streamex (tools/streamex-packages.sh),
# listé dans son index `components.json` avec son sha256. Le kernel l'installe
# depuis Réglages › Émulateurs ; `ensure_emulator`, plus bas, fait de même depuis
# l'hôte. Plus d'adresse d'éditeur : une version retirée ou un éditeur parti
# (Citron, 2026) ne casse plus rien.
#
# POURQUOI LE FICHIER VIT DANS LA BIBLIOTHÈQUE, ET PAS DANS L'IMAGE
#
#   - L'image de la console reste le moteur seul : elle déballe l'émulateur au
#     lancement depuis cores/<console>/ (chantier 29), et elle se publie telle
#     quelle.
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
# L'image dit ce qu'elle attend par ses étiquettes, tirées des valeurs
# ci-dessous (le test de tools/publicdist vérifie qu'elles concordent) :
# `gameflex.emulator.file`, le nom du fichier. Le kernel la lit : il refuse une
# partie sans le fichier, et Réglages › Émulateurs l'installe depuis le paquet
# streamex qui le porte.
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
        switch-eden) echo "Eden-Linux-d85bad8924-amd64-clang-pgo.AppImage" ;;
        # Lichii publie, comme Xenia, un nom SANS version : celui rangé ici la
        # porte (sa version publiée, build-316).
        switch2) echo "Lichii-build-316-x86_64-pgo.AppImage" ;;
        xbox)   echo "xemu-0.8.139-x86_64.AppImage" ;;
        wiiu)   echo "Cemu-2.6-x86_64.AppImage" ;;
        ps3)    echo "rpcs3-v0.0.43-20607-fb6737b1_linux64.AppImage" ;;
        # La PS4 publie un ZIP qui CONTIENT l'AppImage : le fichier gardé est
        # donc le zip, et le déballage fait les deux étages (unzip, puis
        # --appimage-extract). Le pkgextract, lui, est un outil et non
        # l'émulateur : il reste dans l'image.
        ps4)    echo "shadps4-linux-sdl-0.20.0.zip" ;;
        # Kyty publie une archive ordinaire, pas une AppImage.
        ps5)    echo "KytyPS5-2026-11-04-91c5820-Linux-x86_64.tar.gz" ;;
        # Xenia publie une AppImage par commit, sous un nom SANS version :
        # celui rangé ici la porte, pour qu'une montée de version ne
        # retrouve pas l'ancienne sous le même nom.
        xbox360) echo "xenia_canary_linux-f9431eb.AppImage" ;;
        # Les ports PC (chantier 40) : un port par image, tous rangés sous
        # cores/ports/ (emulator_console). Zelda 64: Recompiled publie un zip
        # qui porte une archive tar.gz.
        ports-zelda64recomp) echo "Zelda64Recompiled-v1.2.2-Linux-X64.zip" ;;
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
    echo "switch switch-eden switch2 xbox wiiu ps3 ps4 ps5 xbox360 ports-zelda64recomp"
}

# emulator_console <image> — la console d'une image : le dossier cores/<console>/
# où son fichier se range.
emulator_console() {
    case "$1" in
        switch-eden) echo "switch" ;;
        ports-*) echo "ports" ;;
        *) echo "$1" ;;
    esac
}

# STREAMEX_INDEX — l'index des paquets streamex (chantier 63) ; surchargeable.
STREAMEX_INDEX=${STREAMEX_INDEX:-https://github.com/zinosaure/gameflex-streamex/releases/download/streamex/components.json}

# ensure_emulator <image> <racine de bibliothèque> — le fichier est-il là ?
# Sinon, installer le paquet streamex de l'image (`streamex-<image>`), comme le
# fait le kernel : son empreinte vérifiée, ses fichiers déposés sans remplacer
# ceux déjà là. Rend son dossier, cores/<console>/.
#
# Écrit sur la sortie d'ERREUR : sa sortie standard est le chemin, que
# l'appelant capture.
ensure_emulator() {
    local console=$1 library=$2
    local file dir
    file=$(emulator_file "$console")
    if [ -z "$file" ]; then
        echo "ensure_emulator: console inconnue : $console" >&2

        return 1
    fi
    dir="$library/cores/$(emulator_console "$console")"
    if [ -f "$dir/$file" ]; then
        echo "   $console : $file déjà dans la bibliothèque" >&2
        echo "$dir"

        return 0
    fi
    echo "   $console : $file absent, paquet streamex-$console" >&2
    mkdir -p "$dir" || return 1
    python3 - "$STREAMEX_INDEX" "streamex-$console" "$file" "$dir" <<'PY' >&2 || return 1
import hashlib, json, os, sys, tempfile, urllib.request, zipfile
index_url, image, wanted, folder = sys.argv[1:]
# Un nom à nous : le proxy public de la staging refuse celui de Python (son filtre anti-robots).
fetch = lambda url, timeout: urllib.request.urlopen(urllib.request.Request(url, headers={"User-Agent": "GameFlex-streamex-packages"}), timeout=timeout)
index = json.load(fetch(index_url, 30))
found = [p for p in index["packages"] if p.get("kind", "server") == "server" and p["image"] == image and wanted in p["files"]]
if not found:
    sys.exit(f"ensure_emulator: aucun paquet {image} ne porte {wanted} ({index_url})")
package = found[0]
with tempfile.NamedTemporaryFile(dir=folder, prefix=".streamex-package-", suffix=".zip.part", delete=False) as part:
    digest = hashlib.sha256()
    with fetch(package["url"], 1200) as response:
        for block in iter(lambda: response.read(1 << 20), b""):
            part.write(block)
            digest.update(block)
try:
    if digest.hexdigest() != package["sha256"].lower():
        sys.exit(f"ensure_emulator: {package['file']} ne correspond pas à son index (sha256) : rien d'installé")
    with zipfile.ZipFile(part.name) as archive:
        for name in package["files"]:
            if name != os.path.basename(name) or name in ("", ".", ".."):
                sys.exit(f"ensure_emulator: nom refusé {name!r}")
            target = os.path.join(folder, name)
            if os.path.exists(target):
                continue
            info = archive.getinfo(name)
            with archive.open(info) as source, open(target + ".part", "wb") as out:
                out.write(source.read())
            os.chmod(target + ".part", (info.external_attr >> 16) & 0o777 or 0o644)
            os.replace(target + ".part", target)
            print(f"   → {name}")
finally:
    os.remove(part.name)
PY
    echo "$dir"
}
