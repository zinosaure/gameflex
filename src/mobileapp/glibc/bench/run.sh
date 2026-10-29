#!/usr/bin/env bash
# Le banc Linux du traceur glibc.
#
# Reproduit le filtre seccomp de l'émulateur x86_64 dans un debian:12-slim
# jetable (seccomp_trap.c rend SIGSYS pour la même liste d'appels) et vérifie,
# en une seconde par essai, que ptracer fait vivre un invité glibc : d'abord
# des threads, puis un client X sur un vrai Xvfb. Un tour d'APK coûte cinq
# minutes ; ici une case mal ordonnée de la table de réécriture se voit tout de
# suite.
#
# À lancer depuis l'hôte :   src/mobileapp/glibc/bench/run.sh
# Sort en erreur si une attente n'est pas tenue. Le CONTRÔLE NÉGATIF passe en
# premier — un banc qui ne reproduit pas le mur ne prouverait rien.
set -euo pipefail

if [ "${1:-}" != "--inner" ]; then
    HERE=$(cd "$(dirname "$0")/.." && pwd)
    # LE DEBIAN DU RUNTIME, lu là où il est déclaré : le banc doit éprouver le
    # traceur contre la glibc que l'appareil exécutera, pas contre une autre.
    IMAGE=$(sed -n 's/^RUNTIME_IMAGE="\(.*\)"$/\1/p' "${HERE}/../tools/assemble-glibc.sh")
    [ -n "$IMAGE" ] || { echo "✗ RUNTIME_IMAGE introuvable dans tools/assemble-glibc.sh" >&2; exit 2; }
    exec docker run --rm -v "${HERE}":/glibc:ro "$IMAGE" bash /glibc/bench/run.sh --inner
fi

export DEBIAN_FRONTEND=noninteractive
apt-get update -qq > /dev/null 2>&1
apt-get install -y -qq gcc libc6-dev libxcb1-dev xvfb > /dev/null 2>&1
echo "── glibc $(ldd --version | head -1 | grep -oE '[0-9]+\.[0-9]+$')"

cd /tmp
gcc -O2 -Wall -o ptracer /glibc/ptracer.c
gcc -O2 -Wall -o seccomp-trap /glibc/bench/seccomp_trap.c
gcc -O2 -o draw-xcb /glibc/bench/draw_xcb.c -lxcb
gcc -O2 -pthread -o witness-mt /glibc/bench/witness_mt.c
gcc -O2 -o witness-ids /glibc/bench/witness_ids.c

Xvfb :7 -ac -nolisten tcp -screen 0 640x480x24 > /dev/null 2>&1 &
for _ in $(seq 1 20); do [ -S /tmp/.X11-unix/X7 ] && break; sleep 0.3; done
[ -S /tmp/.X11-unix/X7 ] || { echo "✗ Xvfb :7 ne s'est pas levé"; exit 1; }

fail=0
expect() {   # expect <libellé> <motif grep -E> <texte>
    if grep -qE -- "$2" <<< "$3"; then
        echo "  ✓ $1"
    else
        echo "  ✗ $1 — attendu /$2/, obtenu :"
        sed 's/^/      /' <<< "$3"
        fail=1
    fi
}

echo "── 1. contrôle négatif : sous le filtre, SANS traceur (attendu : 159)"
set +e
./seccomp-trap ./witness-mt > /dev/null 2>&1; c1=$?
./seccomp-trap ./draw-xcb /tmp/.X11-unix/X7 > /dev/null 2>&1; c2=$?
set -e
expect "witness-mt meurt en SIGSYS" "^159$" "$c1"
expect "draw-xcb meurt en SIGSYS" "^159$" "$c2"

echo "── 2. threads sous traceur (set_robust_list, rseq, clone3 → repli)"
out=$(./ptracer ./seccomp-trap ./witness-mt 2>&1)
expect "les 6 threads vivent" "glibc-mt-witness-ok" "$out"
expect "exit=0, 7 tracés" "exit=0 .*tracés=7" "$out"

echo "── 3. client X sous traceur (poll réécrit en ppoll)"
out=$(timeout 40 ./ptracer ./seccomp-trap ./draw-xcb /tmp/.X11-unix/X7 2>&1)
expect "connecté au serveur, dessine" "je dessine" "$out"
expect "va au bout" "draw-xcb-ok" "$out"
expect "poll → ppoll" "7→271" "$out"
expect "exit=0" "exit=0" "$out"
echo "  $(tail -1 <<< "$out")"

echo "── 4. client X par le nom ABSTRAIT standard (celui de l'application)"
# Xvfb écoute aussi sur @/tmp/.X11-unix/X7 : c'est ainsi que l'écran de
# l'application est nommé (Xserver.socketName), sans fichier ; draw-xcb doit
# savoir l'atteindre par un « @ ».
out=$(timeout 40 ./ptracer ./seccomp-trap ./draw-xcb @/tmp/.X11-unix/X7 2>&1)
expect "connecté par la socket abstraite" "je dessine" "$out"
expect "va au bout" "draw-xcb-ok" "$out"

echo "── 5. pathshim : les chemins qu'Android n'a pas, traduits dans la libc"
# cat et readlink sont des binaires glibc stock : ce qu'ils voient, un invité
# le voit. /proc/sys/vm/overcommit_memory → notre fichier ; /proc/self/exe →
# l'invité déclaré, pas le chargeur.
gcc -O2 -shared -fPIC -o libpathshim.so /glibc/pathshim.c -ldl
printf 'traduit\n' > /tmp/faux-overcommit
out=$(GAMEFLEX_PATHMAP="/proc/sys/vm/overcommit_memory=/tmp/faux-overcommit" LD_PRELOAD=./libpathshim.so cat /proc/sys/vm/overcommit_memory 2>&1)
expect "overcommit_memory lu depuis notre fichier" "^traduit$" "$out"
out=$(GAMEFLEX_EXE=/invite/usr/bin/invite LD_PRELOAD=./libpathshim.so readlink /proc/self/exe 2>&1)
expect "/proc/self/exe est l'invité" "^/invite/usr/bin/invite$" "$out"
# cat échoue ici par construction (/tmpfoo n'existe pas) : `|| true`, sinon
# set -e arrête le banc sur la substitution et le test n'est jamais évalué.
out=$(GAMEFLEX_PATHMAP="/tmp=/nulle-part" LD_PRELOAD=./libpathshim.so cat /tmpfoo 2>&1 || true)
expect "un préfixe ne déborde pas (/tmpfoo intact)" "/tmpfoo" "$out"

echo "── 6. identifiants : « abandonner » ses privilèges pour les mêmes (PulseAudio)"
# Contrôle négatif d'abord : sans traceur, le filtre tue sur setresgid.
set +e
./seccomp-trap ./witness-ids > /dev/null 2>&1; c6=$?
set -e
expect "witness-ids meurt en SIGSYS sans traceur" "^159$" "$c6"
out=$(./ptracer ./seccomp-trap ./witness-ids 2>&1)
expect "setresgid/setresuid aux mêmes identifiants : fait" "ids-ok" "$out"
expect "les deux appels sont passés par le traceur" "119×1.*117×1|117×1.*119×1" "$out"

exit "$fail"
