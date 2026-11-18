#!/usr/bin/env bash
# NOS PATCHS D'ÉMULATEURS, RÉEXPORTÉS DEPUIS LEUR CLONE DE TRAVAIL (chantier 63).
#
#   tools/emulator-patches.sh export shadps4|kyty
#
# Aucun dépôt à nous ne porte le code d'un émulateur (le porteur, 2026-11-18 : « Ce que je ne veux pas c'est d'avoir
# des repos avec le code source des émulateurs »). Le code se change dans un clone LOCAL du dépôt officiel — ses
# branches à nous, jamais poussées nulle part —, puis ce script en tire les patchs que le dépôt garde et que
# `build.sh` applique au dépôt officiel, au commit de base que `build.sh` épingle (BASE) :
#
#   shadps4 : branche `gameflex` → tools/shadps4/patches/, un patch par commit ;
#   kyty    : branche `gameflex` → tools/kyty/patches/, un patch par commit ; branche `gameflex-phone-2` →
#             tools/kyty/patches-phone/gameflex-phone.patch, un seul patch (elle reçoit `gameflex` par fusion, une
#             série ne s'y rejoue pas).
#
# Le clone : SHADPS4_CLONE, KYTY_CLONE (défaut : ~/Github/shadPS4, ~/Github/KytyPS5). Le commit de base doit être un
# ancêtre de chaque branche (une base avancée : rebaser la branche d'abord). Chaque export est vérifié : appliqués au
# commit de base, les patchs redonnent l'arbre exact de la branche, ou rien n'est écrit.
set -euo pipefail

HERE=$(cd "$(dirname "$0")" && pwd)
fail() { echo "✗ $*" >&2; exit 1; }
[ "${1:-}" = export ] || fail "usage : tools/emulator-patches.sh export shadps4|kyty"

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# series CLONE BASE BRANCH DIR — un patch par commit, vérifié.
series() {
    local clone=$1 base=$2 branch=$3 dir=$4
    git -C "$clone" merge-base --is-ancestor "$base" "$branch" || fail "$base n'est pas un ancêtre de $branch"
    [ -z "$(git -C "$clone" log --merges --format=%h "$base..$branch")" ] || fail "$branch porte des fusions : une série ne s'y rejoue pas"
    # Les patchs partent dans le dépôt public, avec leur auteur : nos commits n'y portent que l'adresse que le clone
    # emploie (user.email, l'adresse publique de GitHub), jamais une adresse personnelle restée d'un autre clone.
    local me mail stray
    me=$(git -C "$clone" config user.name || true)
    mail=$(git -C "$clone" config user.email || true)
    if [ -n "$me" ]; then
        stray=$(git -C "$clone" log --format='%h %an <%ae>' "$base..$branch" | awk -v me="$me" -v mail="<$mail>" '{ m = $NF; sub(/^[^ ]+ /, ""); sub(/ <[^>]*>$/, ""); if ($0 == me && m != mail) print }')
        [ -z "$stray" ] || fail "des commits de « $me » portent une autre adresse que user.email ($(echo "$stray" | wc -l)) : corriger leur auteur d'abord"
    fi
    mkdir -p "$work/$branch"
    git -C "$clone" format-patch -q --no-signature --zero-commit -N "$base..$branch" -o "$work/$branch" > /dev/null
    git -C "$clone" worktree add -q --detach "$work/tree" "$base"
    git -C "$work/tree" -c user.name=GameFlex -c user.email=gameflex@users.noreply.github.com am -q --keep-cr "$work/$branch"/*.patch \
        || { git -C "$work/tree" am --abort 2>/dev/null; git -C "$clone" worktree remove --force "$work/tree"; fail "la série de $branch ne se rejoue pas"; }
    local got want
    got=$(git -C "$work/tree" rev-parse 'HEAD^{tree}')
    want=$(git -C "$clone" rev-parse "$branch^{tree}")
    git -C "$clone" worktree remove --force "$work/tree"
    [ "$got" = "$want" ] || fail "la série de $branch ne redonne pas son arbre"
    rm -f "$dir"/*.patch
    mkdir -p "$dir"
    cp "$work/$branch"/*.patch "$dir/"
    echo "✓ $dir : $(ls "$dir"/*.patch | wc -l) patchs, l'arbre de $branch à l'identique"
}

# single CLONE BASE BRANCH FILE — un seul patch (la différence), vérifié.
single() {
    local clone=$1 base=$2 branch=$3 file=$4
    git -C "$clone" merge-base --is-ancestor "$base" "$branch" || fail "$base n'est pas un ancêtre de $branch"
    git -C "$clone" diff --binary "$base" "$branch" > "$work/single.patch"
    git -C "$clone" worktree add -q --detach "$work/tree" "$base"
    git -C "$work/tree" apply --index "$work/single.patch" || { git -C "$clone" worktree remove --force "$work/tree"; fail "le patch de $branch ne s'applique pas"; }
    local got want
    got=$(git -C "$work/tree" write-tree)
    want=$(git -C "$clone" rev-parse "$branch^{tree}")
    git -C "$clone" worktree remove --force "$work/tree"
    [ "$got" = "$want" ] || fail "le patch de $branch ne redonne pas son arbre"
    mkdir -p "$(dirname "$file")"
    rm -f "$(dirname "$file")"/*.patch
    cp "$work/single.patch" "$file"
    echo "✓ $file : l'arbre de $branch à l'identique"
}

base_of() { sed -n 's/^BASE=//p' "$1"; }

case "${2:-}" in
    shadps4)
        clone=${SHADPS4_CLONE:-$HOME/Github/shadPS4}
        series "$clone" "$(base_of "$HERE/shadps4/build.sh")" gameflex "$HERE/shadps4/patches"
        ;;
    kyty)
        clone=${KYTY_CLONE:-$HOME/Github/KytyPS5}
        base=$(base_of "$HERE/kyty/build.sh")
        series "$clone" "$base" gameflex "$HERE/kyty/patches"
        single "$clone" "$base" gameflex-phone-2 "$HERE/kyty/patches-phone/gameflex-phone.patch"
        ;;
    *)
        fail "usage : tools/emulator-patches.sh export shadps4|kyty"
        ;;
esac
