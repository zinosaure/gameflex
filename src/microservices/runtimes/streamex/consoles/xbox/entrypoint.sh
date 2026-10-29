#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint (Xbox):
#   PulseAudio (null sink) → X server :2 → openbox → xemu (game) → streamex
# ════════════════════════════════════════════════════════════════
set -euo pipefail

# ── L'émulateur, depuis la bibliothèque ──
# L'image ne le porte pas (chantier 29) : le kernel monte cores/ en lecture
# seule, et unpack-emulator le déballe là où l'adaptateur le lance.
unpack-emulator /opt/xemu/squashfs-root

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
# The null sink receives xemu's audio; streamex captures its monitor.
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

# ── Virtual X server (weston headless + Xwayland : DRI3/glamor) ──
# Stale X sockets/locks survive `docker compose start` (container reuse) and
# make Xwayland abort with "Server is already active for display 2".
rm -f "/tmp/.X11-unix/X${DISPLAY#:}" "/tmp/.X${DISPLAY#:}-lock"
RES="${STREAMEX_RESOLUTION:-1280x720}"
RES_W="${RES%x*}"
RES_H="${RES#*x}"
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
for _ in $(seq 1 15); do
  [ -S "/tmp/.X11-unix/X${DISPLAY#:}" ] && break
  sleep 1
done
echo "[entry] X server ready: $(ls /tmp/.X11-unix/ 2>/dev/null | tr '\n' ' ')"

# ── Window manager ──
openbox --sm-disable &
sleep 1

# ── xemu config (seed only if missing, so the host config is not overwritten) ──
mkdir -p "$XDG_DATA_HOME/xemu/xemu"
if [ ! -f "$XDG_DATA_HOME/xemu/xemu/xemu.toml" ]; then
  cp /opt/xemu-config/xemu.toml "$XDG_DATA_HOME/xemu/xemu/xemu.toml"
  echo "[entry] seeded default xemu config"
fi

# ── Xbox HDD image : xemu l'ouvre en ÉCRITURE (locked=on) ──
# Le mount BIOS est read-only → copier l'image dans le volume persistant au
# premier boot, puis pointer hdd_path dessus (sed idempotent à chaque boot).
if [ ! -f "$XDG_DATA_HOME/xemu/xemu/xbox_hdd.qcow2" ] && [ -f /opt/xbox/bios/xbox_hdd.qcow2 ]; then
  cp /opt/xbox/bios/xbox_hdd.qcow2 "$XDG_DATA_HOME/xemu/xemu/xbox_hdd.qcow2"
  echo "[entry] hdd image copied to data dir"
fi
sed -i "s|^hdd_path = .*|hdd_path = \"$XDG_DATA_HOME/xemu/xemu/xbox_hdd.qcow2\"|" "$XDG_DATA_HOME/xemu/xemu/xemu.toml"

# ── Rendu à la résolution du flux (#340) ──
# xemu dessine la console (640x480) multipliée par `surface_scale`. On prend
# le multiple le plus proche de la hauteur du flux, et à égalité le plus
# petit : 360p, 540p et 720p → ×1, 1080p → ×2. Le téléphone suit la même
# règle (GlibcGuest.xemuSurfaceScale). Le fichier persiste d'une partie à
# l'autre et xemu le réécrit : la clé se remplace, ou s'ajoute à sa section.
SURFACE_SCALE=$(( (RES_H + 239) / 480 ))
[ "$SURFACE_SCALE" -lt 1 ] && SURFACE_SCALE=1
XEMU_TOML="$XDG_DATA_HOME/xemu/xemu/xemu.toml"
if grep -q '^surface_scale *=' "$XEMU_TOML"; then
  sed -i "s/^surface_scale *=.*/surface_scale = ${SURFACE_SCALE}/" "$XEMU_TOML"
elif grep -q '^\[display\.quality\]' "$XEMU_TOML"; then
  sed -i "/^\[display\.quality\]/a surface_scale = ${SURFACE_SCALE}" "$XEMU_TOML"
else
  printf '\n[display.quality]\nsurface_scale = %s\n' "$SURFACE_SCALE" >> "$XEMU_TOML"
fi
echo "[entry] xemu surface scale → ×${SURFACE_SCALE} (stream ${RES_W}x${RES_H})"

# ── SDL2: audio + controller mapping ──
# xemu (SDL2) must play into the null sink (default), streamex captures the
# monitor. The controller map pins the X/Y buttons: SDL's heuristic guess
# swaps BTN_NORTH/WEST (b2/b3), which would swap X and Y on the Xbox pad.
# GUID 0300145e… = bus 0x0003, vendor 0x1234, product 0x5678, version 0x0111
# + name crc (deterministic for "streamex pad xbox").
export SDL_AUDIODRIVER="${SDL_AUDIODRIVER:-pulseaudio}"
# IMPORTANT: weston exporte WAYLAND_DISPLAY → SDL2 choisirait le backend
# wayland et la fenêtre de xemu deviendrait une surface weston invisible
# pour la capture X11 (écran noir). Forcer le backend X11.
export SDL_VIDEODRIVER="${SDL_VIDEODRIVER:-x11}"
export PULSE_SINK="${PULSE_SINK:-xbox_sink}"
export SDL_GAMECONTROLLERCONFIG="${SDL_GAMECONTROLLERCONFIG:-0300145e341200007856000011010000,streamex pad xbox,a:b0,b:b1,x:b3,y:b2,back:b6,guide:b8,start:b7,leftstick:b9,rightstick:b10,leftshoulder:b4,rightshoulder:b5,dpup:h0.1,dpdown:h0.4,dpleft:h0.8,dpright:h0.2,leftx:a0,lefty:a1,rightx:a3,righty:a4,lefttrigger:a2,righttrigger:a5,platform:Linux}"

# ── xemu: booted by streamex (ConsoleAdapter xbox) with GAME_PATH ──
# No launch here anymore: streamex starts the game at boot (GAME_PATH), and
# stops it with the session.

# ── streamex: generic WebRTC streaming server (Go) ──
# xemu's audio goes through the xbox_sink monitor.
export PULSE_SOURCE="${PULSE_SOURCE:-xbox_sink.monitor}"
echo "[entry] starting streamex (session=${SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
