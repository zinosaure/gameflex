#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint:
#   PulseAudio (null sink) → X server :2 → openbox → Lichii (game) → streamex
# ════════════════════════════════════════════════════════════════
set -euo pipefail

# ── L'émulateur, depuis la bibliothèque ──
# L'image ne le porte pas (chantier 29) : le kernel monte cores/ en lecture
# seule, et unpack-emulator le déballe là où l'adaptateur le lance.
unpack-emulator /opt/lichii

export DISPLAY="${DISPLAY:-:2}"
export HOME="${HOME:-/root}"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/xdg-runtime}"
mkdir -p "$XDG_RUNTIME_DIR" && chmod 700 "$XDG_RUNTIME_DIR"

export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
export XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"

# ── Aucun dossier d'un autre émulateur de la famille yuzu ──
# Lichii range ses données sous `lichii/`. S'il trouve au démarrage
# $XDG_DATA_HOME/eden (ou citron, sudachi, suyu, yuzu), il ouvre son assistant
# d'import, une fenêtre modale que personne ne peut fermer dans une partie : la
# fenêtre principale ne s'ouvre jamais (mesuré le 2026-11-11, sans GPU). On le
# dit ici, et on s'arrête, plutôt que de laisser la partie figée. Le kernel
# monte les sauvegardes sur $XDG_DATA_HOME/lichii (switchDataTarget) : un tel
# dossier ne vient que d'un montage ou d'une image qui n'est plus la bonne.
for legacy in eden citron sudachi suyu yuzu; do
  if [ -d "$XDG_DATA_HOME/$legacy" ]; then
    echo "[entry] ERROR: $XDG_DATA_HOME/$legacy exists — Lichii would open its import wizard and the game would never start. Lichii keeps its data in $XDG_DATA_HOME/lichii" >&2
    exit 1
  fi
done

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
# The null sink receives Lichii's audio; streamex captures its monitor.
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
# essential for Lichii's Vulkan presentation (otherwise the game won't render).
# Stale X sockets/locks survive `docker compose start` (container reuse).
rm -f "/tmp/.X11-unix/X${DISPLAY#:}" "/tmp/.X${DISPLAY#:}-lock"
RES="${STREAMEX_RESOLUTION:-1280x720}"
RES_W="${RES%x*}"
RES_H="${RES#*x}"
# --renderer=gl: required to expose linux-dmabuf/GBM to Xwayland
# (otherwise glamor fails → no DRI3 → Lichii's Vulkan cannot present).
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
# session has nobody to click it. Lichii looks for updates at every start
# unless told not to: qt-config.ini turns it off (check_for_updates).
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

# ── Config (only if missing, so the host config is not overwritten) ──
# Lichii a ses propres dossiers : ~/.config/lichii, ~/.local/share/lichii.
mkdir -p "$XDG_CONFIG_HOME/lichii"
if [ ! -f "$XDG_CONFIG_HOME/lichii/qt-config.ini" ]; then
  cp /opt/lichii-config/qt-config.ini "$XDG_CONFIG_HOME/lichii/qt-config.ini"
  echo "[entry] seeded default lichii config"
fi

# ── Rendu à la résolution du flux (#340) ──
# Lichii, comme Eden, multiplie la sortie de la console — 1080p en mode docké —
# par `resolution_setup`, un NUMÉRO dans sa liste, celle d'Eden
# (src/common/settings_enums.h au commit 17baed7 : Res1_4X, Res1_2X, Res3_4X,
# Res1X, Res5_4X, Res3_2X, Res2X, Res3X, …).
# On prend le palier le plus proche du flux, et à égalité le plus bas : 360p et
# 540p → ×0,5, 720p → ×0,75, 1080p → ×1. ×0,25 est écarté : Eden ne le
# propose même pas dans ses menus. Sans `\default=false`, Eden ignore la
# valeur et garde son défaut. Le téléphone suit la même règle
# (GlibcGuest.edenResolution), et emuparams.EdenResolution aussi.
RES_SETUP=$(awk -v h="$RES_H" 'BEGIN {
  n = split("1:0.5 2:0.75 3:1 4:1.25 5:1.5 6:2 7:3", steps, " ")
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
LICHII_INI="$XDG_CONFIG_HOME/lichii/qt-config.ini"
awk -v v="$RES_SETUP" '
  /^resolution_setup(\\default)?=/ { next }
  { print }
  /^\[Renderer\]$/ { print "resolution_setup\\default=false"; print "resolution_setup=" v; done = 1 }
  END { if (!done) { print ""; print "[Renderer]"; print "resolution_setup\\default=false"; print "resolution_setup=" v } }
' "$LICHII_INI" > "$LICHII_INI.tmp" && mv "$LICHII_INI.tmp" "$LICHII_INI"
echo "[entry] lichii resolution_setup → ${RES_SETUP} (stream ${RES_W}x${RES_H})"

# ── Decryption keys: bios/switch2/, the console's own folder ──
# Les clés de la Switch 2 (prod.keys, title.keys) se rangent dans bios/switch2/,
# comme celles de la Switch dans bios/switch/ : un seul chemin par console.
KEYS_SRC=/gameflex/bios/switch2
KEYS_DIR="$XDG_DATA_HOME/lichii/keys"
if [ -d "$KEYS_SRC" ] && [ -n "$(ls -A "$KEYS_SRC"/*.keys 2>/dev/null)" ]; then
    mkdir -p "$KEYS_DIR"
    ln -sfn "$KEYS_SRC"/*.keys "$KEYS_DIR" 2>/dev/null || true
    echo "[entry] keys: $(ls "$KEYS_DIR/" | tr '\n' ' ')"
else
    echo "[entry] WARNING: no keys in $KEYS_SRC"
fi

# ── Bus d'accessibilité : prêt AVANT que streamex lance Lichii ──
# Lichii n'écrit sa cadence que dans sa barre d'état, et streamex la lit par
# le bus d'accessibilité. Qt ne l'y inscrit que si org.a11y.Bus existe déjà à
# son démarrage, et ce service ne démarre qu'à la première demande : on la fait
# ici. Sans lui, le panneau n'a pas la cadence, rien d'autre ne change.
dbus-send --session --print-reply --dest=org.a11y.Bus /org/a11y/bus org.a11y.Bus.GetAddress >/dev/null 2>&1 \
  || echo "[entry] WARNING: bus d'accessibilité indisponible — pas de FPS de Lichii"

# ── Lichii: launched by streamex (ConsoleAdapter) — no launch here ──
# The game is started by streamex at boot (GAME_PATH), and stopped with the
# session.

# ── streamex: generic WebRTC streaming server (Go) ──
# Lichii's audio goes through the lichii_sink monitor (system.pa).
export PULSE_SOURCE="${PULSE_SOURCE:-lichii_sink.monitor}"
echo "[entry] starting streamex (session=${SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
