#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint (Xbox 360):
#   PulseAudio (null sink) → X server :2 → openbox → streamex (qui lance Xenia)
# ════════════════════════════════════════════════════════════════
set -euo pipefail

# ── L'émulateur, depuis la bibliothèque ──
# L'image ne le porte pas (chantier 29) : le kernel monte cores/ en lecture
# seule, et unpack-emulator le déballe là où l'adaptateur le lance.
unpack-emulator /opt/xenia/squashfs-root

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
# The null sink receives Xenia's audio; streamex captures its monitor.
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
# Weston headless + Xwayland (DRI3/glamor, required for Xenia's Vulkan).
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

# ── Dossier de Xenia ──
# Sa racine de stockage par défaut est $XDG_DATA_HOME/Xenia : le kernel y
# monte le dossier du joueur (sauvegardes, profil, configuration, caches).
XENIA_ROOT="$XDG_DATA_HOME/Xenia"
mkdir -p "$XENIA_ROOT"

# UNE CONFIGURATION AMORCÉE, JAMAIS UNE CONFIGURATION À MOITIÉ ÉCRITE.
#
# Xenia réécrit son fichier en entier à chaque démarrage. Mais un fichier sans
# `[Config] defaults_date` lui fait croire à une très vieille configuration :
# il y applique alors toutes ses « mises à jour de valeurs par défaut », dont
# certaines remettent une clé à sa valeur par défaut QUELLE QU'ELLE SOIT
# (ConfigVarUpdateFromAny, src/xenia/base/cvar.h). Or au premier lancement d'un
# joueur, c'est un tel fichier que les paramètres de l'émulateur écriraient —
# leurs seules clés. La date la plus haute dit « rien à mettre à jour » ; Xenia
# la remplace par la sienne en réécrivant le fichier.
XENIA_CONFIG="$XENIA_ROOT/xenia-canary.config.toml"
if [ ! -f "$XENIA_CONFIG" ]; then
  printf '[Config]\ndefaults_date = 4294967295\n' > "$XENIA_CONFIG"
  echo "[entry] configuration de Xenia amorcée"
fi

# LE PROFIL SE RECONNECTE DE LUI-MÊME (#420). Sans profil connecté, un jeu ne
# sauvegarde pas (Halo 3 : « NOT SIGNED IN »), et le choisir passe par la
# fenêtre « Sign In » de Xenia, rétive à la manette. Xenia retient bien le
# profil connecté (`logged_profile_slot_0_xuid`), mais n'écrit sa configuration
# qu'à une sortie propre (xenia_main.cc) — et une partie s'arrête par un TERM :
# il l'oubliait à chaque fois. Un compte qui n'a qu'UN profil est donc
# reconnecté avec lui ; avec plusieurs, le choix reste au joueur.
#
# Un profil est `content/<XUID>/FFFE07D1/00010000/<XUID>/`, comme Xenia les
# cherche (ProfileManager::FindProfiles).
xenia_profiles=()
for dir in "$XENIA_ROOT"/content/*/; do
  xuid="$(basename "$dir")"
  if [ "$xuid" != 0000000000000000 ] && [ -d "$dir/FFFE07D1/00010000/$xuid" ]; then
    xenia_profiles+=("$xuid")
  fi
done
if [ "${#xenia_profiles[@]}" -eq 1 ] && ! grep -Eq '^logged_profile_slot_0_xuid = "[0-9A-F]{16}"' "$XENIA_CONFIG"; then
  xuid="${xenia_profiles[0]}"
  if grep -q '^logged_profile_slot_0_xuid' "$XENIA_CONFIG"; then
    sed -i "s/^logged_profile_slot_0_xuid = \"[^\"]*\"/logged_profile_slot_0_xuid = \"$xuid\"/" "$XENIA_CONFIG"
  elif grep -q '^\[Profiles\]' "$XENIA_CONFIG"; then
    sed -i "/^\[Profiles\]/a logged_profile_slot_0_xuid = \"$xuid\"" "$XENIA_CONFIG"
  else
    printf '\n[Profiles]\nlogged_profile_slot_0_xuid = "%s"\n' "$xuid" >> "$XENIA_CONFIG"
  fi
  echo "[entry] profil Xenia $xuid connecté d'office"
fi
echo "[entry] Xenia ${XENIA_RELEASE:-?}"

# ── streamex: generic WebRTC streaming server (Go) ──
# Xenia joue le son par SDL dans le puits nul, dont streamex capture le
# moniteur. GTK et SDL en X11 : Xenia n'a pas de surface Wayland, et Weston
# exporte WAYLAND_DISPLAY — sans ces deux variables, la fenêtre serait une
# surface Weston invisible pour la capture X11.
export GDK_BACKEND="${GDK_BACKEND:-x11}"
export SDL_AUDIODRIVER="${SDL_AUDIODRIVER:-pulseaudio}"
export SDL_VIDEODRIVER="${SDL_VIDEODRIVER:-x11}"
export PULSE_SINK="${PULSE_SINK:-xbox360_sink}"
export PULSE_SOURCE="${PULSE_SOURCE:-xbox360_sink.monitor}"
echo "[entry] starting streamex (session=${STREAMEX_SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
