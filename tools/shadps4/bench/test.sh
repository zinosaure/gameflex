#!/bin/bash
# Essai de notre shadPS4 (/ours/shadps4) dans l'image PS4 du serveur : affichage
# comme l'entrypoint (Weston headless → Xwayland :2 → openbox), le jeu monté en
# /game (déballé, ou son .pkg, déballé ici), config.json avec le réglage, lancement, captures dans /out.
#   VADDR_GB : user_vaddr_max_in_gbytes (0 = défaut amont, 320 = téléphone)
#   HOLD     : secondes de jeu avant l'arrêt
set -u
export DISPLAY=:2 HOME=/root XDG_RUNTIME_DIR=/tmp/xdg-runtime
export XDG_CONFIG_HOME=/root/.config XDG_DATA_HOME=/root/.local/share XDG_CACHE_HOME=/root/.cache
mkdir -p "$XDG_RUNTIME_DIR" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME" /out
chmod 700 "$XDG_RUNTIME_DIR"
VADDR_GB=${VADDR_GB:-0}
HOLD=${HOLD:-150}
log() { echo "[test] $*" | tee -a /out/test.log; }

rm -f /tmp/.X11-unix/X2 /tmp/.X2-lock
printf '[core]\nbackend=headless-backend.so\nrenderer=gl\n' > /tmp/weston.ini
weston --config=/tmp/weston.ini --socket=wayland-0 --idle-time=0 > /out/weston.log 2>&1 &
sleep 2
WAYLAND_DISPLAY=wayland-0 Xwayland :2 -geometry 1280x720 -ac +extension GLX +extension RANDR > /out/xwayland.log 2>&1 &
for _ in $(seq 1 15); do [ -S /tmp/.X11-unix/X2 ] && break; sleep 1; done
openbox --sm-disable > /dev/null 2>&1 &
sleep 1
log "X prêt ($(ls /tmp/.X11-unix/ | tr '\n' ' '))"

# UN JEU DÉJÀ DÉBALLÉ (son eboot.bin à la racine, tools/ps4-unpack.sh) se lance
# tel quel, en lecture seule ; un .pkg se déballe d'abord ici.
if [ -f /game/eboot.bin ]; then
    GAME=/game
    EBOOT=/game/eboot.bin
else
    GAME=$(ls /game/*.pkg | head -1)
    # pkgextract crée lui-même son dossier de sortie, et refuse s'il existe déjà.
    pkgextract "$GAME" -o /tmp/game > /out/pkgextract.log 2>&1
    EBOOT=$(find /tmp/game -name eboot.bin | head -1)
fi
log "jeu : $(basename "$GAME") → ${EBOOT:-AUCUN EBOOT}"

ldd /ours/shadps4 | grep "not found" > /out/ldd-missing.txt
log "bibliothèques manquantes : $(wc -l < /out/ldd-missing.txt)"

# Comme l'entrypoint : home/1000 avant le premier lancement, sinon la boîte
# modale « Save Migration » que personne ne peut cliquer d'ici.
U="$XDG_DATA_HOME/shadPS4"
mkdir -p "$U/home/1000/savedata" "$U/home/1000/trophy" "$U/home/1000/inputs"
export SDL_VIDEODRIVER=x11 SDL_AUDIODRIVER=dummy

if [ "$VADDR_GB" != "0" ]; then
    # Un premier passage court écrit le config.json complet ; le réglage s'y pose.
    timeout 15 /ours/shadps4 --fullscreen true -g "$EBOOT" > /out/first-run.log 2>&1 || true
    python3 - "$U/config.json" "$VADDR_GB" <<'PY' | tee -a /out/test.log
import json, os, sys
path, gb = sys.argv[1], int(sys.argv[2])
cfg = json.load(open(path)) if os.path.exists(path) else {}
cfg.setdefault("General", {})["user_vaddr_max_in_gbytes"] = gb
json.dump(cfg, open(path, "w"), indent=2)
print("[test] config.json :", "existait" if os.path.exists(path) else "créé", "— user_vaddr_max_in_gbytes =", gb)
PY
    rm -rf "$U/log"
fi

log "lancement (VADDR_GB=$VADDR_GB, HOLD=${HOLD}s)"
( timeout "$HOLD" /ours/shadps4 --fullscreen true -g "$EBOOT" > /out/shadps4.log 2>&1; echo "[test] exit=$?" >> /out/shadps4.log ) &
RUN=$!
prev=0
for t in 20 45 75 110 140; do
    [ "$t" -gt "$HOLD" ] && break
    sleep $((t - prev)); prev=$t
    import -window root "/out/shot-${t}s.png" 2>/dev/null && log "capture ${t}s"
done
wait "$RUN"
cp "$U/log/shad_log.txt" /out/shad_log.txt 2>/dev/null || true
cp "$U/config.json" /out/config.json 2>/dev/null || true
log "fini"
