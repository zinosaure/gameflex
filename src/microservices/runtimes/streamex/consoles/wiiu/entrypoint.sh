#!/bin/bash
# ════════════════════════════════════════════════════════════════
#  Container entrypoint (Wii U):
#   PulseAudio (null sink) → X server :2 → openbox → Cemu (game) → streamex
# ════════════════════════════════════════════════════════════════
set -euo pipefail

# ── L'émulateur, depuis la bibliothèque ──
# L'image ne le porte pas (chantier 29) : le kernel monte cores/ en lecture
# seule, et unpack-emulator le déballe là où l'adaptateur le lance.
unpack-emulator /opt/cemu/squashfs-root

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
# The null sink receives Cemu's audio; streamex captures its monitor.
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
# Weston headless + Xwayland (DRI3/glamor, required for Cemu's Vulkan).
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

# ── Cemu : sa configuration, avant tout premier lancement ──
#
# Cemu 2.x ouvre un assistant « Getting started » au tout premier démarrage —
# une fenêtre modale qu'aucun joueur ne peut fermer d'ici : il n'y a pas de
# souris dans une session, et le jeu ne démarre pas tant qu'elle est là. On
# écrit donc la configuration AVANT, avec l'assistant déjà déclaré vu.
#
# C'est le même geste que le xemu.toml de la Xbox : une console de salon n'a
# pas de premier démarrage à configurer, elle démarre.
CEMU_CONFIG="${XDG_CONFIG_HOME}/Cemu"
mkdir -p "$CEMU_CONFIG" "${XDG_DATA_HOME}/Cemu"
if [ ! -f "$CEMU_CONFIG/settings.xml" ]; then
  cp /opt/cemu-config/settings.xml "$CEMU_CONFIG/settings.xml"
  echo "[entry] Cemu: configuration initiale posée (assistant désactivé)"
fi

# ── Les clés de déchiffrement, si l'administrateur les a posées ──
#
# Un disque Wii U est CHIFFRÉ. Sans le fichier de clés, Cemu démarre, ouvre sa
# fenêtre, affiche le bon chemin, et refuse : « Could not decrypt title. Make
# sure that keys.txt contains the correct disc key for this title. » — rien de
# cassé, il manque une donnée, exactement comme un BIOS PS2 manquant.
#
# Cemu les lit dans son dossier de données. On les prend là où toutes les
# autres consoles prennent les leurs : le dossier BIOS de la console, dans le
# stockage froid — en LECTURE SEULE, on ne fait que recopier.
# UN SEUL DOSSIER, N'IMPORTE QUEL NOM DE FICHIER.
#
# Le dossier est celui de toutes les consoles, `bios/<console>/`, et il n'y a
# pas de repli ailleurs — permettre deux chemins, c'est garantir qu'on
# trouvera les deux (le commentaire des clés Citron, côté Switch, raconte ce
# qu'ça a coûté). Le NOM du fichier, en revanche, appartient à
# l'administrateur : Cemu exige « keys.txt » dans SON dossier à lui, ce qui
# n'oblige personne à l'avoir nommé ainsi sur l'étagère. On prend keys.txt
# s'il est là, sinon l'unique fichier du dossier — et on refuse de deviner
# quand il y en a plusieurs, plutôt que d'en choisir un au hasard.
WIIU_BIOS_DIR="${GAMEFLEX_COLD_ROOT:-/gameflex}/bios/wiiu"
WIIU_KEYS=""
if [ -f "$WIIU_BIOS_DIR/keys.txt" ]; then
  WIIU_KEYS="$WIIU_BIOS_DIR/keys.txt"
elif [ -d "$WIIU_BIOS_DIR" ]; then
  set -- "$WIIU_BIOS_DIR"/*
  if [ "$#" -eq 1 ] && [ -f "$1" ]; then
    WIIU_KEYS="$1"
  elif [ "$#" -gt 1 ]; then
    echo "[entry] Cemu: plusieurs fichiers dans $WIIU_BIOS_DIR — nommez les clés « keys.txt »"
  fi
fi
if [ -n "$WIIU_KEYS" ]; then
  cp "$WIIU_KEYS" "${XDG_DATA_HOME}/Cemu/keys.txt"
  echo "[entry] Cemu: clés en place depuis $(basename "$WIIU_KEYS") ($(wc -l < "$WIIU_KEYS") ligne(s))"
else
  echo "[entry] Cemu: AUCUNE clé dans $WIIU_BIOS_DIR — les jeux chiffrés seront refusés (déposez-y keys.txt)"
fi

# ── Les correctifs de jeu, fournis par l'administrateur ──
#
# Un correctif appartient à un JEU, pas à une image : celui de New Super Mario
# Bros. U ne sert qu'à ce jeu. Il vit donc dans le stockage froid, à côté des
# BIOS et des ROMs — `patches/<console>/`, en lecture seule — et pas dans
# l'image, que tout le monde télécharge.
#
# Cemu, lui, les cherche dans SON dossier de données (`graphicPacks/`), monté
# par utilisateur : on recopie donc à chaque lancement, exactement comme les
# clés. Le sous-dossier `gameflex/` est à nous — un pack que le joueur
# déposerait à côté n'est jamais touché.
#
# Aucun réglage à écrire : un pack dont `rules.txt` porte `default = 1`
# s'active tout seul tant que `settings.xml` ne le contredit pas, et le nôtre
# n'a aucune entrée de pack (`<GraphicPack/>` vide). Vérifié dans le code de
# Cemu 2.6, `GraphicPack2.cpp` : sans entrée de configuration, c'est `default`
# qui décide.
WIIU_PATCHES_DIR="${GAMEFLEX_COLD_ROOT:-/gameflex}/patches/wiiu"
if [ -d "$WIIU_PATCHES_DIR" ] && [ -n "$(ls -A "$WIIU_PATCHES_DIR" 2>/dev/null)" ]; then
  mkdir -p "${XDG_DATA_HOME}/Cemu/graphicPacks/gameflex"
  cp -r "$WIIU_PATCHES_DIR"/. "${XDG_DATA_HOME}/Cemu/graphicPacks/gameflex/"
  echo "[entry] Cemu: correctifs en place depuis patches/wiiu ($(find "$WIIU_PATCHES_DIR" -name rules.txt | wc -l) pack(s))"
else
  echo "[entry] Cemu: AUCUN correctif dans $WIIU_PATCHES_DIR — New Super Mario Bros. U plantera en quittant l'écran titre (voir docs/TROUBLESHOOTING.md)"
fi

# ── streamex: generic WebRTC streaming server (Go) ──
# Cemu's audio goes through the wiiu_sink monitor.
export SDL_AUDIODRIVER="${SDL_AUDIODRIVER:-pulseaudio}"
export SDL_VIDEODRIVER="${SDL_VIDEODRIVER:-x11}"
export PULSE_SINK="${PULSE_SINK:-wiiu_sink}"
export PULSE_SOURCE="${PULSE_SOURCE:-wiiu_sink.monitor}"
echo "[entry] starting streamex (session=${STREAMEX_SESSION_ID:-standalone}, display=$DISPLAY)"
exec /usr/local/bin/streamex
