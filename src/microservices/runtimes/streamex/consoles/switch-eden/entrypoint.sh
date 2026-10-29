#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint:
#   PulseAudio (null sink) → X server :2 → openbox → Eden (game) → streamex
# ════════════════════════════════════════════════════════════════
set -euo pipefail

# ── L'émulateur, depuis la bibliothèque ──
# L'image ne le porte pas (chantier 29) : le kernel monte cores/ en lecture
# seule, et unpack-emulator le déballe là où l'adaptateur le lance.
unpack-emulator /opt/eden

export DISPLAY="${DISPLAY:-:2}"
export HOME="${HOME:-/root}"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/xdg-runtime}"
mkdir -p "$XDG_RUNTIME_DIR" && chmod 700 "$XDG_RUNTIME_DIR"

export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
export XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"

# ── D-Bus session bus (Qt likes it) ──
if command -v dbus-daemon >/dev/null 2>&1 && [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
  DBUS_SESSION_BUS_ADDRESS="$(dbus-daemon --session --fork --print-address 2>/dev/null)"
  export DBUS_SESSION_BUS_ADDRESS
fi

# ── D-Bus system bus (required by PulseAudio --system) ──
# /run/dbus must belong to messagebus, otherwise dbus-daemon cannot
# create its socket and PulseAudio stays stuck at startup.
# A STALE socket survives `docker compose start` (container reuse) — remove
# it: its daemon is long gone.
if [ -S /run/dbus/system_bus_socket ] && ! dbus-send --system --print-reply --dest=org.freedesktop.DBus /org/freedesktop/DBus org.freedesktop.DBus.ListNames >/dev/null 2>&1; then
  rm -f /run/dbus/system_bus_socket /run/dbus/pid
fi
if [ ! -S /run/dbus/system_bus_socket ]; then
  mkdir -p /run/dbus
  chown messagebus:messagebus /run/dbus 2>/dev/null || true
  [ -f /var/lib/dbus/machine-id ] || dbus-uuidgen --ensure=/var/lib/dbus/machine-id 2>/dev/null || true
  dbus-daemon --system --fork 2>/dev/null || true
  for _ in $(seq 1 5); do
    [ -S /run/dbus/system_bus_socket ] && break
    sleep 1
  done
  # Second attempt if the socket is still missing (stale pid)
  if [ ! -S /run/dbus/system_bus_socket ]; then
    rm -f /run/dbus/pid
    dbus-daemon --system --fork 2>/dev/null || true
    for _ in $(seq 1 5); do
      [ -S /run/dbus/system_bus_socket ] && break
      sleep 1
    done
  fi
fi

# ── PulseAudio in system mode (we run as root) ──
# The null sink receives Eden's audio; streamex captures its monitor.
# Dedicated config (auth-anonymous) loaded via -n -F: avoids the
# $HOME/.config/pulse/cookie that the daemon (user pulse) cannot read.
export PULSE_SERVER="${PULSE_SERVER:-unix:/run/pulse/native}"
if ! pactl info >/dev/null 2>&1; then
  pkill pulseaudio 2>/dev/null || true
  sleep 1
  rm -f /run/pulse/pid
  # env -u XDG_*: otherwise XDG_CONFIG_HOME=/root/.config forces the cookie
  # path to /root (unreadable for the pulse user) and the socket module fails.
  env -u XDG_CONFIG_HOME -u XDG_DATA_HOME -u XDG_CACHE_HOME \
    pulseaudio --system --disallow-exit --exit-idle-time=-1 -n -F /opt/pulse/system.pa >/tmp/pulse.log 2>&1 &
  PULSE_PID=$!
  for _ in $(seq 1 15); do
    [ -S /run/pulse/native ] && break
    sleep 1
  done
  if [ ! -S /run/pulse/native ]; then
    echo "[entry] WARNING: pulseaudio socket missing after 15s (pid=$PULSE_PID)"
  fi
fi
echo "[entry] pulse default sink: $(pactl get-default-sink 2>/dev/null || echo 'N/A')"

# ── Virtual X server ──
# Weston headless + Xwayland: unlike Xvfb, Xwayland supports DRI3,
# essential for Eden's Vulkan presentation (otherwise the game won't render).
# Stale X sockets/locks survive `docker compose start` (container reuse).
rm -f "/tmp/.X11-unix/X${DISPLAY#:}" "/tmp/.X${DISPLAY#:}-lock"
RES="${STREAMEX_RESOLUTION:-1280x720}"
RES_W="${RES%x*}"
RES_H="${RES#*x}"
# --renderer=gl: required to expose linux-dmabuf/GBM to Xwayland
# (otherwise glamor fails → no DRI3 → Eden's Vulkan cannot present).
# Headless backend mode syntax: mode=WxH (width/height are invalid here).
cat > /tmp/weston.ini <<EOF
[core]
backend=headless-backend.so
renderer=gl
EOF
weston --config=/tmp/weston.ini --socket=wayland-0 --idle-time=0 \
  >/tmp/weston.log 2>&1 &
WESTON_PID=$!
sleep 2
# LA TAILLE DE L'ÉCRAN X SE DONNE À XWAYLAND LUI-MÊME (-geometry, #344). Elle
# se réglait après coup par xrandr, qui ne connaît que la liste de modes de
# Xwayland 24.1 : ni 960x540 ni 640x360 n'y sont. En 540p — le réglage par
# défaut — l'écran restait donc en 640x480, et le flux partait en 4/3, après
# dix essais d'une seconde. -geometry prend n'importe quelle taille, et openbox
# ne la touche pas (vérifié le 2026-10-12).
WAYLAND_DISPLAY=wayland-0 Xwayland "${DISPLAY}" -geometry "${RES_W}x${RES_H}" -ac +extension GLX +extension RANDR \
  >/tmp/xwayland.log 2>&1 &
XSERVER_PID=$!
trap 'kill "$XSERVER_PID" "$WESTON_PID" 2>/dev/null || true' EXIT TERM INT
# Wait until the X socket is actually ready (otherwise the emulator crashes at boot)
for _ in $(seq 1 15); do
  [ -S "/tmp/.X11-unix/X${DISPLAY#:}" ] && break
  sleep 1
done
echo "[entry] X server ready: $(ls /tmp/.X11-unix/ 2>/dev/null | tr '\n' ' ')"

# ── Window manager ──
openbox --sm-disable &
sleep 1

# ── Auto-dismiss an "Update Available" popup (headless) ──
# Closed automatically (Return = OK) should the emulator ever show one: a
# session has nobody to click it.
# `|| true` on the search: xdotool exits 1 when nothing matches and set -e
# would otherwise kill this subshell on its very first iteration.
( for _ in $(seq 1 600); do
    wid="$(DISPLAY="$DISPLAY" xdotool search --name "Update Available" 2>/dev/null | head -1 || true)"
    if [ -n "$wid" ]; then
      DISPLAY="$DISPLAY" xdotool windowactivate --sync "$wid" 2>/dev/null || true
      DISPLAY="$DISPLAY" xdotool key --window "$wid" Return 2>/dev/null || true
    fi
    sleep 3
  done ) &

# ── Eden config (only if missing, so the host config is not overwritten) ──
mkdir -p "$XDG_CONFIG_HOME/eden"
if [ ! -f "$XDG_CONFIG_HOME/eden/qt-config.ini" ]; then
  cp /opt/eden-config/qt-config.ini "$XDG_CONFIG_HOME/eden/qt-config.ini"
  echo "[entry] seeded default eden config"
fi

# ── Rendu à la résolution du flux (#340) ──
# Eden multiplie la sortie de la console — 1080p en mode docké, son défaut —
# par `resolution_setup`, un NUMÉRO dans sa liste (src/common/settings_enums.h :
# Res1_4X, Res1_2X, Res3_4X, Res1X, Res3_2X, Res2X, Res3X, …, Res5_4X, Res7_4X).
# On prend le palier le plus proche du flux, et à égalité le plus bas : 360p et
# 540p → ×0,5, 720p → ×0,75, 1080p → ×1. ×0,25 est écarté : Eden ne le
# propose même pas dans ses menus. Sans `\default=false`, Eden ignore la
# valeur et garde son défaut. Le téléphone suit la même règle
# (GlibcGuest.edenResolution).
RES_SETUP=$(awk -v h="$RES_H" 'BEGIN {
  n = split("1:0.5 2:0.75 3:1 12:1.25 4:1.5 13:1.75 5:2 6:3", steps, " ")
  best = 3; bestDistance = -1; bestHeight = 0
  for (i = 1; i <= n; i++) {
    split(steps[i], step, ":"); height = step[2] * 1080; distance = height - h
    if (distance < 0) distance = -distance
    if (bestDistance < 0 || distance < bestDistance || (distance == bestDistance && height < bestHeight)) {
      best = step[1]; bestDistance = distance; bestHeight = height
    }
  }
  print best
}')
EDEN_INI="$XDG_CONFIG_HOME/eden/qt-config.ini"
awk -v v="$RES_SETUP" '
  /^resolution_setup(\\default)?=/ { next }
  { print }
  /^\[Renderer\]$/ { print "resolution_setup\\default=false"; print "resolution_setup=" v; done = 1 }
  END { if (!done) { print ""; print "[Renderer]"; print "resolution_setup\\default=false"; print "resolution_setup=" v } }
' "$EDEN_INI" > "$EDEN_INI.tmp" && mv "$EDEN_INI.tmp" "$EDEN_INI"
echo "[entry] eden resolution_setup → ${RES_SETUP} (stream ${RES_W}x${RES_H})"

# ── Decryption keys (from the shared cold-storage root /gameflex) ──
# Kernel mounts the whole cold root like the retro children — the keys
# subpath is this console's own business, not the platform's.
# LE DOSSIER DE LA CONSOLE, PAS UNE VERSION DEDANS.
#
# C'était `/gameflex/bios/switch/2026` — un sous-dossier nommé d'après le jeu
# de clés. Deux choses le condamnaient : le nom est arbitraire (renommer le
# dossier cassait la console sans rien dire), et la surface fédérée sert les
# BIOS PAR NOM DE FICHIER — un téléphone qui demandait « prod.keys » ne
# trouvait rien, alors que le serveur les avait (2026-09-08).
#
# Un seul chemin, celui de toutes les autres consoles : `bios/<console>/`.
# Pas de repli sur un sous-dossier — permettre les deux, c'est garantir qu'on
# trouvera les deux, et un dossier de BIOS a déjà un sens ailleurs (un dump
# PS2 est un dossier avec ses fichiers compagnons).
KEYS_SRC=/gameflex/bios/switch
KEYS_DIR="$XDG_DATA_HOME/eden/keys"
if [ -d "$KEYS_SRC" ] && [ -n "$(ls -A "$KEYS_SRC"/*.keys 2>/dev/null)" ]; then
    mkdir -p "$KEYS_DIR"
    ln -sfn "$KEYS_SRC"/*.keys "$KEYS_DIR" 2>/dev/null || true
    echo "[entry] keys: $(ls "$KEYS_DIR/" | tr '\n' ' ')"
else
    echo "[entry] WARNING: no keys in $KEYS_SRC"
fi

# ── Les sauvegardes faites par Citron (chantier 25) ──
# Citron nommait `.citron_save_size` le fichier de taille qu'yuzu, et Eden,
# nomment `.yuzu_save_size`. Eden ne reconnaît pas le premier : un dossier de
# profil qui ne contient que lui passe pour ORPHELIN, et Eden ouvre à son
# démarrage une fenêtre bloquante que personne ne peut fermer dans une partie
# (« Orphaned Profiles Detected! », QtCommon::Content::FixProfiles). Renommé
# une fois, il ne revient plus.
SAVES="$XDG_DATA_HOME/eden/nand/user/save"
if [ -d "$SAVES" ]; then
  renamed=0
  while IFS= read -r size_file; do
    mv -f "$size_file" "$(dirname "$size_file")/.yuzu_save_size" && renamed=$((renamed + 1))
  done < <(find "$SAVES" -type f -name .citron_save_size)
  [ "$renamed" -eq 0 ] || echo "[entry] sauvegardes de Citron : ${renamed} .citron_save_size → .yuzu_save_size"
fi

# ── Bus d'accessibilité : prêt AVANT que streamex lance Eden ──
# Eden n'écrit sa cadence que dans sa barre d'état, et streamex la lit par le
# bus d'accessibilité. Qt n'y inscrit Eden que si org.a11y.Bus existe déjà à
# son démarrage, et ce service ne démarre qu'à la première demande : on la fait
# ici. Sans lui, le panneau n'a pas les FPS d'Eden, rien d'autre ne change.
dbus-send --session --print-reply --dest=org.a11y.Bus /org/a11y/bus org.a11y.Bus.GetAddress >/dev/null 2>&1 \
  || echo "[entry] WARNING: bus d'accessibilité indisponible — pas de FPS d'Eden"

# ── Eden: launched by streamex (ConsoleAdapter) — no launch here ──
# The game is started by streamex at boot (GAME_PATH), and stopped with the
# session.

# ── streamex: generic WebRTC streaming server (Go) ──
# Eden's audio goes through the eden_sink monitor.
export PULSE_SOURCE="${PULSE_SOURCE:-eden_sink.monitor}"
echo "[entry] starting streamex (session=${SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
