#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint (PS4):
#   PulseAudio (null sink) → X server :2 → openbox → shadPS4 (game) → streamex
# ════════════════════════════════════════════════════════════════
set -euo pipefail

# ── L'émulateur, depuis la bibliothèque ──
# L'image ne le porte pas (chantier 29) : le kernel monte cores/ en lecture
# seule, et unpack-emulator le déballe là où l'adaptateur le lance.
unpack-emulator /opt/shadps4/squashfs-root

export DISPLAY="${DISPLAY:-:2}"
export HOME="${HOME:-/root}"
export XDG_RUNTIME_DIR="${XDG_RUNTIME_DIR:-/tmp/xdg-runtime}"
mkdir -p "$XDG_RUNTIME_DIR" && chmod 700 "$XDG_RUNTIME_DIR"

export XDG_CONFIG_HOME="${XDG_CONFIG_HOME:-$HOME/.config}"
export XDG_DATA_HOME="${XDG_DATA_HOME:-$HOME/.local/share}"
export XDG_CACHE_HOME="${XDG_CACHE_HOME:-$HOME/.cache}"
mkdir -p "$XDG_CONFIG_HOME" "$XDG_DATA_HOME" "$XDG_CACHE_HOME"

# ── D-Bus session bus ──
if command -v dbus-daemon >/dev/null 2>&1 && [ -z "${DBUS_SESSION_BUS_ADDRESS:-}" ]; then
  DBUS_SESSION_BUS_ADDRESS="$(dbus-daemon --session --fork --print-address 2>/dev/null)"
  export DBUS_SESSION_BUS_ADDRESS
fi

# ── D-Bus system bus (required by PulseAudio --system) ──
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
# The null sink receives shadPS4's audio; streamex captures its monitor.
export PULSE_SERVER="${PULSE_SERVER:-unix:/run/pulse/native}"
if ! pactl info >/dev/null 2>&1; then
  pkill pulseaudio 2>/dev/null || true
  sleep 1
  rm -f /run/pulse/pid
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
# Weston headless + Xwayland (DRI3/glamor, required for shadPS4's Vulkan).
# Stale X sockets/locks survive `docker compose start` (container reuse).
rm -f "/tmp/.X11-unix/X${DISPLAY#:}" "/tmp/.X${DISPLAY#:}-lock"
RES="${STREAMEX_RESOLUTION:-1280x720}"
RES_W="${RES%x*}"
RES_H="${RES#*x}"
cat > /tmp/weston.ini <<EOF
[core]
backend=headless-backend.so
renderer=gl
EOF
# --debug : ouvre le protocole de capture d'écran de Weston, refusé sinon
# (« Output capture error: unauthorized »). C'est la SEULE façon de voir ce
# que le COMPOSITEUR affiche, sans passer par X — et donc de distinguer un
# émulateur qui ne dessine rien d'une capture qui lit au mauvais endroit.
# Gardé délibérément : le 2026-09-09 c'est ce cliché qui a tranché la
# question de l'écran noir (le compositeur voyait déjà du noir), après une
# journée passée à soupçonner la capture. Le conteneur est éphémère et son
# seul client Wayland est l'émulateur — le protocole privilégié n'y expose
# personne. Pour l'ouvrir sur une autre console, ajouter le même drapeau.
weston --debug --config=/tmp/weston.ini --socket=wayland-0 --idle-time=0 \
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
for _ in $(seq 1 15); do
  [ -S "/tmp/.X11-unix/X${DISPLAY#:}" ] && break
  sleep 1
done
echo "[entry] X server ready: $(ls /tmp/.X11-unix/ 2>/dev/null | tr '\n' ' ')"

# ── Window manager ──
openbox --sm-disable &
sleep 1

# ── shadPS4 data dirs (writable volume) ──
mkdir -p "$PS4_EXTRACT_DIR"

# ── shadPS4 : le premier jeu d'un compte ne doit pas attendre un clic ──
# Au tout premier démarrage d'un dossier de données, shadPS4 0.18 crée le
# dossier de l'utilisateur 1000, puis demande par une boîte SDL modale s'il faut
# migrer d'anciennes sauvegardes (« Save Migration »). Aucun de ses boutons ne
# répond à Entrée ni à Échap, et personne ne peut cliquer d'ici : le premier
# jeu PS4 d'un compte neuf ne démarrait jamais (2026-09-11).
#
# La question n'est posée que si home/1000 n'existe pas encore
# (UserManager::CreateDefaultUsers) : on le crée avant. Un compte neuf n'a
# aucune ancienne sauvegarde à migrer, et un compte qui a déjà joué a déjà ce
# dossier — mkdir -p n'y touche pas.
SHADPS4_FIRST_USER="$XDG_DATA_HOME/shadPS4/home/1000"
mkdir -p "$SHADPS4_FIRST_USER/savedata" "$SHADPS4_FIRST_USER/trophy" "$SHADPS4_FIRST_USER/inputs"

# ── streamex: generic WebRTC streaming server (Go) ──
# shadPS4's audio goes through the ps4_sink monitor.
export SDL_AUDIODRIVER="${SDL_AUDIODRIVER:-pulseaudio}"
export SDL_VIDEODRIVER="${SDL_VIDEODRIVER:-x11}"
export PULSE_SINK="${PULSE_SINK:-ps4_sink}"
export PULSE_SOURCE="${PULSE_SOURCE:-ps4_sink.monitor}"
echo "[entry] starting streamex (session=${STREAMEX_SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
