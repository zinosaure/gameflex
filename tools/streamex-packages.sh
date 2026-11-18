#!/usr/bin/env bash
# LES PAQUETS STREAMEX DU SERVEUR, CONSTRUITS ET PUBLIÉS (chantier 63, P2 et P3).
#
#   tools/streamex-packages.sh build <dossier cores/> <sortie> [adresse]
#                                       les paquets de tools/streamex-packages.tsv et leur index ; `adresse`, où les paquets
#                                       seront servis (défaut : la Release `streamex`) — un serveur d'essai sert les siens
#                                       sur son propre /downloads/streamex, comme son APK, et le kernel qui le suit le dit
#                                       par GAMEFLEX_STREAMEX_INDEX (le porteur, 2026-11-18)
#   GITHUB_TOKEN=… tools/streamex-packages.sh publish <sortie>   dans la Release `streamex` de zinosaure/gameflex-streamex
#
# PHONE_DIR=<dossier> ajoute les paquets du téléphone (P5) : `<invité>-<abi>.zip` de ce dossier (le /downloads de la dev,
# src/microservices/edge/src/public/downloads).
#
# Le porteur, 2026-11-18 : « Nous on ne fournit pas d'émulateurs, on fournit un moteur qui peut utiliser les
# émulateurs » ; « On ne publie pas Eden mais streamex-switch. À l'intérieur du streamex-switch on aura Eden. » Un paquet
# est un zip, `<image>-<version>.zip` : les fichiers que `cores/<console>/` tient (non compressés : ils le sont déjà) et
# `component.json` (la console, l'image, la version, les fichiers ; pour un émulateur patché, son dépôt officiel, le
# commit de base et le chemin de nos patchs). L'index, `components.json`, liste chaque paquet avec son adresse, sa taille
# et son sha256 : le kernel le lit pour l'écran Émulateurs, et vérifie l'empreinte avant d'installer.
#
# Un paquet déjà dans la Release n'est pas redéposé (son nom porte sa version) ; l'index, lui, est remplacé à chaque
# publication. Le jeton est lu dans l'environnement et passe par un fichier d'en-tête jetable.
set -euo pipefail

ROOT=$(cd "$(dirname "$0")/.." && pwd)
TABLE=$ROOT/tools/streamex-packages.tsv
REPO=zinosaure/gameflex-streamex
TAG=streamex
fail() { echo "✗ $*" >&2; exit 1; }

build() {
    local cores=$1 out=$2 base=${3:-https://github.com/$REPO/releases/download/$TAG}
    [ -d "$cores" ] || fail "pas de dossier $cores"
    mkdir -p "$out"
    python3 - "$TABLE" "$cores" "$out" "${base%/}" "$(git -C "$ROOT" rev-parse --short=8 HEAD)" <<'EOF'
import hashlib, json, os, sys, zipfile
table, cores, out, base, commit = sys.argv[1:]
packages = []
for line in open(table, encoding="utf-8"):
    if not line.strip() or line.startswith("#"):
        continue
    console, image, version, files, source = line.rstrip("\n").split("\t")
    files = files.split()
    name = f"{image}-{version}.zip"
    component = {"console": console, "image": image, "version": version, "files": files}
    if source != "-":
        upstream, patches = source.split()
        repository, commit_base = upstream.split("@")
        component["source"] = {"repository": repository, "base": commit_base, "patches": f"https://github.com/zinosaure/gameflex/tree/main/{patches}"}
    target = os.path.join(out, name)
    if not os.path.exists(target):
        for file in files:
            if not os.path.isfile(os.path.join(cores, console, file)):
                sys.exit(f"✗ {console}/{file} manque dans {cores}")
        partial = target + ".part"
        with zipfile.ZipFile(partial, "w", zipfile.ZIP_STORED) as archive:
            for file in files:
                archive.write(os.path.join(cores, console, file), file)
            archive.writestr("component.json", json.dumps(component, indent=2) + "\n")
        os.replace(partial, target)
        print(f"→ {name} ({os.path.getsize(target) >> 20} Mio)")
    else:
        print(f"= {name} (déjà construit)")
    digest = hashlib.sha256()
    with open(target, "rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            digest.update(block)
    packages.append({**component, "kind": "server", "file": name, "sizeBytes": os.path.getsize(target),
                     "sha256": digest.hexdigest(), "url": f"{base}/{name}"})
# LES CORRECTIFS (P6) : un paquet `streamex-patches-<empreinte courte>.zip`, `<console>/<identifiant>/…` dedans, depuis
# patches/ de la même bibliothèque, la liste de tools/streamex-patches.txt.
library = os.path.dirname(os.path.abspath(cores.rstrip("/")))
listed = [l.strip() for l in open(os.path.join(os.path.dirname(table), "streamex-patches.txt"), encoding="utf-8") if l.strip() and not l.startswith("#")]
if listed:
    entries, items = [], []
    for line in listed:
        console, name = line.split("/")
        folder = os.path.join(library, "patches", console, name)
        if not os.path.isdir(folder):
            sys.exit(f"✗ patches/{line} manque dans {library}")
        files = sorted(os.path.relpath(os.path.join(root, f), folder) for root, _, fs in os.walk(folder) for f in fs)
        items.append({"console": console, "name": name, "files": files})
        entries += [(os.path.join(folder, f), f"{console}/{name}/{f}") for f in files]
    content = hashlib.sha256()
    for path, arc in entries:
        content.update(arc.encode()); content.update(open(path, "rb").read())
    name = f"streamex-patches-{content.hexdigest()[:8]}.zip"
    target = os.path.join(out, name)
    if not os.path.exists(target):
        with zipfile.ZipFile(target + ".part", "w", zipfile.ZIP_DEFLATED) as archive:
            for path, arc in entries:
                info = zipfile.ZipInfo(arc, date_time=(2026, 1, 1, 0, 0, 0))
                info.compress_type = zipfile.ZIP_DEFLATED
                archive.writestr(info, open(path, "rb").read())
        os.replace(target + ".part", target)
        print(f"→ {name} ({len(items)} correctifs)")
    else:
        print(f"= {name} (déjà construit)")
    digest = hashlib.sha256(open(target, "rb").read()).hexdigest()
    packages.append({"kind": "patches", "file": name, "sizeBytes": os.path.getsize(target), "sha256": digest,
                     "url": f"{base}/{name}", "items": items})
# LES PAQUETS DU TÉLÉPHONE (P5, #639) : ceux que l'assemblage pose dans le /downloads de la dev (`<invité>-<abi>.zip`),
# pour les invités de tools/phone-guests.sh, chacun à la version que dit son `usr/VERSION`. Pour la Release, copiés sous
# le nom que l'APK connaît (`<version>-<abi>.zip`, un « + » écrit « . » comme GitHub le range) ; pour un /downloads
# (la sortie y est), l'index les désigne là où ils sont.
phone_dir = os.environ.get("PHONE_DIR", "")
if phone_dir:
    import shutil, subprocess
    in_place = os.path.dirname(os.path.abspath(out)) == os.path.abspath(phone_dir)
    guests = subprocess.run([os.path.join(os.path.dirname(table), "phone-guests.sh")], capture_output=True, text=True, check=True).stdout
    for line in guests.splitlines():
        guest = line.split()[0]
        for abi in ("aarch64", "x86_64"):
            source = os.path.join(phone_dir, f"{guest}-{abi}.zip")
            if not os.path.isfile(source):
                continue
            with zipfile.ZipFile(source) as bundle:
                version = bundle.read("usr/VERSION").decode().strip()
            if in_place:
                target, url = source, f"{base.rsplit('/', 1)[0]}/{guest}-{abi}.zip"
            else:
                name = f"{version.replace('+', '.')}-{abi}.zip"
                target, url = os.path.join(out, name), f"{base}/{name}"
                if not os.path.exists(target):
                    shutil.copyfile(source, target)
            digest = hashlib.sha256()
            with open(target, "rb") as stream:
                for block in iter(lambda: stream.read(1 << 20), b""):
                    digest.update(block)
            packages.append({"kind": "phone", "guest": guest, "abi": abi, "version": version, "file": os.path.basename(target),
                             "sizeBytes": os.path.getsize(target), "sha256": digest.hexdigest(), "url": url})
            print(f"{'=' if in_place else '→'} {guest}-{abi} : {version}")
index = {"format": 1, "builtFrom": commit, "packages": packages}
with open(os.path.join(out, "components.json"), "w", encoding="utf-8") as stream:
    json.dump(index, stream, indent=2)
    stream.write("\n")
print(f"→ components.json : {len(packages)} paquets")
EOF
}

publish() {
    local out=$1
    [ -f "$out/components.json" ] || fail "pas d'index dans $out : build d'abord"
    [ -n "${GITHUB_TOKEN:-}" ] || fail "GITHUB_TOKEN doit être dans l'environnement"
    local work headers
    work=$(mktemp -d)
    trap 'rm -rf "$work"' RETURN
    headers=$work/headers
    (umask 077 && printf 'Authorization: Bearer %s\n' "$GITHUB_TOKEN" > "$headers")
    github() {
        local method=$1 url=$2
        shift 2
        curl -fsS -X "$method" -H @"$headers" -H "Accept: application/vnd.github+json" "$@" "$url"
    }
    local api=https://api.github.com/repos/$REPO uploads=https://uploads.github.com/repos/$REPO
    if ! github GET "$api/releases/tags/$TAG" > "$work/release.json" 2>/dev/null; then
        github POST "$api/releases" -d "$(printf '{"tag_name":"%s","name":"Streamex packages","make_latest":"false","body":"%s"}' \
            "$TAG" "The streamex packages that GameFlex installs for its consoles, and their index (components.json). GameFlex downloads them from here.")" \
            > "$work/release.json"
        echo "→ Release $TAG créée"
    fi
    local id
    id=$(python3 -c 'import json,sys; print(json.load(open(sys.argv[1]))["id"])' "$work/release.json")
    python3 -c 'import json,sys; [print(a["name"], a["id"]) for a in json.load(open(sys.argv[1])).get("assets", [])]' "$work/release.json" > "$work/assets"
    local name size
    for name in $(python3 -c 'import json,sys; [print(p["file"]) for p in json.load(open(sys.argv[1]))["packages"]]' "$out/components.json"); do
        [ -f "$out/$name" ] || fail "$name n'est pas dans $out : un index fait pour un /downloads ne se publie pas"
        if grep -q "^$name " "$work/assets"; then
            echo "= $name (déjà dans la Release)"
            continue
        fi
        size=$(du -h "$out/$name" | cut -f1)
        github POST "$uploads/releases/$id/assets?name=$name" -H "Content-Type: application/zip" --data-binary @"$out/$name" > /dev/null
        echo "→ $name ($size)"
    done
    # L'index remplacé : l'ancien retiré, le nouveau déposé.
    local old
    old=$(awk '$1 == "components.json" {print $2}' "$work/assets")
    [ -z "$old" ] || github DELETE "$api/releases/assets/$old" > /dev/null
    github POST "$uploads/releases/$id/assets?name=components.json" -H "Content-Type: application/json" \
        --data-binary @"$out/components.json" > /dev/null
    echo "→ components.json"
}

case "${1:-}" in
    build) [ $# -ge 3 ] && [ $# -le 4 ] || fail "usage : $0 build <dossier cores/> <sortie> [adresse]"; build "$2" "$3" "${4:-}" ;;
    publish) [ $# -eq 2 ] || fail "usage : $0 publish <sortie>"; publish "$2" ;;
    *) fail "usage : $0 build <dossier cores/> <sortie> | publish <sortie>" ;;
esac
