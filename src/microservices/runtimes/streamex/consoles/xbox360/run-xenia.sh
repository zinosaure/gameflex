#!/bin/bash
# Xenia, RELANCÉ UNE FOIS s'il meurt à son démarrage.
#
# Sous Linux, Xenia plante parfois dès le lancement du titre (segmentation
# fault une seconde après le départ, 1 lancement sur 4 de Halo 3 le
# 2026-10-22) : ses fils d'exécution y sont encore incomplets (#519 chez
# Xenia). Sans relance, le joueur garde un écran noir. Une mort dans les 30
# premières secondes, par un signal de plantage, relance donc Xenia une fois —
# et le journal le dit. Un arrêt de la partie (TERM au groupe de processus)
# ne relance jamais : le piège sort aussitôt.
#
# Pas de core dump : une image mémoire de Xenia pèse des gigaoctets, et le
# disque système est serré.
ulimit -c 0
trap 'exit 143' TERM INT

for attempt in 1 2; do
    started=$SECONDS
    /opt/xenia/squashfs-root/usr/bin/xenia_canary "$@"
    code=$?
    elapsed=$(( SECONDS - started ))
    case "$code" in
        134|135|136|139) ;;   # SIGABRT, SIGBUS, SIGFPE, SIGSEGV
        *) exit "$code" ;;
    esac
    if [ "$attempt" -eq 2 ] || [ "$elapsed" -ge 30 ]; then
        exit "$code"
    fi
    echo "[xenia] mort au démarrage (code $code, après ${elapsed} s) — relancé une fois"
done
