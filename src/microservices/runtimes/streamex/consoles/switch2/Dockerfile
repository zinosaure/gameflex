# syntax=docker/dockerfile:1
# ════════════════════════════════════════════════════════════════
#  streamex-switch2: Lichii (Nintendo Switch 2, a fork of Eglantine,
#                    itself an Eden fork) streamed over WebRTC by streamex
#
#  Stage skeleton: streamex, built once (streamex-bin).
#  Stage mesa:     Mesa from trixie-backports.
#  Stage radv:     RADV rebuilt from Debian's source, plus Mesa's mesh fix.
#  Final stage:    weston + xwayland + pulseaudio, Lichii unpacked at launch.
# ════════════════════════════════════════════════════════════════

# Squelette pré-compilé (streamex/sessreg/sdiag-guid).
# Le squelette : streamex-bin en développement, l'image versionnée pour une
# installation publique (--build-arg STREAMEX_BIN=<espace>/streamex-bin:<version>).
ARG STREAMEX_BIN=streamex-bin
FROM ${STREAMEX_BIN} AS skeleton

# MESA VIENT DE trixie-backports (26.1.6 au 2026-11-11), comme pour la PS3.
#
# Le RADV 25.0.7 de trixie a fait planter RPCS3 au passage en combat de Final
# Fantasy XIII (segfault DANS libvulkan_radeon.so, 2026-09-11 — le détail est
# dans consoles/ps3/Dockerfile). Toutes les consoles streamex tournent sur le
# même APU et le même pilote : elles s'alignent, plutôt que d'attendre leur
# propre crash. Simulé image par image avant l'écriture : aucun paquet issu de
# Mesa ne reste en 25.0.7.
#
# MESA D'ABORD, le reste ensuite (dans l'étape finale) : Weston et Xwayland
# tirent Mesa en dépendance, et dans l'ordre inverse apt poserait la 25.0.7 pour
# la remplacer aussitôt. libglx-mesa0 fait partie du lot : libgl1/libglx0 le
# tirent, et venu de trixie il réclamerait une autre version exacte de
# libgallium.
#
# UNE ÉTAPE À LUI, d'où partent l'étape « radv » et l'étape finale, comme pour la
# PS5 : quoi que garde le cache, les deux tiennent le même Mesa. Refaire cette
# couche refait les deux ; une montée de Lichii, un paquet ajouté à l'étape
# finale ou un fichier de consoles/mesa/ touché ne la refont pas.
FROM debian:trixie-slim AS mesa
ENV DEBIAN_FRONTEND=noninteractive
RUN echo "deb http://deb.debian.org/debian trixie-backports main" \
        > /etc/apt/sources.list.d/backports.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t trixie-backports \
        libgl1-mesa-dri mesa-vulkan-drivers libegl-mesa0 libgbm1 libglx-mesa0 mesa-va-drivers \
    && rm -rf /var/lib/apt/lists/*

# RADV CORRIGÉ : libvulkan_radeon.so reconstruit avec le correctif des mesh
# shaders de Mesa (#644, décidé par le porteur le 2026-11-11).
#
# Ce que c'est : le pilote Vulkan d'AMD du paquet que l'étape « mesa » installe,
# reconstruit de SON paquet source (même version, correctifs de Debian compris),
# avec la configuration de debian/rules réduite à RADV, plus
# ../mesa/radv-mesh-stale-drawid.patch : le commit 48da0f3a de Mesa (!45092),
# porté sur 26.1. La recette est celle de la PS5, partagée
# (../mesa/build-radv.sh) : un témoin sans correctif doit être, section par
# section, le pilote du paquet, et le correctif s'applique sans flou, sinon la
# construction s'arrête avec ce qu'il faut faire.
#
# Pourquoi : les jeux de la Switch 2 dessinent par mesh shaders. Sur GFX11 (le
# 780M), RADV avant 26.2.5 gardait l'état du shader précédent en liant un mesh
# shader qui ne lit ni la taille de la grille ni l'index du dessin : un paquet
# tronqué, puis « Illegal opcode in command stream », et le GPU entier remis à
# zéro. C'est la signature de la panne du 2026-11-06 sous Eglantine (journal
# 459) ; le détail, et ce qui n'est pas prouvé, sont dans l'en-tête du correctif.
#
# MESA 26.2 L'ARRÊTERA : le correctif y est déjà (26.2.5), et patch le refusera.
# C'est voulu : on le retire alors, selon la liste de son en-tête. Ce qu'elle
# coûte, le temps de la construction : ~1,2 Gio de dépendances, ~6,3 Gio de
# mémoire au pic, deb.debian.org joignable, et ~1 min 30 avec BuildKit (mesuré
# pour la PS5).
FROM mesa AS radv
COPY src/microservices/runtimes/streamex/consoles/mesa/ /radv/
RUN bash /radv/build-radv.sh /radv/out /radv/radv-mesh-stale-drawid.patch

FROM mesa

# GARDÉE À L'ABRI DE `docker system prune -a`.
#
# Une image de session n'a pas de conteneur permanent : entre deux parties,
# rien ne la référence, et un élagage la considère donc comme inutilisée. Elle
# part, et la partie suivante répond « l'image de cette console est en cours de
# reconstruction » — le kernel la rebâtit tout seul, mais il y en a pour
# plusieurs minutes, à chaque fois (vécu le 2026-09-07, et déjà le 2026-08-17).
#
# Cette étiquette est ce qu'un élagage peut filtrer :
#
#     docker system prune -a --filter "label!=gameflex.prune=keep"
#
# Voir `tools/prune.sh`, qui l'écrit pour ne pas avoir à s'en souvenir.
LABEL gameflex.prune=keep

# Le reste, sur le Mesa de l'étape « mesa ». Sans -t trixie-backports : apt
# garde les paquets de Mesa installés, que rien ici ne demande plus récents (la
# vérification de la pose le contrôle).
RUN apt-get update \
    && apt-get install -y --no-install-recommends \
        openbox pulseaudio pulseaudio-utils dbus dbus-x11 \
        weston xwayland \
        fontconfig fonts-dejavu-core \
        # System Mesa: glamor/DRI3 for Xwayland + Vulkan/GL fallback
        x11-xserver-utils xauth xdotool procps imagemagick \
        # streamex runtime: VA-API (encodage GPU) + openh264 (repli) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 libxcb-cursor0 libc++1 libc++abi1 \
        # Les deux bibliothèques que Lichii attend du système, et que son
        # AppImage n'emporte pas (ldd, 2026-11-06 pour Eglantine, 2026-11-11
        # pour Lichii) : sans elles, il ne démarre même pas
        # (« libusb-1.0.so.0: cannot open shared object file »).
        libusb-1.0-0 libopengl0 \
        # Bus d'accessibilité : streamex y lit la barre d'état de Lichii (FPS)
        at-spi2-core \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# LE RADV DE L'ÉTAPE « radv », par-dessus celui du paquet. Seul Lichii le
# charge : Weston rend en GL, Xwayland passe par glamor. Puis les trois
# vérifications de la PS5 : la version installée est celle qui a été construite,
# le fichier posé est le nôtre (son empreinte), rien ne manque à ses liens. La
# version, l'empreinte et le correctif restent dans /usr/share/doc/gameflex/radv/.
# `dpkg --verify` signale le fichier remplacé : c'est voulu.
COPY --from=radv /radv/out/libvulkan_radeon.so /usr/lib/x86_64-linux-gnu/libvulkan_radeon.so
COPY --from=radv /radv/out/mesa-version /radv/out/libvulkan_radeon.so.sha256 /radv/out/*.patch /usr/share/doc/gameflex/radv/
RUN installed=$(dpkg-query -W -f='${Version}' mesa-vulkan-drivers) \
    && built=$(cat /usr/share/doc/gameflex/radv/mesa-version) \
    && if [ "$installed" != "$built" ]; then \
        echo "mesa-vulkan-drivers installé en $installed, RADV construit depuis $built : l'apt de l'étape finale a monté Mesa. Reconstruire sans cache" >&2; \
        exit 1; \
    fi \
    && sha256sum --check --quiet /usr/share/doc/gameflex/radv/libvulkan_radeon.so.sha256 \
    && if ldd -r /usr/lib/x86_64-linux-gnu/libvulkan_radeon.so 2>&1 | grep -E 'not found|undefined symbol'; then \
        exit 1; \
    fi

# The "pulse" system user is required by PulseAudio in --system mode
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
# In system mode, module-native-protocol-unix only accepts the pulse-access group
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# LICHII (#644, 2026-11-11), l'émulateur de la Nintendo Switch 2 : un fork
# d'Eglantine (renommé le 2026-10-31), lui-même fork d'Eden (GPL-3.0). Il en
# garde la ligne de commande (-f -g <jeu>) et le qt-config.ini, mais range ses
# données sous `lichii/` (~/.config/lichii, ~/.local/share/lichii,
# ~/.cache/lichii) et NON plus sous `eden/` : un ~/.local/share/eden lui fait
# ouvrir son assistant d'import, une fenêtre que personne ne peut fermer
# (l'entrypoint refuse de démarrer dans ce cas). Sa version publiée build-150
# (17baed7, 2026-11-10), en AppImage avec son profil PGO.
# L'éditeur publie un nom SANS version (lichii-linux-x86_64-pgo.AppImage) :
# celui rangé dans cores/switch2/ la porte, comme pour Xenia.
# Même valeur que emulator_file switch2 (consoles/emulators.sh) : ce build le
# PREND là, et nulle part ailleurs (#145). Déclarée après les apt : une montée
# de Lichii ne refait que la suite.
ARG LICHII_FILE=Lichii-build-150-x86_64-pgo.AppImage

# ── Lichii : l'AppImage officielle, DÉBALLÉE AU LANCEMENT ──
# L'image ne la porte pas : on ne redistribue pas un émulateur (#145), et
# c'est ce qui permet de publier l'image (chantier 29). Elle dit comment
# reconnaître ses versions — un motif de nom —, et laquelle a été testée avec
# elle : celle de consoles/emulators.sh, que l'écran Émulateurs propose de
# télécharger quand le dossier n'en a aucune. Le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe la DERNIÈRE DÉPOSÉE dans /opt/lichii
# (voir consoles/unpack-emulator.sh). Son fichier se range sous cores/switch2/.
ARG LICHII_URL=https://github.com/zortos293/Lichii/releases/download/build-150/lichii-linux-x86_64-pgo.AppImage
# Les versions éprouvées avec cette image (emulator_tested) : la liste informe,
# elle ne bloque pas — toute version que reconnaît le motif se lance.
ARG LICHII_TESTED="${LICHII_FILE}"
ENV GAMEFLEX_EMULATOR_FILE=${LICHII_FILE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/switch2 \
    GAMEFLEX_EMULATOR_PATTERN="Lichii-*.AppImage" GAMEFLEX_EMULATOR_NAME="Lichii" \
    GAMEFLEX_EMULATOR_TESTED="${LICHII_TESTED}"
LABEL gameflex.emulator.file=${LICHII_FILE} gameflex.emulator.url=${LICHII_URL} \
    gameflex.emulator.pattern="Lichii-*.AppImage" \
    gameflex.emulator.tested="${LICHII_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY src/microservices/runtimes/streamex/consoles/switch2/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/switch2/qt-config.ini /opt/lichii-config/qt-config.ini
COPY src/microservices/runtimes/streamex/consoles/switch2/system.pa /opt/pulse/system.pa
# L'adaptateur lance usr/bin/lichii, PAS AppRun. Dans cette AppImage, AppRun
# n'est qu'un lien vers ce binaire : lancé par lui, le processus s'appellerait
# « AppRun », et l'arrêt (`pkill -x lichii`) ne le trouverait pas (le même piège
# qu'Eglantine, vérifié le 2026-11-06 ; l'arbre de Lichii a le même lien,
# 2026-11-11). Le binaire trouve seul ses bibliothèques (RUNPATH $ORIGIN/../lib)
# et ses modules Qt (usr/bin/qt.conf).
COPY src/microservices/runtimes/streamex/consoles/switch2/adapter.json /opt/adapter.json

# Pad identity (Switch Pro ids). Lichii reads the pad through cemuhook, fed from
# streamex's own pad state; the uinput device itself is only visible in this
# container, streamex creating its node (gamepad.createNode).
ENV STREAMEX_PAD_VENDOR=0x057e STREAMEX_PAD_PRODUCT=0x2009
# Contrat data-driven : games/launch/stop/gamepad/tools de CETTE console —
# streamex (le squelette) ne les connaît pas, il lit ce fichier (voir
# internal/adapter/config.go).
ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json
# Accessibilité Qt allumée d'office : Lichii n'écrit sa cadence que dans sa
# barre d'état, et c'est par le bus d'accessibilité (at-spi2-core, ci-dessus)
# que streamex peut la lire.
ENV QT_LINUX_ACCESSIBILITY_ALWAYS_ON=1
# L'ÉCRAN EST X11. Le Qt que Lichii embarque a aussi ses greffons Wayland, et
# le weston de l'entrypoint ouvre `wayland-0` : sans ce choix, Qt pourrait
# ouvrir sa fenêtre hors de l'écran X que streamex capture. Les autres images
# forcent de même SDL et GTK en X11.
ENV QT_QPA_PLATFORM=xcb

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator

EXPOSE 8080

# Liveness binaire (healthy/unhealthy) : Docker ne connaît que ces deux
# états, pas de "caution" — la nuance "en service mais dégradé" (fallback
# clavier actif faute de pad uinput) vit dans /status.input.degraded, pas
# ici. start-period large : weston + Xwayland + l'émulateur peuvent prendre
# du temps à booter, un HEALTHCHECK qui tire trop tôt marquerait le
# conteneur unhealthy pendant un démarrage parfaitement normal.
HEALTHCHECK --interval=15s --timeout=5s --start-period=45s --retries=3 \
    CMD ["/bin/sh", "-c", "curl -fsS http://127.0.0.1:${STREAMEX_PORT}/status >/dev/null || exit 1"]

# JAMAIS DE RENDU LOGICIEL : llvmpipe est retiré du chargeur Vulkan.
#
# mesa-vulkan-drivers installe aussi llvmpipe (lavapipe), un moteur Vulkan qui
# rend sur le PROCESSEUR. Un émulateur qui le choisit tourne quand même —
# lentement, ou sur un écran noir — sans rien signaler. shadPS4 0.18.0 le
# choisissait à chaque partie (2026-09-11) : il départage deux GPU « non
# dédiés » à la taille du tas mémoire, et llvmpipe annonce toute la RAM de
# l'hôte contre la seule VRAM de l'APU. RPCS3 et Kyty choisissent RADV (mesuré),
# mais rien ne le garantit pour les autres : le chargeur l'écarte pour tous.
# Vérifié : avec cette variable, vulkaninfo ne liste plus que RADV. Lichii
# embarque son propre chargeur Vulkan, qui lit aussi cette variable
# (2026-11-11, `strings`).
ENV VK_LOADER_DRIVERS_DISABLE="*lvp*"

ENTRYPOINT ["/entrypoint.sh"]
