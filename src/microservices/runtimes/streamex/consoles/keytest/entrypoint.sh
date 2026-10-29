#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint (keytest console):
#   PulseAudio (null sink) → X server :2 → openbox → keytest display → streamex
#
#  Same pipeline as the other consoles, except the "emulator" slot is the
#  keytest display app (keytest/display.py), started by streamex itself
#  through the KeyTest adapter at boot (GAME_PATH is a placeholder).
# ════════════════════════════════════════════════════════════════
set -euo pipefail

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
fi

# ── PulseAudio in system mode (we run as root) ──
export PULSE_SERVER="${PULSE_SERVER:-unix:/run/pulse/native}"
if ! pactl info >/dev/null 2>&1; then
  pkill pulseaudio 2>/dev/null || true
  sleep 1
  rm -f /run/pulse/pid
  env -u XDG_CONFIG_HOME -u XDG_DATA_HOME -u XDG_CACHE_HOME \
    pulseaudio --system --disallow-exit --exit-idle-time=-1 -n -F /opt/pulse/system.pa >/tmp/pulse.log 2>&1 &
  for _ in $(seq 1 15); do
    [ -S /run/pulse/native ] && break
    sleep 1
  done
fi

# ── Virtual X server (Weston headless + Xwayland, DRI3) ──
rm -f "/tmp/.X11-unix/X${DISPLAY#:}" "/tmp/.X${DISPLAY#:}-lock"
RES="${STREAMEX_RESOLUTION:-1280x720}"
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
WAYLAND_DISPLAY=wayland-0 Xwayland "${DISPLAY}" -geometry "$RES" -ac +extension GLX +extension RANDR \
  >/tmp/xwayland.log 2>&1 &
XSERVER_PID=$!
trap 'kill "$XSERVER_PID" "$WESTON_PID" 2>/dev/null || true' EXIT TERM INT
for _ in $(seq 1 15); do
  [ -S "/tmp/.X11-unix/X${DISPLAY#:}" ] && break
  sleep 1
done

# ── Window manager ──
openbox --sm-disable &
sleep 1

# ── streamex: generic WebRTC streaming server (Go) ──
# The KeyTest adapter (CONSOLE=keytest) launches keytest/display.py on
# GAME_PATH at boot, then forwards every input event to it.
export PULSE_SOURCE="${PULSE_SOURCE:-keytest_sink.monitor}"
echo "[entry] starting streamex (session=${SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
