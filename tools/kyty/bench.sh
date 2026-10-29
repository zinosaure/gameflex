#!/usr/bin/env bash
# Banc d'essai de notre Kyty sur x86_64, dans l'image PS5 du serveur
# (streamex-ps5:latest, plus de quoi capturer l'écran) : le jeu en lecture
# seule, le GPU de l'hôte, deux parcours — notre construction (option compacte)
# et le témoin, le Kyty officiel de l'image —, captures et journaux dans le
# dossier de sortie. Même écran, même journal → l'option ne change rien au jeu.
#
#   tools/kyty/bench.sh <kyty_emulator> <dossier du jeu (eboot.bin)> [sortie] [secondes]
#
# FORCE_NO_BARYCENTRIC=1 : le parcours `compact` seul, avec
# KYTY_NO_FRAGMENT_BARYCENTRIC=1 — le chemin du téléphone (patch 0002 : les
# entrées des pixel shaders interpolées par le matériel, sans gl_BaryCoordKHR)
# sur le GPU de l'hôte, qui a l'extension. Comparer au `compact` d'un banc nu.
# KYTY_ARGS="--printf-direction console" : les lignes LOGF de Kyty (« Select
# device », « Vulkan fragment shader barycentric: … ») dans kyty.log — par défaut
# elles ne vont nulle part.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
BIN=$(readlink -f "${1:?le kyty_emulator construit par build.sh}")
GAME=$(readlink -f "${2:?le dossier du jeu}")
OUT=${3:-$PWD/kyty-bench}
HOLD=${4:-200}
RUNS="compact official"
[ -z "${FORCE_NO_BARYCENTRIC:-}" ] || RUNS=compact

docker build -q -t gameflex-kyty-bench "$HERE/bench" > /dev/null
for run in $RUNS; do
    rm -rf "${OUT:?}/$run"
    mkdir -p "$OUT/$run"
    if [ "$run" = compact ]; then
        mounts=(-v "$BIN:/ours/kyty_emulator:ro"); env=(-e BIN=/ours/kyty_emulator)
        [ -z "${FORCE_NO_BARYCENTRIC:-}" ] || env+=(-e KYTY_NO_FRAGMENT_BARYCENTRIC=1)
    else
        mounts=(); env=(-e BIN=/opt/kyty/kyty_emulator)
    fi
    docker run --rm --name "gameflex-kyty-bench-$run" --device /dev/dri \
        "${mounts[@]}" -v "$GAME:/game:ro" -v "$OUT/$run:/out" "${env[@]}" \
        -e HOLD="$HOLD" -e KYTY_ARGS="${KYTY_ARGS:-}" gameflex-kyty-bench > "$OUT/$run.log" 2>&1 || true
    echo "== $run : $(grep -ac '' "$OUT/$run/kyty.log" 2>/dev/null || echo 0) lignes de journal, captures : $(ls "$OUT/$run"/shot-*.png 2>/dev/null | wc -l), $(grep -a 'exit=' "$OUT/$run/kyty.log" 2>/dev/null | tail -1)"
done
echo "→ $OUT : comparer compact/shot-140s.png et official/shot-140s.png"
