# ============================================================
#  streamex-ps5 : KytyPS5 (émulateur PlayStation 5) diffusé
#  en WebRTC par streamex.
#
#  UN ÉMULATEUR EXPÉRIMENTAL, ET C'EST DIT ICI.
#
#  KytyPS5 est en développement précoce : son propre README annonce
#  « expect crashes, graphical glitches, low compatibility, and poor
#  performance ». On l'installe pour AVOIR LE SETUP, pas pour jouer — et
#  l'écran restera celui du bureau tant qu'aucun jeu n'est présent, parce que
#  `kyty_emulator` exige `--game` et n'a ni lanceur graphique ni BIOS
#  (vérifié sur la version 0.3.0, le 2026-09-10).
#
#  Même squelette que l'image ps4 : Weston headless + Xwayland pour donner à
#  Vulkan un DRI3 présentable, PulseAudio en puits nul, et le binaire streamex
#  déjà compilé qu'on recopie.
# ============================================================
# Le squelette : streamex-bin en développement, l'image versionnée pour une
# installation publique (--build-arg STREAMEX_BIN=<espace>/streamex-bin:<version>).
ARG STREAMEX_BIN=streamex-bin
FROM ${STREAMEX_BIN} AS skeleton

FROM debian:trixie-slim

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
#
# Elle MANQUAIT ici alors que ses six voisines l'avaient (constaté le
# 2026-09-23 : `streamex-ps3` construite le même jour la portait, pas celle-ci).
LABEL gameflex.prune=keep

ENV DEBIAN_FRONTEND=noninteractive

# LA VERSION EST UN ARGUMENT, et le projet publie une archive PAR JOUR.
#
# Épingler une version datée plutôt que « latest » : un émulateur qui bouge
# tous les jours ne doit pas changer sous une session en cours, et une
# régression doit pouvoir se comparer à quelque chose. Pour monter : la
# version ici ET dans consoles/emulators.sh, puis
#   tools/streamex-emulators.sh build ps5
ARG KYTY_RELEASE=KytyPS5-2026-10-26-597137e

# MESA VIENT DE trixie-backports (26.1.2), comme pour la PS3.
#
# Le RADV 25.0.7 de trixie a fait planter RPCS3 au passage en combat de Final
# Fantasy XIII (segfault DANS libvulkan_radeon.so, 2026-09-11 — le détail est
# dans consoles/ps3/Dockerfile). Toutes les consoles streamex tournent sur le
# même APU et le même pilote : elles s'alignent, plutôt que d'attendre leur
# propre crash. Simulé image par image avant l'écriture : aucun paquet issu de
# Mesa ne reste en 25.0.7.
#
# MESA D'ABORD, le reste ensuite — Weston et Xwayland tirent Mesa en
# dépendance, et dans l'ordre inverse apt poserait la 25.0.7 pour la remplacer
# aussitôt. libglx-mesa0 fait partie du lot : libgl1/libglx0 le tirent, et
# venu de trixie il réclamerait une autre version exacte de libgallium.
RUN echo "deb http://deb.debian.org/debian trixie-backports main" \
        > /etc/apt/sources.list.d/backports.list \
    && apt-get update \
    && apt-get install -y --no-install-recommends -t trixie-backports \
        libgl1-mesa-dri mesa-vulkan-drivers libegl-mesa0 libgbm1 libglx-mesa0 mesa-va-drivers \
    && apt-get install -y --no-install-recommends \
        openbox pulseaudio pulseaudio-utils dbus dbus-x11 \
        weston xwayland \
        fontconfig fonts-dejavu-core \
        # Mesa système : glamor/DRI3 pour Xwayland + Vulkan (RADV) pour Kyty
        x11-xserver-utils xauth xdotool procps \
        # runtime streamex : openh264 (encodage) + opus + pulse
        libpulse0 libopenh264-8 libopus0 libva2 libva-drm2 \
        # Qt 6 est EMBARQUÉ dans l'archive de Kyty (plugins/, lib/, qt.conf) —
        # il ne reste que ses dépendances système, celles qu'un Qt ne porte
        # jamais lui-même.
        libxcb-cursor0 libxcb-icccm4 libxcb-image0 libxcb-keysyms1 \
        libxcb-randr0 libxcb-render-util0 libxcb-shape0 libxcb-xinerama0 \
        libxcb-xkb1 libxkbcommon-x11-0 libxkbcommon0 \
        libx11-xcb1 libsm6 libice6 libglib2.0-0 libfreetype6 \
        libudev1 libcap2 \
        ca-certificates curl \
    && rm -rf /var/lib/apt/lists/*

# L'utilisateur système « pulse » est exigé par PulseAudio en mode --system
RUN id pulse >/dev/null 2>&1 || useradd -r -d /var/run/pulse -s /usr/sbin/nologin pulse
RUN groupadd -f pulse-access && usermod -aG pulse-access root && usermod -aG pulse-access pulse

# KytyPS5, la publication officielle, DÉBALLÉE AU LANCEMENT. L'archive porte
# le binaire, son Qt et ses plugins : on la déballe telle quelle, sans rien
# réarranger. L'image ne la porte pas : on ne redistribue pas un émulateur
# (#145), et c'est ce qui permet de publier l'image (chantier 29). Elle dit comment
# reconnaître ses versions — un motif de nom —, et laquelle a été testée avec
# elle : celle de consoles/emulators.sh, que l'écran Émulateurs propose de
# télécharger quand le dossier n'en a aucune. Le kernel monte `cores/` à
# l'exécution, et l'entrypoint déballe la DERNIÈRE DÉPOSÉE dans /opt/kyty
# (voir consoles/unpack-emulator.sh). Kyty publie un tar.gz ordinaire, pas une
# AppImage.
#
# NOTRE Kyty, s'il est dans la bibliothèque (tools/kyty/build.sh --server) : le
# même, avec nos patchs pour le serveur (#431, tools/kyty/README.md).
# Sans lui, celui de l'éditeur — et le journal de la partie le dit.
ARG KYTY_FILE=${KYTY_RELEASE}-Linux-x86_64.tar.gz
ARG KYTY_URL=https://github.com/KytyPS5/KytyPS5/releases/download/KytyPS5-2026-10-26-597137e/KytyPS5-2026-10-26-597137e-Linux-x86_64.tar.gz
# Les versions éprouvées avec cette image (emulator_tested) : la liste informe,
# elle ne bloque pas — toute version que reconnaît le motif se lance.
ARG KYTY_TESTED="${KYTY_FILE}"
ENV GAMEFLEX_EMULATOR_FILE=${KYTY_FILE} GAMEFLEX_EMULATOR_DIR=/gameflex/cores/ps5 \
    GAMEFLEX_EMULATOR_PATTERN="KytyPS5-*-Linux-x86_64.tar.gz" GAMEFLEX_EMULATOR_NAME="KytyPS5" \
    GAMEFLEX_EMULATOR_TESTED="${KYTY_TESTED}" \
    GAMEFLEX_EMULATOR_BUILD="kyty_emulator-gameflex-KytyPS5-*" \
    GAMEFLEX_EMULATOR_BUILD_TARGET=/opt/kyty/kyty_emulator
LABEL gameflex.emulator.file=${KYTY_FILE} gameflex.emulator.url=${KYTY_URL} \
    gameflex.emulator.pattern="KytyPS5-*-Linux-x86_64.tar.gz" \
    gameflex.emulator.tested="${KYTY_TESTED}"
COPY src/microservices/runtimes/streamex/consoles/unpack-emulator.sh /usr/local/bin/unpack-emulator

COPY --from=skeleton /streamex /usr/local/bin/streamex
# FFmpeg, que streamex charge — en bibliothèques partagées, sa licence (la
# LGPL) le veut ainsi — et sa source (voir streamex/src/Dockerfile).
COPY --from=skeleton /ffmpeg/lib/ /usr/local/lib/
COPY --from=skeleton /ffmpeg/*.tar.xz /usr/share/doc/gameflex/sources/
RUN ldconfig
COPY --from=skeleton /sdiag-guid /usr/local/bin/sdiag-guid
COPY src/microservices/runtimes/streamex/consoles/ps5/entrypoint.sh /entrypoint.sh
COPY src/microservices/runtimes/streamex/consoles/ps5/system.pa /opt/pulse/system.pa
# L'adaptateur : UN JEU PS5 EST UN DOSSIER.
#
# `eboot.bin` à la racine, et l'essentiel du poids dans `Media/`, `sce_sys/`,
# `sce_module/` — c'est donc `scan_dir_for` qui reconnaît le jeu, et il n'y a
# aucune extension à déclarer pour ce cas (le garde-fou qui l'exigeait quand
# même a été corrigé le 2026-09-10 : voir internal/adapter/config.go).
# Vérifié : `--game` accepte indifféremment le dossier ou le chemin de
# l'`eboot.bin` que `scan_dir_for` lui passe — Kyty retrouve la racine seul.
#
# La règle `.elf` sert les homebrew, que `kyty_emulator --game <dir|elf>`
# accepte directement. Sans elle, un `.elf` déposé dans `roms/ps5`
# apparaîtrait dans la bibliothèque (le scanner du kernel liste tout fichier)
# et échouerait au lancement — visible, et injouable.
COPY src/microservices/runtimes/streamex/consoles/ps5/adapter.json /opt/adapter.json

# La manette annoncée à l'invité : une DualSense, puisque c'est une PS5.
ENV STREAMEX_PAD_VENDOR=0x054c \
    STREAMEX_PAD_PRODUCT=0x0ce6 \
    STREAMEX_PAD_NAME="Sony Interactive Entertainment DualSense Wireless Controller"

# LE MAPPING DEVINÉ PAR SDL EST FAUX POUR NOTRE PAD, et il l'est d'une façon
# très reconnaissable : Kyty embarque SDL en statique, base de mappings
# comprise, et cette base a une entrée pour 054c/0ce6 — celle d'une VRAIE
# DualSense. SDL l'applique donc à notre pad uinput, qui n'a pas le même ordre
# de boutons :
#
#   SDL attend   a:b1 b:b2 x:b0 y:b3   back:b8 start:b9   droite a2/a5   gâchettes a3/a4
#   on émet      a:b0 b:b1 x:b3 y:b2   back:b6 start:b7   droite a3/a4   gâchettes a2/a5
#
# Le hat (D-pad) et le stick gauche sont identiques de part et d'autre — d'où
# le symptôme exact rapporté le 2026-09-10 : « les directionnelles et
# l'analogique gauche marchent, pas le reste ».
#
# GUID SANS CRC, à la différence de l'image ps4 (qui porte le CRC du nom
# « streamex pad ps4 »). Deux raisons : la preuve que la forme sans CRC matche
# bien notre périphérique est sous nos yeux — c'est ainsi que l'entrée
# intégrée s'applique aujourd'hui — et elle couvre les QUATRE pads d'un coup,
# alors qu'un CRC ne vaut que pour le premier (les suivants s'appellent
# « …-p2/-p3/-p4 », donc un autre CRC). Un mapping fourni par
# SDL_GAMECONTROLLERCONFIG est prioritaire sur celui de la base intégrée pour
# le même GUID.
#
# Ce mapping suppose que streamex émet les boutons tels quels :
# `swap_xy_from_slot: -1` dans adapter.json. Absent, il valait 0 — X et Y
# inversés à l'émission sur chaque pad, carré et triangle échangés en jeu
# (trouvé sur la PS4, qui avait le même mapping, le 2026-10-22).
ENV SDL_GAMECONTROLLERCONFIG="030000004c050000e60c000011010000,PS5 Controller,a:b0,b:b1,x:b3,y:b2,back:b6,start:b7,guide:b8,leftshoulder:b4,rightshoulder:b5,leftstick:b9,rightstick:b10,leftx:a0,lefty:a1,rightx:a3,righty:a4,lefttrigger:a2,righttrigger:a5,dpup:h0.1,dpdown:h0.4,dpleft:h0.8,dpright:h0.2,platform:Linux,"

# Où les jeux sont montés, et où le scanner de l'adapter les cherche.
ENV KYTY_GAMES_DIR=/opt/game

ENV STREAMEX_ADAPTER_CONFIG=/opt/adapter.json

RUN chmod +x /entrypoint.sh /usr/local/bin/streamex /usr/local/bin/unpack-emulator

EXPOSE 8080

# JAMAIS DE RENDU LOGICIEL : llvmpipe est retiré du chargeur Vulkan.
#
# mesa-vulkan-drivers installe aussi llvmpipe (lavapipe), un moteur Vulkan qui
# rend sur le PROCESSEUR. Un émulateur qui le choisit tourne quand même —
# lentement, ou sur un écran noir — sans rien signaler. shadPS4 0.18.0 le
# choisissait à chaque partie (2026-09-11) : il départage deux GPU « non
# dédiés » à la taille du tas mémoire, et llvmpipe annonce toute la RAM de
# l'hôte contre la seule VRAM de l'APU. RPCS3 et Kyty choisissent RADV (mesuré),
# mais rien ne le garantit pour les autres : le chargeur l'écarte pour tous.
# Vérifié : avec cette variable, vulkaninfo ne liste plus que RADV.
ENV VK_LOADER_DRIVERS_DISABLE="*lvp*"

ENTRYPOINT ["/entrypoint.sh"]
