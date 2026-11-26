#!/bin/sh
# L'ASSEMBLAGE DE L'APPDIR, puis l'AppImage (tools/appimage/build.sh le lance dans l'atelier, Dockerfile). Reçoit :
#   /in/gameflex-desktop   le programme du PC (cmd/desktop, construit dans gameflex-retrogaming)
#   /in/web                le bundle d'edge (src/microservices/edge/src/dist)
#   /in/window             la fenêtre (cmd/desktop/window)
#   /in/docs               les licences de GameFlex (public/ : LICENSE, LICENSE-BINARIES.md, les notices des tiers)
#   /out                   où déposer GameFlex-x86_64.AppImage et son .zsync
# et GAMEFLEX_VERSION, OWNER (uid:gid des fichiers rendus), GAMEFLEX_UPDATE_INFORMATION (l'adresse de mise à jour
# inscrite dans l'AppImage ; vide, aucune).
set -eu
: "${GAMEFLEX_VERSION:?}"
app=/tmp/AppDir
rm -rf "$app"
mkdir -p "$app/usr/bin" "$app/usr/share/gameflex" "$app/usr/lib/gameflex" "$app/usr/share/doc/ffmpeg" \
    "$app/usr/share/icons/hicolor/512x512/apps" "$app/usr/share/applications"

install -m 755 /in/gameflex-desktop "$app/usr/bin/gameflex-desktop"
install -m 755 /opt/ffmpeg/bin/ffmpeg "$app/usr/bin/ffmpeg"
cp /opt/ffmpeg/SOURCE.txt "$app/usr/share/doc/ffmpeg/SOURCE.txt"
# L'écran des consoles récentes : Xvfb, et ses bibliothèques à part (AppRun les désigne au programme, qui ne les donne
# qu'à lui).
install -m 755 /opt/xvfb/bin/Xvfb "$app/usr/bin/Xvfb"
mkdir -p "$app/usr/lib/gameflex/xvfb" "$app/usr/share/doc/xvfb"
cp /opt/xvfb/lib/* "$app/usr/lib/gameflex/xvfb/"
cp /opt/xvfb/SOURCE.txt "$app/usr/share/doc/xvfb/SOURCE.txt"
# Le son des consoles récentes : le démon PulseAudio privé, ses modules et ses bibliothèques à part.
mkdir -p "$app/usr/lib/gameflex/pulse" "$app/usr/share/doc/pulseaudio"
cp -a /opt/pulse/bin /opt/pulse/lib /opt/pulse/modules "$app/usr/lib/gameflex/pulse/"
cp /opt/pulse/SOURCE.txt "$app/usr/share/doc/pulseaudio/SOURCE.txt"
# LES LICENCES VOYAGENT AVEC L'APPIMAGE, comme avec les images et l'APK : usr/share/doc/gameflex/.
mkdir -p "$app/usr/share/doc/gameflex"
cp /in/docs/LICENSE /in/docs/LICENSE-BINARIES.md /in/docs/THIRD-PARTY-NOTICES.md /in/docs/THIRD-PARTY-LICENSES.txt "$app/usr/share/doc/gameflex/"
cp -a /in/web "$app/usr/share/gameflex/web"
mkdir -p "$app/usr/share/gameflex/window"
cp /in/window/main.js /in/window/package.json "$app/usr/share/gameflex/window/"
# Electron tel que publié (son LICENSE et LICENSES.chromium.html avec lui). Son chrome-sandbox ne sert pas : la fenêtre
# part sans le bac à sable de Chromium (window.go dit pourquoi).
cp -a /opt/electron "$app/usr/lib/gameflex/electron"
rm -f "$app/usr/lib/gameflex/electron/chrome-sandbox"

install -m 755 /opt/atelier/AppRun "$app/AppRun"
cp /opt/atelier/gameflex.desktop "$app/gameflex.desktop"
cp /opt/atelier/gameflex.desktop "$app/usr/share/applications/gameflex.desktop"
cp /in/web/images/icon-512.png "$app/gameflex.png"
cp /in/web/images/icon-512.png "$app/usr/share/icons/hicolor/512x512/apps/gameflex.png"
desktop-file-validate "$app/gameflex.desktop"

# LA MISE À JOUR PAR DIFFÉRENCE (zsync) : l'adresse inscrite dans l'AppImage, que les outils des distributions lisent
# (AppImageUpdate, Gear Lever), et le .zsync rendu à côté d'elle, à publier avec elle. Le nom de l'AppImage est sans
# version : c'est celui de la Release (`releases/latest/download/GameFlex-x86_64.AppImage`), que le .zsync désigne.
out=/out/GameFlex-x86_64.AppImage
rm -f "$out" "$out.zsync"
set -- --no-appstream --runtime-file /opt/runtime-x86_64
if [ -n "${GAMEFLEX_UPDATE_INFORMATION:-}" ]; then
    set -- "$@" --updateinformation "$GAMEFLEX_UPDATE_INFORMATION" --file-url GameFlex-x86_64.AppImage
fi
# appimagetool écrit le .zsync dans le dossier courant.
cd /out
ARCH=x86_64 /opt/appimagetool/AppRun "$@" "$app" "$out"
[ -z "${OWNER:-}" ] || chown "$OWNER" "$out" $( [ ! -f "$out.zsync" ] || echo "$out.zsync" )
ls -l "$out"*
