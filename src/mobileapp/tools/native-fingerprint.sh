#!/usr/bin/env bash
# L'EMPREINTE « NATIVE » DE L'APK (chantier 64, `#786`) : ce que l'interface appelle et que seule une APK change — le
# kernel du téléphone et le Go qu'il embarque (gatewaywebrtc, shared, le service d'identité), le Java de l'application
# et son manifeste. Une interface téléchargée ne s'installe depuis la source officielle que si son empreinte est celle
# de l'APK installée : un changement d'interface seule passe sans APK, un changement qui touche aussi le natif en
# demande une. Aucun numéro à tenir à la main.
#
#   native-fingerprint.sh <dossier mobileapp> <dossier shared> <dossier du service d'identité (src)>
#
# Les mêmes fichiers donnent la même empreinte, où qu'ils soient montés : les chemins entrent dans le calcul sous un
# nom de rôle (kernel/…, shared/…), jamais sous leur chemin réel. Les tests n'y entrent pas : ils ne changent rien à
# ce que l'APK exécute.
set -euo pipefail

mobile=${1:?dossier mobileapp}
shared=${2:?dossier shared}
credential=${3:?dossier du service d\'identité}

list() {
    local role=$1 dir=$2
    shift 2
    (cd "$dir" && find . -type f \( "$@" \) ! -name '*_test.go' ! -path '*/build/*' ! -path '*/testdata/*' -print0 |
        LC_ALL=C sort -z | xargs -0 -r sha256sum | sed "s# \./# ${role}/#")
}

{
    list kernel "$mobile/kernel" -name '*.go' -o -name 'go.mod' -o -name 'go.sum'
    list gatewaywebrtc "$mobile/gatewaywebrtc" -name '*.go' -o -name 'go.mod' -o -name 'go.sum'
    list shared "$shared" -name '*.go' -o -name 'go.mod' -o -name 'go.sum'
    list credential "$credential" -name '*.go' -o -name 'go.mod' -o -name 'go.sum'
    list java "$mobile/app/src/main/java" -name '*.java'
    list manifest "$mobile/app/src/main" -maxdepth 1 -name 'AndroidManifest.xml'
} | sha256sum | cut -c1-16
