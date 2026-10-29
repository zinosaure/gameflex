#!/usr/bin/env bash
# Banc d'essai de notre shadPS4 sur x86_64, dans l'image PS4 du serveur
# (streamex-ps4:latest, plus de quoi capturer l'écran) : le jeu en lecture
# seule, le GPU de l'hôte, deux parcours — la plage ramenée à 256 Gio (celle
# d'un téléphone) et le témoin à la mise en page de l'éditeur —, captures et
# journaux dans le dossier de sortie. Le verdict se lit en comparant les deux :
# même écran, même journal → le réglage ne change rien au jeu.
#
#   tools/shadps4/bench.sh <shadps4> <dossier du jeu : déballé, ou son .pkg> [sortie] [secondes]
#
# RUNS : les plages essayées, en Gio (défaut « 320 0 » : le téléphone, puis
# l'éditeur) — « 256 320 » compare deux plafonds.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
BIN=$(readlink -f "${1:?le binaire shadps4 construit par build.sh}")
GAME=$(readlink -f "${2:?le dossier du jeu (déballé, ou son .pkg)}")
OUT=${3:-$PWD/shadps4-bench}
HOLD=${4:-150}

docker build -q -t gameflex-shadps4-bench "$HERE/bench" > /dev/null
for run in ${RUNS:-320 0}; do
    rm -rf "${OUT:?}/$run"
    mkdir -p "$OUT/$run"
    docker run --rm --name "gameflex-shadps4-bench-$run" --device /dev/dri \
        -v "$BIN:/ours/shadps4:ro" -v "$GAME:/game:ro" -v "$OUT/$run:/out" \
        -e VADDR_GB="$run" -e HOLD="$HOLD" gameflex-shadps4-bench > "$OUT/$run.log" 2>&1 || true
    region=$(grep -ah "User virtual memory region" "$OUT/$run/shad_log.txt" 2>/dev/null | sed 's/.*region: //' | head -1)
    echo "== plage ${run} (0 = éditeur) : ${region:-pas de journal} — captures : $(ls "$OUT/$run"/shot-*.png 2>/dev/null | wc -l), journal : $(grep -ac '' "$OUT/$run/shad_log.txt" 2>/dev/null || echo 0) lignes"
done
echo "→ $OUT : comparer les shot-140s.png de chaque plage"
