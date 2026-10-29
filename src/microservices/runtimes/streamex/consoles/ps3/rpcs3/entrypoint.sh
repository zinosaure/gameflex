#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint (PS3):
#   PulseAudio (null sink) → X server :2 → openbox → RPCS3 (game) → streamex
# ════════════════════════════════════════════════════════════════
set -euo pipefail

# ── L'émulateur, depuis la bibliothèque ──
# L'image ne le porte pas (chantier 29) : le kernel monte cores/ en lecture
# seule, et unpack-emulator le déballe là où l'adaptateur le lance.
unpack-emulator /opt/rpcs3/squashfs-root

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
# The null sink receives RPCS3's audio; streamex captures its monitor.
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
# Qt Multimedia speaks to PulseAudio, never PipeWire: there is no PipeWire
# here, and the Qt 6.12 of RPCS3 0.0.43-20404 dereferenced a null PipeWire
# context while building its music handler — a segfault on the main thread
# before any game, every game (2026-10-26).
export QT_AUDIO_BACKEND=pulseaudio

# ── Virtual X server ──
# Weston headless + Xwayland: unlike Xvfb, Xwayland supports DRI3,
# essential for RPCS3's Vulkan presentation (otherwise the game won't render).
# Stale X sockets/locks survive `docker compose start` (container reuse).
rm -f "/tmp/.X11-unix/X${DISPLAY#:}" "/tmp/.X${DISPLAY#:}-lock"
RES="${STREAMEX_RESOLUTION:-1280x720}"
RES_W="${RES%x*}"
RES_H="${RES#*x}"
# --renderer=gl: required to expose linux-dmabuf/GBM to Xwayland
# (otherwise glamor fails → no DRI3 → RPCS3's Vulkan cannot present).
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
# Wait until the X socket is actually ready (otherwise RPCS3 crashes at boot)
for _ in $(seq 1 15); do
  [ -S "/tmp/.X11-unix/X${DISPLAY#:}" ] && break
  sleep 1
done
echo "[entry] X server ready: $(ls /tmp/.X11-unix/ 2>/dev/null | tr '\n' ' ')"

# ── Window manager ──
openbox --sm-disable &
sleep 1

# ── RPCS3 config (seed only if missing, so the host config is not overwritten) ──
mkdir -p "$XDG_CONFIG_HOME/rpcs3"
if [ ! -f "$XDG_CONFIG_HOME/rpcs3/config.yml" ]; then
  cp /opt/rpcs3-config/config.yml "$XDG_CONFIG_HOME/rpcs3/config.yml"
  echo "[entry] seeded default rpcs3 config"
fi
# Pad config (P1-P4 evdev, virtual pad NAMES) : statique, comme config.yml
# ci-dessus — plus de génération Go au lancement (voir config.yml, tête de
# fichier, pour le pourquoi).
mkdir -p "$XDG_CONFIG_HOME/rpcs3/input_configs/global"
if [ ! -f "$XDG_CONFIG_HOME/rpcs3/input_configs/global/Default.yml" ]; then
  cp /opt/rpcs3-config/input_configs/global/Default.yml \
     "$XDG_CONFIG_HOME/rpcs3/input_configs/global/Default.yml"
  echo "[entry] seeded default rpcs3 pad config (P1-P4 evdev)"
fi
# Cubeb récent ne résout plus "PulseAudio" comme dev_id : forcer le nom du
# sink null (rpcs3_sink) sur les configs existantes, sinon pas de son.
sed -i 's/^  Audio Device:.*/  Audio Device: rpcs3_sink/' "$XDG_CONFIG_HOME/rpcs3/config.yml"
echo "[entry] rpcs3 audio device → rpcs3_sink"
# Les configs semées avant le 2026-10-08 portent encore 2 fils de compilation
# LLVM (voir config.yml) : 8 rend la première partie d'un jeu 2,8 fois plus
# courte. Seule la valeur d'origine est migrée — un réglage choisi à la main
# dans RPCS3 reste le sien.
sed -i 's/^  Max LLVM Compile Threads: 2$/  Max LLVM Compile Threads: 8/' "$XDG_CONFIG_HOME/rpcs3/config.yml"
# La résolution de RENDU suit celle du flux (2026-10-12). RPCS3 dessinait
# toujours sa sortie 1280x720, réduite ou agrandie ensuite à la taille de
# l'écran : « faire en sorte que l'émulateur rende les images tel qu'on a
# choisi dans stream settings » (le porteur). Resolution Scale est un
# pourcentage de cette sortie — 540p → 75, 1080p → 150 —, la règle du
# téléphone (gameflex/emuparams, RenderScale). RPCS3 refuse moins de 25.
#
# POSÉE si elle manque (2026-10-12, #347) : les paramètres de l'émulateur
# (params/ps3/rpcs3.json, appliqués par streamex juste avant RPCS3) retirent
# ce qu'ils avaient posé quand on les désactive — y compris cette ligne. Un
# `sed` qui ne fait que remplacer ne la remettait plus, et RPCS3 retombait à
# 100 % quel que soit le flux.
RENDER_SCALE=$(( (RES_H * 100 + 360) / 720 ))
[ "$RENDER_SCALE" -lt 25 ] && RENDER_SCALE=25
if grep -q '^  Resolution Scale:' "$XDG_CONFIG_HOME/rpcs3/config.yml"; then
  sed -i "s/^  Resolution Scale: .*/  Resolution Scale: ${RENDER_SCALE}/" "$XDG_CONFIG_HOME/rpcs3/config.yml"
else
  sed -i "/^Video:$/a\\  Resolution Scale: ${RENDER_SCALE}" "$XDG_CONFIG_HOME/rpcs3/config.yml"
fi
echo "[entry] rpcs3 render scale → ${RENDER_SCALE}% (stream ${RES_W}x${RES_H})"

# ── Welcome screen must never come back ──
# The RPCS3 welcome/first-run dialog blocks --no-gui booting (its Continue
# stays disabled until a checkbox is ticked). Seed the GUI settings with it
# off, and force it off on every boot (the file persists in the saves mount).
mkdir -p "$XDG_CONFIG_HOME/rpcs3/GuiConfigs"
if [ ! -f "$XDG_CONFIG_HOME/rpcs3/GuiConfigs/CurrentSettings.ini" ]; then
  cat > "$XDG_CONFIG_HOME/rpcs3/GuiConfigs/CurrentSettings.ini" <<'EOF'
[GSFrame]
geometry=@Rect(0 0 1280 720)
screen=0
visibility=FullScreen

[Meta]
attachCommandLine=false

[main_window]
infoBoxEnabledWelcome=false
EOF
fi
sed -i 's/^infoBoxEnabledWelcome=.*/infoBoxEnabledWelcome=false/' "$XDG_CONFIG_HOME/rpcs3/GuiConfigs/CurrentSettings.ini"

# ── PS3 firmware (PS3UPDAT.PUP) ──
# Mounted read-only from the host (see docker-compose.yaml). RPCS3 cannot run
# games without the official firmware installed in dev_flash.
# --headless makes InstallPup run without any dialog (returns when done).
#
# THE COMPLETION MARKER IS vsh/MODULE/vsh.self — and this test named the wrong
# path. RPCS3 0.0.42 never creates dev_flash/vsh/vsh.self: under vsh/ there is
# only etc/, module/ and resource/. Measured inside a live session container on
# 2026-09-24 — dev_flash 189 MB, vsh/module/vsh.self present, vsh/vsh.self
# absent — so the guard could NEVER fire and every PS3 session wiped the
# firmware and re-extracted 189 MB before booting. The phone carried the same
# defect in GlibcGuest.installPs3Firmware; both were fixed the same day.
#
# The wipe itself stays: a half-installed dev_flash (dirs created, extraction
# interrupted) must be cleared before re-installing, otherwise the first-run
# welcome blocks --no-gui booting forever.
#
# SHARED BY EVERY ACCOUNT SINCE CHANTIER 22: dev_flash is mounted from
# cache/installs/ps3/dev_flash (#448), so it is installed once, not once per account. It is
# a mount point now: the wipe empties it instead of removing it, and a lock on
# the folder itself puts two first launches one behind the other — the second
# finds the firmware installed. The kernel drops the lock if a session dies.
FW_DIR="$XDG_CONFIG_HOME/rpcs3/dev_flash"
if [ -f /opt/ps3/firmware/PS3UPDAT.PUP ]; then
  mkdir -p "$FW_DIR"
  flock "$FW_DIR" sh -c '
    if [ ! -f "$1/vsh/module/vsh.self" ]; then
      find "$1" -mindepth 1 -delete
      /opt/rpcs3/squashfs-root/AppRun --headless --installfw /opt/ps3/firmware/PS3UPDAT.PUP >/tmp/fw.log 2>&1 || true
      echo "[entry] PS3 firmware installed (see /tmp/fw.log)"
    fi' sh "$FW_DIR"
else
  echo "[entry] WARNING: no PS3UPDAT.PUP in /opt/ps3/firmware (firmware not mounted?)"
fi

# ── RPCS3: booted by streamex (ConsoleAdapter ps3) with GAME_PATH ──
# No launch here anymore: streamex starts the game at boot (GAME_PATH), and
# stops it with the session.

# ── streamex: generic WebRTC streaming server (Go) ──
# RPCS3's audio goes through the rpcs3_sink monitor.
export PULSE_SOURCE="${PULSE_SOURCE:-rpcs3_sink.monitor}"

# ── Fullscreen guard ──
# RPCS3's "Start games in fullscreen mode" doesn't stick under --no-gui: the
# game window (GSFrame) stays at the game's boot resolution (~640x360) and
# streamex upscales that small window to the stream resolution → zoomed,
# cropped image. openbox DOES honor a WM-triggered fullscreen request over
# Qt's size hints, so keep asking while the session lives (idempotent; the
# state sticks when the window title later changes to the FPS: line).
(
  while :; do
    sleep 3
    for w in $(xdotool search --onlyvisible --name "." 2>/dev/null); do
      geom=$(xdotool getwindowgeometry --shell "$w" 2>/dev/null) || continue
      width=$(printf '%s' "$geom" | sed -n 's/^WIDTH=//p')
      height=$(printf '%s' "$geom" | sed -n 's/^HEIGHT=//p')
      [ "$width" -le 2 ] && [ "$height" -le 2 ] && continue
      wmctrl -i -r "$w" -b add,fullscreen 2>/dev/null
    done
  done
) &

echo "[entry] starting streamex (session=${SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
