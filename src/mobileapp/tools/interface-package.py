#!/usr/bin/env python3
"""LE PAQUET DE L'INTERFACE DE L'APK (chantier 64, `#786`).

    interface-package.py <dossier de la page> <dossier edge-versions> <empreinte native>

La page (le `dist/` d'edge et les licences, ce que l'APK embarque sous `assets/web`) part dans
`interface-<empreinte du contenu>.zip`, avec `interface.json` à sa racine : sa version, son commit, la date de son
build (ceux de `build-info.json`) et l'empreinte native de l'APK pour laquelle elle est faite. Le zip est
déterministe — entrées triées, dates fixes : le même contenu donne le même nom et le même sha256.

LES VERSIONS D'EDGE, COMME LES PAQUETS STREAMEX (le porteur, 2026-11-18 : « Tu fais comme pour l'Apk et Streamex. Le
staging passe par …/downloads/edge-versions/… Et pour release dans GitHub avec `edge-versions` ») : le dossier garde
plusieurs paquets et leur index, `index.json` — `{format, versions: [entrée…]}`, la plus récente d'abord. Chaque
téléphone y prend la plus récente faite pour son APK (la même empreinte native). Un paquet déjà listé n'est pas répété ;
l'index garde les vingt dernières, et les paquets qu'il ne nomme plus quittent le dossier. Imprime l'entrée.
"""
import hashlib
import json
import os
import sys
import zipfile

web, out, native = sys.argv[1], sys.argv[2], sys.argv[3]
KEPT = 20

info = {}
try:
    with open(os.path.join(web, "build-info.json"), encoding="utf-8") as stream:
        info = json.load(stream)
except (OSError, ValueError):
    sys.exit(f"✗ {web}/build-info.json illisible : la page n'est pas un build d'edge")
if not os.path.isfile(os.path.join(web, "index.html")):
    sys.exit(f"✗ {web}/index.html absent")

files = []
for root, _, names in os.walk(web):
    for name in names:
        path = os.path.join(root, name)
        files.append((os.path.relpath(path, web).replace(os.sep, "/"), path))
files.sort()

description = {"version": info.get("version", ""), "commit": info.get("commit", ""),
               "builtAt": info.get("builtAt", ""), "native": native}
content = hashlib.sha256()
for name, path in files:
    content.update(name.encode() + b"\0")
    with open(path, "rb") as stream:
        content.update(stream.read())
content.update(json.dumps(description, sort_keys=True).encode())

os.makedirs(out, exist_ok=True)
name = f"interface-{content.hexdigest()[:12]}.zip"
target = os.path.join(out, name)
partial = target + ".part"
with zipfile.ZipFile(partial, "w", zipfile.ZIP_DEFLATED) as archive:
    for entry, path in files:
        record = zipfile.ZipInfo(entry, date_time=(2026, 1, 1, 0, 0, 0))
        record.compress_type = zipfile.ZIP_DEFLATED
        with open(path, "rb") as stream:
            archive.writestr(record, stream.read())
    record = zipfile.ZipInfo("interface.json", date_time=(2026, 1, 1, 0, 0, 0))
    archive.writestr(record, json.dumps(description, indent=2) + "\n")
os.replace(partial, target)

digest = hashlib.sha256()
with open(target, "rb") as stream:
    for block in iter(lambda: stream.read(1 << 20), b""):
        digest.update(block)
entry = {"file": name, "sha256": digest.hexdigest(), "sizeBytes": os.path.getsize(target), **description}

index_path = os.path.join(out, "index.json")
try:
    with open(index_path, encoding="utf-8") as stream:
        index = json.load(stream)
except (OSError, ValueError):
    index = {"format": 1, "versions": []}
versions = [entry] + [v for v in index.get("versions", []) if v.get("file") != name]
versions.sort(key=lambda v: v.get("builtAt", ""), reverse=True)
index = {"format": 1, "versions": versions[:KEPT]}
with open(index_path + ".part", "w", encoding="utf-8") as stream:
    json.dump(index, stream, indent=2)
    stream.write("\n")
os.replace(index_path + ".part", index_path)
named = {v["file"] for v in index["versions"]}
for leftover in os.listdir(out):
    if leftover.startswith("interface-") and leftover.endswith(".zip") and leftover not in named:
        os.remove(os.path.join(out, leftover))

print(json.dumps(entry))
