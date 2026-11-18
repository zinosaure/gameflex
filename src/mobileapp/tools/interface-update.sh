#!/usr/bin/env bash
# PUBLIER L'INTERFACE SEULE, SANS APK (chantier 64, `#786`) — dans les versions d'edge du serveur de dev
# (/downloads/edge-versions/, celles de l'APK staging) : la page d'edge construite (`npm run build`) et ses licences,
# empaquetées (interface-package.py), ajoutées à l'index. L'écran « Mise à jour » du téléphone la propose ensuite, si
# l'APK installée a la même empreinte native ; sinon il faut une APK.
#
#   src/mobileapp/tools/interface-update.sh
#
# La Release `edge-versions` du dépôt public reçoit les siennes de tools/publish.sh.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/../../.." && pwd)
EDGE=$ROOT/src/microservices/edge/src
VERSIONS=$EDGE/public/downloads/edge-versions
[ -f "$EDGE/dist/build-info.json" ] || { echo "✗ pas de build d'edge : docker exec gameflex-edge sh -c 'cd /src && npm run build'" >&2; exit 1; }
NEWER=$(find "$EDGE/app" -type f -newer "$EDGE/dist/bundle.js" -print -quit)
[ -z "$NEWER" ] || { echo "✗ le build d'edge est plus vieux que $NEWER : reconstruis-le" >&2; exit 1; }

WEB=$(mktemp -d)
trap 'rm -rf "${WEB:?}"' EXIT
cp -r "$EDGE/dist/." "$WEB/"
mkdir -p "$WEB/licenses"
cp "$ROOT/public/LICENSE" "$ROOT/public/THIRD-PARTY-NOTICES.md" "$ROOT/public/THIRD-PARTY-LICENSES.txt" "$WEB/licenses/"

NATIVE=$("$ROOT/src/mobileapp/tools/native-fingerprint.sh" "$ROOT/src/mobileapp" "$ROOT/src/microservices/shared" \
    "$ROOT/src/microservices/kernel/extras/credential/src")
mkdir -p "$VERSIONS"
entry=$(python3 "$ROOT/src/mobileapp/tools/interface-package.py" "$WEB" "$VERSIONS" "$NATIVE")
file=$(python3 -c 'import json,sys; print(json.loads(sys.argv[1])["file"])' "$entry")
echo "→ $file dans $VERSIONS (empreinte native $NATIVE)"
