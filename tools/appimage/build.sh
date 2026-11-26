#!/usr/bin/env bash
# L'APPIMAGE DE GAMEFLEX (chantier 70) : le programme du PC, le bundle d'edge, la fenêtre (Electron), ffmpeg — un seul
# fichier, rendu dans `dist/appimage/` avec ce qui se publie à côté de lui :
#   GameFlex-x86_64.AppImage        l'AppImage ;
#   GameFlex-x86_64.AppImage.zsync  sa mise à jour par différence (AppImageUpdate, Gear Lever) ;
#   gameflex-desktop.json           sa fiche (version, date, taille, SHA-256) : la console d'une AppImage installée la
#                                   lit pour se mettre à jour (update.go).
#
#   tools/appimage/build.sh
#
# LE PROGRAMME DU PC : construit depuis ses sources quand elles sont là (le dépôt de GameFlex), sinon pris dans son
# archive publiée — `GAMEFLEX_DESKTOP_BUNDLE`, le chemin de `gameflex-desktop-x86_64.tar.gz` téléchargé de la Release
# de la même version (le programme et sa fenêtre, sous LICENSE-BINARIES.md). C'est ainsi qu'on construit sa propre
# AppImage depuis le dépôt public : tout le reste s'y trouve.
#
# Le bundle d'edge doit être construit pour cette version (`npm run build` dans edge : build-info.json porte la version
# de VERSION) — publish.sh le fait pour l'APK juste avant. Rien ne s'installe sur l'hôte : le programme se construit
# dans gameflex-retrogaming (cgo, la glibc de Debian 12), l'AppImage dans l'atelier (Dockerfile).
#
# `GAMEFLEX_UPDATE_INFORMATION` : l'adresse de mise à jour inscrite dans l'AppImage ; par défaut, la Release du dépôt
# public. Une AppImage construite ailleurs la remplace par la sienne, ou la vide (aucune).
set -euo pipefail
cd "$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
update_information=${GAMEFLEX_UPDATE_INFORMATION-"gh-releases-zsync|zinosaure|gameflex|latest|GameFlex-x86_64.AppImage.zsync"}

fail() {
    echo "✗ $*" >&2
    exit 1
}

# La version : celle de VERSION (le dépôt de GameFlex), sinon GAMEFLEX_VERSION — celle de l'archive du programme et du
# bundle d'edge (construit avec la même GAMEFLEX_VERSION dans l'environnement).
if [ -f VERSION ]; then
    version=$(tr -d '[:space:]' < VERSION)
else
    version=${GAMEFLEX_VERSION:?la version à construire : GAMEFLEX_VERSION=x.y.z, celle de l\'archive du programme}
fi
# Les licences de GameFlex : public/ ici, la racine du dépôt public.
docs=public
[ -f "$docs/LICENSE-BINARIES.md" ] || docs=.
[ -f "$docs/LICENSE-BINARIES.md" ] || fail "LICENSE-BINARIES.md manque"
web=src/microservices/edge/src/dist
[ -f "$web/index.html" ] || fail "le bundle d'edge manque ($web) : npm run build dans gameflex-edge"
built=$(sed -n 's/.*"version":"\([^"]*\)".*/\1/p' "$web/build-info.json" 2>/dev/null || true)
[ "$built" = "$version" ] || fail "le bundle d'edge est celui de « $built », pas de $version : le reconstruire"

out=dist/appimage
mkdir -p "$out/program"
if [ -d src/mobileapp/kernel/cmd/desktop ]; then
    echo "→ le programme du PC ($version), depuis ses sources"
    docker run --rm --entrypoint sh -e CGO_ENABLED=1 -e GOTOOLCHAIN=auto -e GOFLAGS=-buildvcs=false \
        -v gameflex-gomod-cache:/go/pkg/mod -v "$PWD/src/mobileapp:/workspace" -v "$PWD/src/microservices/shared:/shared" \
        -v "$PWD/src/microservices/kernel/extras/credential/src:/credential" -v "$PWD/$out/program:/program" \
        -w /workspace/kernel gameflex-retrogaming:latest \
        -c "export PATH=\$PATH:/usr/local/go/bin; go build -trimpath -ldflags='-s -w -checklinkname=0 -X main.version=$version' \
            -o /program/gameflex-desktop ./cmd/desktop && chown -R $(id -u):$(id -g) /program"
    rm -rf "${out:?}/program/window"
    mkdir -p "$out/program/window"
    cp src/mobileapp/kernel/cmd/desktop/window/main.js src/mobileapp/kernel/cmd/desktop/window/package.json "$out/program/window/"
else
    bundle=${GAMEFLEX_DESKTOP_BUNDLE:-}
    [ -f "$bundle" ] || fail "le programme du PC n'est pas dans ce dépôt : GAMEFLEX_DESKTOP_BUNDLE=<gameflex-desktop-x86_64.tar.gz de la Release $version>"
    echo "→ le programme du PC ($version), depuis $bundle"
    rm -rf "${out:?}/program"
    mkdir -p "$out/program"
    tar -xzf "$bundle" -C "$out/program"
    [ -x "$out/program/gameflex-desktop" ] && [ -f "$out/program/window/main.js" ] || fail "$bundle n'est pas l'archive du programme du PC"
fi

echo "→ l'atelier (ffmpeg, Electron, appimagetool)"
docker build -q -t gameflex-appimage:latest tools/appimage >/dev/null

echo "→ l'AppImage"
docker run --rm -e GAMEFLEX_VERSION="$version" -e OWNER="$(id -u):$(id -g)" -e GAMEFLEX_UPDATE_INFORMATION="$update_information" \
    -v "$PWD/$out/program/gameflex-desktop:/in/gameflex-desktop:ro" -v "$PWD/$web:/in/web:ro" \
    -v "$PWD/$out/program/window:/in/window:ro" -v "$PWD/$docs:/in/docs:ro" -v "$PWD/$out:/out" \
    gameflex-appimage:latest

echo "→ sa fiche (gameflex-desktop.json)"
python3 - "$out/GameFlex-x86_64.AppImage" "$version" > "$out/gameflex-desktop.json" <<'PY'
import datetime, hashlib, json, os, sys
path, version = sys.argv[1], sys.argv[2]
digest = hashlib.sha256()
with open(path, "rb") as image:
    for block in iter(lambda: image.read(1 << 20), b""):
        digest.update(block)
print(json.dumps({"version": version, "builtAt": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
                  "size": os.path.getsize(path), "sha256": digest.hexdigest(), "file": os.path.basename(path)}, indent=2))
PY
ls -l "$out"
