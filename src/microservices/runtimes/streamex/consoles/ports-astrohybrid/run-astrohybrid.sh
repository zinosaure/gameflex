#!/bin/sh
# LANCER ASTRO BOT PAR ASTROHYBRID (chantier 72, à l'essai sur la dev).
#
#   run-astrohybrid.sh <jeu>        (l'adaptateur passe le dossier du jeu, ou son eboot.bin)
#
# GameFlex ne fournit pas le jeu : il le CONSTRUIT chez le joueur, depuis sa copie. À la première partie (ou quand le
# moteur change), les SELF de la copie sont déballés (tools/unwrap_self.py : ils ne sont pas chiffrés) et reliés en
# programme Linux (tools/relinker) dans le dossier du joueur, APP_FOLDER_PATH, que le kernel monte par compte. Ensuite :
# le dossier de lancement (l'exe, libs/ du moteur, app0/ en liens vers la copie, plus ses modules reliés), et le jeu,
# rendu par la couche GPU de Kyty (APS5_GPU_ENGINE=kyty).
set -eu

game=${1:?usage: run-astrohybrid.sh <jeu>}
[ -f "$game" ] && game=$(dirname "$game")
app=${APP_FOLDER_PATH:?set by the entrypoint}
engine=$(ls -d /opt/astrohybrid/*/ 2>/dev/null | head -n 1)
engine=${engine%/}
[ -x "$engine/tools/relinker" ] || { echo "[astrohybrid] ERROR: engine not unpacked in /opt/astrohybrid" >&2; exit 1; }

# LA VERSION QUE LE PORT CONNAÎT : Astro Bot 01.019.000 (le kernel ne propose que celle-là ; on le redit ici).
param="$game/sce_sys/param.json"
title=$(python3 -I -c 'import json,sys; d=json.load(open(sys.argv[1])); print(d.get("titleId",""), d.get("contentVersion",""))' "$param")
if [ "$title" != "PPSA21567 01.019.000" ]; then
    echo "[astrohybrid] ERROR: $(basename "$game") is not Astro Bot PPSA21567 01.019.000 ($title)" >&2
    exit 1
fi

# ── La construction, une fois par version du moteur ──
stamp=$(sha1sum "$engine/tools/relinker" "$engine/VERSION.txt" | sha1sum | cut -c1-12)
run="$app/run"
mkdir -p "$run"
if [ ! -f "$run/.built-$stamp" ] || [ ! -x "$run/AstroBot.elf" ]; then
    echo "[astrohybrid] building Astro Bot from the player's copy (first launch: a minute or two)"
    work="$app/build"
    rm -rf "$work" "$run"/.built-* "$run/AstroBot.elf" "$run/app0"
    mkdir -p "$work/sce_module"
    python3 -I "$engine/tools/unwrap_self.py" "$game/eboot.bin" "$work/eboot.elf"
    for module in "$game"/sce_module/*.prx; do
        python3 -I "$engine/tools/unwrap_self.py" "$module" "$work/sce_module/$(basename "$module")"
    done
    ( cd "$work" && "$engine/tools/relinker" eboot.elf "$run/AstroBot.elf" )
    rm -rf "$work"
    touch "$run/.built-$stamp"
    echo "[astrohybrid] built"
fi

# ── Le correctif « SoftAssert » d'AstroHybrid (son issue #25, Tools/hybrid/make_run_hybrid.py) ──
# Quand la mémoire des commandes de dessin d'une image déborde (« There are too many objects/batches in this scene »,
# command_buffer_writer.ppr.cpp:31), le moteur du jeu ouvre une boîte d'assertion puis s'arrête par int 0x41/0x42 : Astro
# Bot s'arrêtait en arrivant au premier niveau. Le correctif saute la boîte et les deux pièges ; le jeu suit sa propre voie
# d'échec (le bloc est marqué raté, l'image continue). Cherché par contenu, pas par adresse : le relieur Linux ne place pas
# le code au même endroit que l'exe Windows. Une fois par exe, sans effet ensuite.
python3 -I - "$run/AstroBot.elf" <<'PY'
import sys
path = sys.argv[1]
before = bytes.fromhex("554889e5488d3ded6fbb07488d0dc47fc2074c8d052299e407be1f000000ba0100000031c0e8660eb6ff85c07402cd42488d3d3c54d20731c0e88235fbffcd4131c05dc3")
after = bytes.fromhex("554889e5eb2a3ded6fbb07488d0dc47fc2074c8d052299e407be1f000000ba0100000031c0e8660eb6ff85c07402cd42488d3d3c54d20731c0e88235fbff909031c05dc3")
data = open(path, "rb").read()
if data.count(after) == 1:
    sys.exit(0)
if data.count(before) != 1:
    print("[astrohybrid] WARNING: soft-assert patch not applied (code not found once)", file=sys.stderr)
    sys.exit(0)
open(path, "wb").write(data.replace(before, after))
print("[astrohybrid] soft-assert patch applied")
PY

# ── Le dossier de lancement ──
# app0/ : un lien par entrée de la copie, sauf sce_module/, réel, qui porte les modules reliés à côté des originaux.
mkdir -p "$run/app0/sce_module"
for entry in "$game"/*; do
    name=$(basename "$entry")
    [ "$name" = sce_module ] && continue
    ln -sfn "$entry" "$run/app0/$name"
done
for module in "$game"/sce_module/*; do
    ln -sfn "$module" "$run/app0/sce_module/$(basename "$module")"
done
ln -sfn "$engine/libs" "$run/libs"

# ── Le moteur : le préréglage de Kyty (Jetsku int21), puis ce que pose le lanceur d'AstroHybrid ──
eval "$(python3 -I -c 'import json,shlex,sys; d=json.load(open(sys.argv[1])); print("\n".join(f"export {k}={shlex.quote(str(v))}" for k,v in d.items()))' "$engine/u59-preset.json")"
res=${STREAMEX_RESOLUTION:-1280x720}
export APS5_GPU_ENGINE=kyty \
    KYTY_GPU_HOST_MODULE="$engine/libs/libAstroHybridHost.prx" \
    KYTY_BUFFER_RANGE_FIXES=1 KYTY_HANG_WATCHDOG=0 ASTROHYBRID_COMMIT_EPOCH_FASTPATH=0 \
    ASTROHYBRID_CACHE_TITLE_ID=PPSA21567 APS5_PAD_TRIGGER_STATE=1 APS5_SAVEDATA_TRANSFER_FALLBACK=1 \
    KYTY_GPU_WIDTH="${res%x*}" KYTY_GPU_HEIGHT="${res#*x}" KYTY_GPU_FULLSCREEN=1 \
    ASTROHYBRID_WINDOW_TITLE=AstroHybrid ASTROHYBRID_GAME_SPEED=1
# LES REQUÊTES D'OCCLUSION, VRAIES : le préréglage les rend « toujours visibles » (plus rapide), et le jeu, qui écarte
# ce qui est caché d'après elles, dessine alors tout — au premier niveau, ses commandes de dessin débordaient à chaque
# image (« too many objects/batches », puis l'arrêt de Kyty).
export KYTY_GPU_OCCLUSION=1
# LE CACHE DE SHADERS DE MESA (RADV) DANS LE DOSSIER DU JOUEUR, sur disque, à côté de celui de Kyty (_PipelineCache) et
# des sauvegardes (_sd) : sous ~/.cache, il restait dans le conteneur et se perdait à chaque partie.
export MESA_SHADER_CACHE_DIR="$app/cache/mesa"
mkdir -p "$MESA_SHADER_CACHE_DIR"
# LA LANGUE DE LA CONSOLE, celle de la page qui a lancé (chantier 21) : le kernel la passe en GAMEFLEX_LANGUAGE (« fr »
# ou « en »), et la couche système d'AnyPS5 la lit en ANYPS5_LANGUAGE, un numéro de langue de la console (les mêmes que
# `--console-language` de Kyty, emuparams). Sans elle, le jeu partait toujours en anglais.
case "${GAMEFLEX_LANGUAGE:-}" in
    fr) export ANYPS5_LANGUAGE=2 ;;
    en) export ANYPS5_LANGUAGE=1 ;;
esac

cd "$run"
exec ./AstroBot.elf
