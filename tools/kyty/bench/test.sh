#!/bin/bash
# Essai de notre Kyty (/ours/kyty_emulator) dans l'image PS5 du serveur :
# affichage comme l'entrypoint (Weston headless → Xwayland :2 → openbox), le jeu
# monté en /game (un dossier avec eboot.bin), lancement, captures dans /out.
#   HOLD : secondes de jeu avant l'arrêt
set -u
export DISPLAY=:2 HOME=/root XDG_RUNTIME_DIR=/tmp/xdg-runtime
export XDG_CONFIG_HOME=/root/.config XDG_DATA_HOME=/root/.local/share XDG_CACHE_HOME=/root/.cache
mkdir -p "$XDG_RUNTIME_DIR" "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME" /out
chmod 700 "$XDG_RUNTIME_DIR"
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

GAME=${GAME_PATH:-$(find /game -maxdepth 2 -iname eboot.bin | head -1)}
# BIN : notre construction (/ours), ou le binaire officiel de l'image (témoin).
BIN=${BIN:-/ours/kyty_emulator}
log "jeu : $GAME — binaire : $BIN"
ldd "$BIN" | grep "not found" > /out/ldd-missing.txt
log "bibliothèques manquantes : $(wc -l < /out/ldd-missing.txt)"
mkdir -p /root/.local/share/kyty
export SDL_VIDEODRIVER=x11 SDL_AUDIODRIVER=dummy

log "lancement (HOLD=${HOLD}s)"
# KYTY_ARGS : des options de plus pour kyty_emulator (ex. --printf-direction console,
# pour avoir ses lignes LOGF — « Select device », le chemin barycentrique — dans kyty.log).
# shellcheck disable=SC2086
( cd "$(dirname "$BIN")" && timeout "$HOLD" "$BIN" --fullscreen --game "$GAME" ${KYTY_ARGS:-} > /out/kyty.log 2>&1; echo "[test] exit=$?" >> /out/kyty.log ) &
RUN=$!
prev=0
for t in 20 45 75 110 140; do
    [ "$t" -gt "$HOLD" ] && break
    sleep $((t - prev)); prev=$t
    import -window root "/out/shot-${t}s.png" 2>/dev/null && log "capture ${t}s"
done
wait "$RUN"
cp -r /root/.local/share/kyty /out/kyty-data 2>/dev/null || true
log "fini"
